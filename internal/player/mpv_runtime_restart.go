package player

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"
)

// healthCheck runs in a goroutine after the IPC socket connects. It pings mpv
// at regular intervals; on the first ping failure it closes the stale client,
// marks it as disconnected, and forwards the error to disconnectCh.
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
			_, err := client.SendCommand("get_version")
			if err == nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			_ = client.Close()

			// If the manager is shutting down, do not resurrect the cmdQueue
			// (Close set it to nil) — doing so would make subsequent Player
			// calls silently enqueue commands that will never be replayed
			// instead of returning ErrNotConnected.
			shuttingDown := false
			if m.rootCtx != nil {
				select {
				case <-m.rootCtx.Done():
					shuttingDown = true
				default:
				}
			}

			m.mu.Lock()
			if m.client == client {
				m.client = nil
				if !shuttingDown {
					m.cmdQueue = []func() error{}
				}
			}
			m.mu.Unlock()

			slog.Info("mpv disconnected", "err", err)
			if !shuttingDown {
				select {
				case m.disconnectCh <- err:
				default:
				}
			}
			return
		}
	}
}

// watchAndRestart loops until the manager is closed, restarting mpv whenever
// the health-check goroutine signals a disconnect on disconnectCh. It applies
// exponential backoff between consecutive failures and gives up after
// restartMaxAttempts attempts without a successful reconnect.
func (m *Manager) watchAndRestart() {
	initialDelay := m.restartBaseDelay
	if initialDelay == 0 {
		initialDelay = restartInitialDelay
	}

	var consecutiveFailures int
	for {
		select {
		case <-m.rootCtx.Done():
			return
		case <-m.disconnectCh:
			consecutiveFailures++
			if consecutiveFailures > restartMaxAttempts {
				m.mu.Lock()
				m.cmdQueue = nil
				fn := m.callbacks.OnStatus
				m.mu.Unlock()
				if fn != nil {
					fn("mpv: gave up after too many restart attempts")
				}
				return
			}
			if consecutiveFailures > 1 {
				delay := min(initialDelay*time.Duration(1<<(consecutiveFailures-2)), restartMaxDelay)
				select {
				case <-m.rootCtx.Done():
					return
				case <-time.After(delay):
				}
			}
			m.handleRestart()
			if m.backoffReset.Swap(false) {
				consecutiveFailures = 0
			}
		}
	}
}

// handleRestart performs one auto-restart cycle: emits a status notification,
// cancels the old subprocess context, launches a new mpv process, queues a
// resume loadfile command, and begins polling for the new socket.
func (m *Manager) handleRestart() {
	m.mu.RLock()
	fn := m.callbacks.OnStatus
	disconnFn := m.callbacks.OnDisconnected
	path := m.lastTrackPath
	elapsed := m.lastElapsed
	paused := m.lastPaused
	m.mu.RUnlock()

	if disconnFn != nil {
		disconnFn()
	}
	if fn != nil {
		fn("mpv disconnected, restarting…")
	}

	m.mu.Lock()
	m.cancel()
	oldCmd := m.cmd
	m.cmd = nil
	newCtx, newCancel := context.WithCancel(m.rootCtx)
	m.ctx = newCtx
	m.cancel = newCancel
	m.cmdQueue = []func() error{}
	m.prefetchedIndex = -1
	m.prefetchedPath = ""
	m.mu.Unlock()

	// Reap the old mpv process so it does not remain as a zombie.
	if oldCmd != nil && oldCmd.Process != nil {
		_ = oldCmd.Wait()
	}

	if err := os.Remove(m.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("player: remove socket", "path", m.socketPath, "err", err)
	}

	if m.launchFn == nil {
		// Clear the cmd queue so subsequent player calls return
		// ErrNotConnected rather than silently appending to a queue
		// that will never be drained.
		m.mu.Lock()
		m.cmdQueue = nil
		m.mu.Unlock()
		if fn != nil {
			fn("mpv: restart failed: launchFn is nil")
		}
		return
	}

	newCmd, err := m.launchFn(newCtx, m.socketPath)
	if err != nil {
		// Clear the cmd queue so subsequent player calls return
		// ErrNotConnected rather than silently appending to a queue
		// that will never be drained.
		m.mu.Lock()
		m.cmdQueue = nil
		m.mu.Unlock()
		if fn != nil {
			fn("mpv: restart failed: " + err.Error())
		}
		return
	}
	m.mu.Lock()
	m.cmd = newCmd
	m.mu.Unlock()

	// Re-apply audio filters (normalize, EQ, stereo wide) that were active
	// before the crash. The audioFilters map survives the restart; mpv's
	// fresh process does not have them, so we must replay them.
	m.mu.RLock()
	hasFilters := len(m.audioFilters) > 0
	gapless := m.gaplessEnabled
	m.mu.RUnlock()
	if hasFilters {
		if err := m.enqueue(func() error { return m.rebuildAudioFilters() }); err != nil {
			slog.Warn("mpv: failed to enqueue audio filters on restart", "err", err)
		}
	}
	if gapless {
		if err := m.enqueue(func() error {
			c, ipcErr := m.ipc()
			if ipcErr != nil {
				return ipcErr
			}
			if _, err := c.SendCommand(cmdSetProperty, propGaplessAudio, "yes"); err != nil {
				return err
			}
			_, err := c.SendCommand(cmdSetProperty, propPrefetchPlaylist, "yes")
			return err
		}); err != nil {
			slog.Warn("mpv: failed to enqueue gapless on restart", "err", err)
		}
	}

	if path != "" {
		if err := m.enqueue(func() error {
			return m.loadFileAt(path, elapsed, paused)
		}); err != nil {
			slog.Warn("mpv: failed to enqueue resume command", "err", err)
		}
	}

	go m.pollUntilReady(newCtx)
}
