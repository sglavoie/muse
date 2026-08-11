package player

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

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

// handleRestart runs one full outage recovery cycle synchronously (the
// supervisor's inner attempt loop). Retained as a stable entry point for tests
// that drive recovery directly rather than through the disconnect channel.
func (m *Manager) handleRestart() {
	m.recoverOutage()
}
