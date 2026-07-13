package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
)

func (m *Manager) emitCurrentTrackChanged() {
	m.mu.RLock()
	idx := m.currentIndex
	if idx < 0 || idx >= len(m.queue) || m.callbacks.OnTrackChanged == nil {
		m.mu.RUnlock()
		return
	}
	track := m.queue[idx]
	fn := m.callbacks.OnTrackChanged
	m.mu.RUnlock()

	fn(track.Hash, track.Artist, track.Title, track.Source, idx)
}

func (m *Manager) emitPlaybackEnded() {
	m.mu.RLock()
	fn := m.callbacks.OnPlaybackEnded
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// startEventReader opens a dedicated read-only connection to the mpv socket
// and dispatches async events. It runs as a goroutine until ctx is cancelled
// or the connection drops. On an end-file event with reason "eof" it
// dispatches auto-advance work away from the event-reader loop.
//
// client is the command client for this subprocess generation. Any failure
// that ends the event stream while the generation is still active — dial
// failure, observe-property write failure, a scanner error, or an unexpected
// EOF — is treated as a connection failure and routed through
// handleConnectionFailure so the whole mpv generation restarts. Reconnecting
// only the event socket is deliberately avoided: a lost EOF must not leave
// playback state ambiguous while the command socket appears healthy.
func (m *Manager) startEventReader(ctx context.Context, client *IPCClient) {
	conn, err := net.Dial("unix", m.socketPath)
	if err != nil {
		m.handleConnectionFailure(ctx, client, fmt.Errorf("event reader: dial: %w", err))
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
	observeCmd, err := json.Marshal(ipcRequest{
		Command:   []any{cmdObserveProperty, 1, propPause},
		RequestID: 0,
	})
	if err != nil {
		// Marshalling a static request essentially never fails, but if it does
		// the pause subscription cannot be established; treat it like the write
		// failure below so the generation restarts rather than silently
		// running without external pause notifications.
		slog.Warn("mpv: event reader: failed to marshal observe command", "err", err)
		m.handleConnectionFailure(ctx, client, fmt.Errorf("event reader: marshal observe: %w", err))
		return
	}
	observeCmd = append(observeCmd, '\n')
	if _, err := conn.Write(observeCmd); err != nil {
		slog.Warn("mpv: event reader: failed to observe pause property", "err", err)
		m.handleConnectionFailure(ctx, client, fmt.Errorf("event reader: observe pause: %w", err))
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
	m.emitCurrentTrackChanged()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var event ipcAsyncEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Event == "" {
			continue
		}
		switch event.Event {
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
			m.emitCurrentTrackChanged()

			// Safety net: ensure the next track is always prefetched after a
			// file starts. Covers mpv restarts (loadFileAt) and any edge
			// case where prefetch was not triggered elsewhere. Idempotent.
			go m.prefetchNext()
		case "end-file":
			if event.Reason != "eof" {
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
			prefetchedID := m.prefetchedIdentity
			prefetchedPath := m.prefetchedPath
			shuffle := m.shuffleEnabled
			queueLen := len(m.queue)
			m.mu.Unlock()
			if stopAfter {
				m.emitPlaybackEnded()
				continue
			}

			// If a track was prefetched (any index after the current one),
			// mpv has already started transitioning to it gaplessly.
			// Just advance bookkeeping and prefetch the track after
			// that — do NOT send a new loadfile replace which would
			// destroy the gapless transition. The prefetched index may
			// be > idx+1 when intermediate tracks were missing on disk
			// and skipped by prefetchNext.
			//
			// Validate prefetchedIdx against the current queue length:
			// a concurrent SetQueue that shrinks the queue can leave the
			// prefetched index pointing past the queue end (the prefetch
			// state is cleared by SetQueue → clearPrefetch but that runs
			// after m.mu is released, so this handler may observe a
			// stale prefetchedIdx). Falling through to the PlayFromIndex
			// path correctly reports queue exhaustion in that case.
			//
			// The check below is repeated inside the write lock to defeat
			// a TOCTOU race: SetQueue (or any other queue mutation) can
			// run between the read-lock release above and the write-lock
			// acquire below, invalidating the prefetchedIdx/queueLen we
			// captured. Without the re-check, currentIndex can be set to
			// an index past the end of the freshly-replaced queue, which
			// breaks subsequent start-file dispatch (silently drops the
			// callback because idx >= len(q)).
			advanced := false
			if shuffle {
				// Shuffle prefetch commit: the successor was resolved but not
				// consumed, so history is advanced to it now. Re-validate under
				// the write lock — the identity must still resolve to a live row
				// whose path matches what was prefetched — before committing, so
				// a queue mutation between EOF snapshot and commit cannot promote
				// a stale prefetch.
				if prefetchedIdx >= 0 && prefetchedID != "" {
					m.mu.Lock()
					liveIdx := identityIndex(m.queue, prefetchedID)
					valid := m.prefetchedIndex >= 0 &&
						m.prefetchedIdentity == prefetchedID &&
						liveIdx >= 0 && liveIdx < len(m.queue) &&
						m.queue[liveIdx].Path == prefetchedPath
					var skippedMissing []QueueTrack
					marker := m.marker
					if valid {
						committed, skipped := m.commitShufflePrefetchLocked(prefetchedID)
						skippedMissing = skipped
						if committed >= 0 {
							m.currentIndex = committed
							m.prefetchedIndex = -1
							m.prefetchedPath = ""
							m.prefetchedIdentity = ""
							advanced = true
						}
					}
					m.mu.Unlock()
					// Mark the identities the prefetch peek skipped as missing on
					// disk, outside the lock, so gapless traversal produces the
					// same durable markers and callbacks as non-gapless traversal.
					for _, e := range skippedMissing {
						m.markEntryMissing(e, marker)
					}
				}
			} else if prefetchedIdx >= 0 && prefetchedIdx > idx && prefetchedIdx < queueLen {
				m.mu.Lock()
				if m.prefetchedIndex == prefetchedIdx &&
					prefetchedIdx > m.currentIndex &&
					prefetchedIdx < len(m.queue) {
					m.currentIndex = prefetchedIdx
					m.prefetchedIndex = -1
					m.prefetchedPath = ""
					m.prefetchedIdentity = ""
					advanced = true
				}
				m.mu.Unlock()
			}
			if advanced {
				go m.bestEffortUnpause()

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

			// Fallback: no prefetch available, load the next track directly.
			// Use the internal advance (not the public PlayFromIndex) so that
			// under shuffle this walks existing history/unseen order instead of
			// reanchoring the cycle around idx+1.
			go func(nextIdx int) {
				if err := m.advanceAfterCurrent(nextIdx); err != nil {
					// Queue exhausted: notify the TUI so it can reset the toggle.
					m.emitPlaybackEnded()
				}
			}(idx + 1)
		case "property-change":
			if event.Name != propPause {
				continue
			}
			paused, ok, err := event.boolData()
			if err != nil || !ok {
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

	// The scan loop only exits when the event stream ends. If ctx was cancelled
	// (companion goroutine closed conn on restart/shutdown), the failure belongs
	// to a dead generation and handleConnectionFailure ignores it. Otherwise the
	// event socket dropped unexpectedly while the command socket may still look
	// healthy: restart the whole generation so a silently lost EOF cannot leave
	// playback state ambiguous.
	if err := scanner.Err(); err != nil {
		slog.Warn("mpv: event reader: scanner error",
			"err", err, "socket", m.socketPath)
		m.handleConnectionFailure(ctx, client, fmt.Errorf("event reader: scanner: %w", err))
		return
	}
	m.handleConnectionFailure(ctx, client, errors.New("event reader: unexpected EOF on event socket"))
}
