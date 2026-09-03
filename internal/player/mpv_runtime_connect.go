package player

import (
	"context"
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/safego"
)

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
	// A freshly connected mpv starts with an empty playlist, and the restore
	// sequence below replaces whatever it holds, so nothing that was tracked as
	// prefetched still exists there. Open a new playlist generation and forget
	// the tracking without an IPC removal: any prefetch resolved against the old
	// generation is now unpromotable at EOF.
	m.playlistGen++
	m.dropPrefetchTrackingLocked()
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

	// The activation gate is open again. Best-effort prefetch is suppressed for
	// the whole activation window (playlistClient returns nil while activating)
	// so an append cannot land in a playlist the restore is about to replace;
	// rebuild it now that mpv's playlist reflects the restored state.
	safego.Go("player.prefetchNext", m.prefetchNext)

	// startEventReader decodes mpv's async JSON event stream, the most
	// third-party-data-driven loop in the process; a panic there must not take
	// muse down with it.
	safego.Go("player.healthCheck", func() { m.healthCheck(ctx, client) })
	safego.Go("player.startEventReader", func() { m.startEventReader(ctx, client) })
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
