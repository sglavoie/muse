package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/sglavoie/muse/internal/safego"
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
	// Deliberately not wrapped in safego: a select over two channels plus
	// net.Conn.Close has no reachable panic, and this goroutine's whole purpose
	// is to unblock the scanner below. The event-reader body it guards is
	// wrapped at its spawn site in activateConnection.
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
			safego.Go("player.prefetchNext", m.prefetchNext)
		case "end-file":
			if event.Reason != "eof" {
				// "stop", "replace", "quit", "error" — do not auto-advance.
				continue
			}
			// The whole transition lives in handleEndOfFile: it makes the
			// state decision here, on this goroutine, and hands everything
			// that can block to one ordered background operation.
			m.handleEndOfFile()
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
