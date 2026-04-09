package player

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
)

// startEventReader opens a dedicated read-only connection to the mpv socket
// and dispatches async events. It runs as a goroutine until ctx is cancelled
// or the connection drops. On an end-file event with reason "eof" it calls
// PlayFromIndex(currentIndex+1) to drive auto-advance.
func (m *Manager) startEventReader(ctx context.Context) {
	conn, err := net.Dial("unix", m.socketPath)
	if err != nil {
		return
	}
	defer conn.Close()

	// Close the connection when the context is cancelled so the scanner
	// unblocks and the goroutine exits cleanly.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	// Subscribe to mpv's pause property so we learn about external pause
	// toggles (e.g. AirPods hardware button via macOS media key routing).
	observeCmd, err := json.Marshal(map[string]any{
		"command":    []any{cmdObserveProperty, 1, propPause},
		"request_id": 0,
	})
	if err != nil {
		slog.Warn("mpv: event reader: failed to marshal observe command", "err", err)
		return
	}
	observeCmd = append(observeCmd, '\n')
	if _, err := conn.Write(observeCmd); err != nil {
		slog.Warn("mpv: event reader: failed to observe pause property", "err", err)
		return
	}

	// Block until all callbacks are wired so events are never dispatched to
	// nil function pointers.
	select {
	case <-m.callbacksReady:
	case <-ctx.Done():
		return
	}

	// Emit the current track info immediately so the TUI gets a
	// TrackChangedMsg even when the start-file event fired before this
	// reader connected (race during reconnect / session restore).
	m.mu.RLock()
	idx := m.currentIndex
	q := m.queue
	fn := m.callbacks.OnTrackChanged
	m.mu.RUnlock()
	if fn != nil && idx >= 0 && idx < len(q) {
		t := q[idx]
		fn(t.Hash, t.Artist, t.Title, t.Source, idx)
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var raw map[string]any
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		event, _ := raw["event"].(string)
		switch event {
		case "start-file":
			// Immediately sync the position so that a crash right after track
			// start resumes at the correct file (elapsed≈0) rather than the
			// previous track's last persisted position.
			if err := m.SyncPosition(); err != nil {
				slog.Warn("mpv: sync position failed on track start", "err", err)
			}

			// Notify the TUI that the active track has changed. Resolve the
			// current track from the in-memory queue using currentIndex so
			// the TUI can update the status bar and persist the correct hash.
			m.mu.RLock()
			idx := m.currentIndex
			q := m.queue
			fn := m.callbacks.OnTrackChanged
			m.mu.RUnlock()
			if fn != nil && idx >= 0 && idx < len(q) {
				t := q[idx]
				fn(t.Hash, t.Artist, t.Title, t.Source, idx)
			}

			// Safety net: ensure the next track is always prefetched after a
			// file starts. Covers mpv restarts (loadFileAt) and any edge
			// case where prefetch was not triggered elsewhere. Idempotent.
			go m.prefetchNext()
		case "end-file":
			reason, _ := raw["reason"].(string)
			if reason != "eof" {
				// "stop", "replace", "quit", "error" — do not auto-advance.
				continue
			}
			m.mu.Lock()
			stopAfter := m.stopAfterCurrent
			if stopAfter {
				m.stopAfterCurrent = false
			}
			idx := m.currentIndex
			prefetchedIdx := m.prefetchedIndex
			m.mu.Unlock()
			if stopAfter {
				m.mu.RLock()
				fn := m.callbacks.OnPlaybackEnded
				m.mu.RUnlock()
				if fn != nil {
					fn()
				}
				continue
			}

			// If a track was prefetched (any index after the current one),
			// mpv has already started transitioning to it gaplessly.
			// Just advance bookkeeping and prefetch the track after
			// that — do NOT send a new loadfile replace which would
			// destroy the gapless transition. The prefetched index may
			// be > idx+1 when intermediate tracks were missing on disk
			// and skipped by prefetchNext.
			if prefetchedIdx >= 0 && prefetchedIdx > idx {
				m.mu.Lock()
				m.currentIndex = prefetchedIdx
				m.prefetchedIndex = -1
				m.prefetchedPath = ""
				m.mu.Unlock()

				// Remove the completed track (position 0) from mpv's
				// playlist so it stays at 2 entries instead of growing
				// unbounded across gapless transitions.
				go func() {
					if c, err := m.ipc(); err == nil {
						_, _ = c.SendCommand(cmdPlaylistRemove, 0)
					}
				}()

				go m.prefetchNext()
				continue
			}

			// Fallback: no prefetch available, load next track directly.
			if err := m.PlayFromIndex(idx + 1); err != nil {
				// Queue exhausted: notify the TUI so it can reset the toggle.
				m.mu.RLock()
				fn := m.callbacks.OnPlaybackEnded
				m.mu.RUnlock()
				if fn != nil {
					fn()
				}
			}
		case "property-change":
			name, _ := raw["name"].(string)
			if name != propPause {
				continue
			}
			paused, ok := raw["data"].(bool)
			if !ok {
				continue
			}
			m.mu.Lock()
			m.lastPaused = paused
			fn := m.callbacks.OnPauseChanged
			m.mu.Unlock()
			if fn != nil {
				fn(paused)
			}
		}
	}
}
