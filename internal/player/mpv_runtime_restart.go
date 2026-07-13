package player

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// healthCheck runs in a goroutine after the IPC socket connects. It pings mpv
// at regular intervals; on the first ping failure it hands the error to the
// shared connection-failure transition, which detaches the client and signals
// the restart supervisor.
func (m *Manager) healthCheck(ctx context.Context, client *IPCClient) {
	interval := m.pingInterval
	if interval == 0 {
		interval = defaultPingInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := client.SendCommand("get_version"); err != nil {
				m.handleConnectionFailure(ctx, client, err)
				return
			}
		}
	}
}

// handleConnectionFailure is the single generation-guarded disconnect
// transition shared by the health-check goroutine and the dedicated event
// reader. Whichever goroutine first observes the still-installed command client
// closes it, detaches it, re-enables command queueing, and signals the restart
// supervisor exactly once; a second caller for the same generation sees
// m.client != client and does nothing, so simultaneous health and event
// failures cannot launch duplicate restarts.
//
// Failures from a canceled or superseded generation are ignored: if ctx is
// already done, a restart has replaced this connection and a late failure from
// the old generation must not resurrect the queue or restart a healthy
// replacement. A nil client means there is no installed generation to fail
// (only reachable from isolated unit tests), so it is likewise a no-op.
func (m *Manager) handleConnectionFailure(ctx context.Context, client *IPCClient, err error) {
	if client == nil {
		return
	}

	// Stale/canceled generation: the subprocess context was already replaced by
	// a restart, so this failure belongs to a dead generation.
	select {
	case <-ctx.Done():
		return
	default:
	}

	// If the manager is shutting down, do not resurrect the cmdQueue (Close set
	// it to nil) — doing so would make subsequent Player calls silently enqueue
	// commands that will never be replayed instead of returning ErrNotConnected.
	shuttingDown := false
	if m.rootCtx != nil {
		select {
		case <-m.rootCtx.Done():
			shuttingDown = true
		default:
		}
	}

	m.mu.Lock()
	if m.client != client {
		// Another goroutine already handled this generation's failure, or the
		// client was already replaced by a newer generation. Do nothing.
		m.mu.Unlock()
		return
	}
	m.client = nil
	if !shuttingDown {
		m.cmdQueue = []func() error{}
	}
	m.mu.Unlock()

	_ = client.Close()

	slog.Info("mpv disconnected", "err", err)
	if !shuttingDown {
		select {
		case m.disconnectCh <- err:
		default:
		}
	}
}

// watchAndRestart is the restart supervisor. It loops until the manager is
// closed, and on each disconnect signal it runs a full outage recovery cycle
// (recoverOutage owns the inner launch-and-connect attempt loop). It returns
// once recovery is permanently abandoned or the manager shuts down.
func (m *Manager) watchAndRestart() {
	for {
		select {
		case <-m.rootCtx.Done():
			return
		case <-m.disconnectCh:
			if !m.recoverOutage() {
				return
			}
		}
	}
}

// restoreState captures the user-visible playback settings that must be
// re-applied after mpv reconnects. It is snapshotted once per outage so every
// attempt restores the same pre-crash state.
type restoreState struct {
	path       string
	elapsed    float64
	paused     bool
	volume     float64
	hasVolume  bool
	repeatMode string
	gapless    bool
}

// snapshotRestore captures the current user-visible playback state under the
// manager lock. Audio filters (normalize, EQ, stereo widening) are not copied
// here: the audioFilters map itself survives the subprocess restart, so
// restoreStableProperties re-reads it directly.
func (m *Manager) snapshotRestore() restoreState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return restoreState{
		path:       m.lastTrackPath,
		elapsed:    m.lastElapsed,
		paused:     m.lastPaused,
		volume:     m.lastVolume,
		hasVolume:  m.volumeKnown,
		repeatMode: m.repeatMode,
		gapless:    m.gaplessEnabled,
	}
}

// recoverOutage drives the inner attempt loop for a single mpv outage. It makes
// an immediate first attempt, applies exponential backoff (capped) before each
// subsequent attempt, and gives up after the configured maximum number of
// attempts. It returns true once a launch-and-connect attempt succeeds and
// false when recovery is abandoned (after clearing the command queue and
// notifying the user). This is the shared retry path for every failure mode:
// initial socket timeout, restart launch failure, and restart socket timeout.
func (m *Manager) recoverOutage() bool {
	m.mu.RLock()
	disconnFn := m.callbacks.OnDisconnected
	statusFn := m.callbacks.OnStatus
	m.mu.RUnlock()
	if disconnFn != nil {
		disconnFn()
	}
	if statusFn != nil {
		statusFn("mpv disconnected, restarting…")
	}

	restore := m.snapshotRestore()
	base := m.effectiveBaseDelay()
	maxDelay := m.effectiveMaxDelay()
	maxAttempts := m.effectiveMaxAttempts()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-m.rootCtx.Done():
				return false
			case <-time.After(backoffDelay(base, maxDelay, attempt)):
			}
		}
		if m.attemptRestart(restore) {
			return true
		}
	}

	// Every attempt failed. Fail fast so subsequent Player calls return
	// ErrNotConnected instead of silently queueing into a dead queue, and tell
	// the user recovery was abandoned. The give-up notification and queue
	// teardown happen only here — never on an individual failed attempt.
	m.mu.Lock()
	m.cmdQueue = nil
	fn := m.callbacks.OnStatus
	m.mu.Unlock()
	if fn != nil {
		fn("mpv: gave up after too many restart attempts")
	}
	return false
}

// backoffDelay returns the delay before the given attempt (attempt >= 2), an
// exponential ramp from base doubling each attempt, capped at maxDelay. The
// shift is bounded so a large max-attempts override cannot overflow.
func backoffDelay(base, maxDelay time.Duration, attempt int) time.Duration {
	if shift := attempt - 2; shift >= 0 && shift < 31 {
		if d := base << shift; d > 0 && d < maxDelay {
			return d
		}
	}
	return maxDelay
}

// attemptRestart performs one launch-and-connect attempt: it tears down the old
// subprocess (bounded reap/kill), removes the stale socket, launches a fresh
// mpv, and waits for the IPC socket. On success it activates the connection and
// restores playback state. It returns true on a successful reconnect and false
// on a launch failure or socket timeout — either of which the caller retries.
// User commands queued during the outage are preserved across failed attempts.
func (m *Manager) attemptRestart(restore restoreState) bool {
	m.mu.Lock()
	m.cancel()
	oldCmd := m.cmd
	m.cmd = nil
	newCtx, newCancel := context.WithCancel(m.rootCtx)
	m.ctx = newCtx
	m.cancel = newCancel
	// Preserve user commands queued during the outage: only ensure the queue is
	// non-nil so Player calls keep queueing while reconnecting. Resetting it
	// here would drop controls issued between attempts.
	if m.cmdQueue == nil {
		m.cmdQueue = []func() error{}
	}
	m.prefetchedIndex = -1
	m.prefetchedPath = ""
	m.prefetchedIdentity = ""
	// Shuffle traversal state (shuffleEnabled/History/Pos/Unseen) is deliberately
	// left intact: the Manager survives the mpv subprocess restart, so playback
	// resumes into the same shuffle cycle rather than reanchoring.
	launchFn := m.launchFn
	m.mu.Unlock()

	// Reap the old process on every attempt so it never lingers as a zombie.
	m.reapProcess(oldCmd)

	if err := os.Remove(m.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("player: remove socket", "path", m.socketPath, "err", err)
	}

	if launchFn == nil {
		slog.Warn("mpv: restart attempt failed: launchFn is nil")
		newCancel()
		return false
	}

	newCmd, err := launchFn(newCtx, m.socketPath)
	if err != nil {
		slog.Warn("mpv: restart attempt: launch failed", "err", err)
		newCancel()
		return false
	}
	m.mu.Lock()
	m.cmd = newCmd
	m.mu.Unlock()

	client, err := m.connectSocket(newCtx)
	if err != nil {
		// The freshly launched process never opened its socket. Cancel its
		// context and reap it so it does not linger before the next attempt.
		slog.Warn("mpv: restart attempt: socket did not come up", "err", err)
		m.mu.Lock()
		stuck := m.cmd
		m.cmd = nil
		m.mu.Unlock()
		newCancel()
		m.reapProcess(stuck)
		return false
	}

	m.activateConnection(newCtx, client, func() {
		// Stable properties first, resume second (replay of queued user
		// commands happens afterwards, inside activateConnection).
		m.restoreStableProperties(restore)
		if restore.path != "" {
			if err := m.loadFileAt(restore.path, restore.elapsed, restore.paused); err != nil {
				slog.Warn("mpv: resume after restart failed", "err", err)
			}
		}
	})
	return true
}

// reapProcess waits for cmd to exit so it does not remain a zombie, bounding the
// wait so a non-responsive process cannot freeze the restart cycle. A process
// that refuses to exit within shutdownTimeout is force-killed and reaped in the
// background.
func (m *Manager) reapProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		slog.Warn("mpv: old process did not exit within timeout on restart, killing")
		if err := cmd.Process.Kill(); err != nil {
			slog.Debug("mpv: kill old process on restart", "err", err)
		}
		go func() { <-done }()
	}
}

// connectSocket polls the mpv IPC socket until it accepts a connection, the
// context is cancelled, or the poll timeout elapses. It returns the connected
// client, or an error (the context error on cancellation, errConnectTimeout on
// timeout). It performs no installation and starts no goroutines — activation
// is the supervisor's responsibility.
func (m *Manager) connectSocket(ctx context.Context) (*IPCClient, error) {
	deadline := time.Now().Add(m.effectiveSocketPollTimeout())
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		client, err := NewIPCClient(m.socketPath)
		if err == nil {
			return client, nil
		}

		if !time.Now().Before(deadline) {
			return nil, errConnectTimeout
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(socketPollInterval):
		}
	}
}

// activateConnection installs a freshly connected client, applies any state
// restoration (restore != nil on a restart), replays the commands queued while
// disconnected, and starts the health-check and event-reader goroutines. The
// restore hook runs before the queued commands so stable properties and the
// resume loadfile are applied ahead of the user's outage-time controls.
func (m *Manager) activateConnection(ctx context.Context, client *IPCClient, restore func()) {
	// Install the client under the lock, re-checking cancellation to defeat a
	// TOCTOU race against Close (which nils m.client and m.cmdQueue together).
	// The client is installed with activating=true so public commands keep
	// enqueueing (admit / setFilter observe the flag) while the restore sequence
	// runs; the activation goroutine below reaches mpv through the ungated ipc()
	// helper. Without this gate a concurrent user command could run against the
	// fresh client and then be clobbered by restore().
	m.mu.Lock()
	select {
	case <-ctx.Done():
		m.mu.Unlock()
		_ = client.Close()
		return
	default:
	}
	m.client = client
	m.activating = true
	m.mu.Unlock()

	slog.Info("mpv connected", "socket", m.socketPath)

	if restore != nil {
		restore()
	}

	// Restore is complete, so nothing the user issued can be overwritten now.
	// Replay everything queued during the outage and the activation window in
	// order; the gate stays closed for the whole drain so a control arriving
	// mid-replay is admitted behind the backlog instead of overtaking it, and is
	// only opened once the queue is fully drained.
	m.drainQueue()

	go m.healthCheck(ctx, client)
	go m.startEventReader(ctx, client)
}

// drainQueue replays every command captured during the outage and activation
// window against the freshly restored client, in order, and only then opens the
// activation gate. The gate stays closed (activating remains true) for the whole
// drain, so a control that arrives mid-drain is admitted behind the backlog
// rather than racing ahead of it. Draining continues until the queue is empty,
// at which point activating is cleared and cmdQueue detached in the same locked
// step: from that instant admit lets subsequent controls reach the client
// directly, guaranteed to run after everything replayed here. Each queued
// closure resolves the client through the ungated ipc() helper, so a replayed
// command never re-enters the activation gate and cannot recursively re-enqueue
// itself.
func (m *Manager) drainQueue() {
	for {
		m.mu.Lock()
		if m.client == nil {
			// Close (or a fresh failure) detached the client mid-drain; it
			// already cleared activating and cmdQueue, so stop replaying.
			m.mu.Unlock()
			return
		}
		if len(m.cmdQueue) == 0 {
			m.activating = false
			m.cmdQueue = nil
			m.mu.Unlock()
			return
		}
		cmd := m.cmdQueue[0]
		m.cmdQueue = m.cmdQueue[1:]
		m.mu.Unlock()

		if err := cmd(); err != nil {
			slog.Warn("mpv: queued command failed on replay", "err", err)
		}
	}
}

// restoreStableProperties re-applies the user-visible settings that mpv loses
// on restart: volume, repeat mode, gapless playback, and the active audio
// filters (normalize, equalizer, stereo widening). It runs after the client is
// installed and before the resume loadfile.
func (m *Manager) restoreStableProperties(restore restoreState) {
	c, err := m.ipc()
	if err != nil {
		return
	}
	if restore.hasVolume {
		if _, err := c.SendCommand(cmdSetProperty, propVolume, restore.volume); err != nil {
			slog.Warn("mpv: restore volume after restart failed", "err", err)
		}
	}
	if restore.repeatMode != "" {
		// Record the mode and send the loop properties directly. Using SetRepeat
		// here would route through the gated admit path (activating is still
		// true), deferring the repeat restore behind the resume loadfile and
		// every replayed user command.
		m.mu.Lock()
		m.repeatMode = restore.repeatMode
		m.mu.Unlock()
		if err := sendRepeat(c, restore.repeatMode); err != nil {
			slog.Warn("mpv: restore repeat mode after restart failed", "err", err)
		}
	}
	// Always re-apply the saved gapless state explicitly. Sending only for the
	// enabled case would let a disabled setting fall back to mpv's default weak
	// gapless mode while the manager still reports gapless off, so send yes or no
	// to match the snapshotted boolean (mirroring SetGapless).
	// Always re-apply the saved gapless state explicitly. Sending only for the
	// enabled case would let a disabled setting fall back to mpv's default weak
	// gapless mode while the manager still reports gapless off, so send yes or no
	// to match the snapshotted boolean (mirroring SetGapless).
	gaplessVal := "no"
	if restore.gapless {
		gaplessVal = "yes"
	}
	if _, err := c.SendCommand(cmdSetProperty, propGaplessAudio, gaplessVal); err != nil {
		slog.Warn("mpv: restore gapless after restart failed", "err", err)
	} else if _, err := c.SendCommand(cmdSetProperty, propPrefetchPlaylist, gaplessVal); err != nil {
		slog.Warn("mpv: restore prefetch-playlist after restart failed", "err", err)
	}
	m.mu.RLock()
	hasFilters := len(m.audioFilters) > 0
	m.mu.RUnlock()
	if hasFilters {
		if err := m.rebuildAudioFilters(); err != nil {
			slog.Warn("mpv: restore audio filters after restart failed", "err", err)
		}
	}
}

// handleRestart runs one full outage recovery cycle synchronously (the
// supervisor's inner attempt loop). Retained as a stable entry point for tests
// that drive recovery directly rather than through the disconnect channel.
func (m *Manager) handleRestart() {
	m.recoverOutage()
}

// effectiveBaseDelay is the initial restart backoff delay, honouring the
// test-only restartBaseDelay override.
func (m *Manager) effectiveBaseDelay() time.Duration {
	if m.restartBaseDelay != 0 {
		return m.restartBaseDelay
	}
	return restartInitialDelay
}

// effectiveMaxDelay is the backoff cap, honouring the test-only override.
func (m *Manager) effectiveMaxDelay() time.Duration {
	if m.restartMaxDelayOverride != 0 {
		return m.restartMaxDelayOverride
	}
	return restartMaxDelay
}

// effectiveMaxAttempts is the maximum number of launch-and-connect attempts per
// outage, honouring the test-only override.
func (m *Manager) effectiveMaxAttempts() int {
	if m.restartMaxAttemptsOverride > 0 {
		return m.restartMaxAttemptsOverride
	}
	return restartMaxAttempts
}

// effectiveSocketPollTimeout bounds a single connect attempt, honouring the
// test-only override.
func (m *Manager) effectiveSocketPollTimeout() time.Duration {
	if m.socketPollTimeoutOverride != 0 {
		return m.socketPollTimeoutOverride
	}
	return socketPollTimeout
}
