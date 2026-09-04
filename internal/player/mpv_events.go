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

// mpv's observe ids for the properties the event reader subscribes to. They
// name the subscription rather than the request, and mpv echoes them back on
// every property-change; the reader keys off the property name, so they only
// have to be distinct.
const (
	observeIDPause = 1
	observeIDPath  = 2
)

// writeObserve subscribes the event connection to one mpv property. mpv answers
// an observe by emitting the property's current value as a property-change on
// the same stream, so the subscription doubles as a snapshot ordered against
// every event that follows it.
func writeObserve(conn net.Conn, id int, property string) error {
	req, err := json.Marshal(ipcRequest{
		Command:   []any{cmdObserveProperty, id, property},
		RequestID: 0,
	})
	if err != nil {
		// Marshalling a static request essentially never fails, but if it does
		// the subscription cannot be established. It is reported like the write
		// failure below so the caller restarts the generation rather than
		// silently running without it.
		return fmt.Errorf("marshal observe %s: %w", property, err)
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("observe %s: %w", property, err)
	}
	return nil
}

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

	// The connection exists, so mpv broadcasts to it every event it emits from
	// here on. Capture the observation boundary now, before anything below can
	// block: the command gate is already open by the time activateConnection
	// spawns this goroutine, so the loads recorded from this point are live
	// replacements whose start-files this reader will read, and the adoption
	// below has to keep their generations rather than retire them along with the
	// prefix that was emitted before this dial.
	obs := m.loadStartBoundary()

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
	if err := writeObserve(conn, observeIDPause, propPause); err != nil {
		slog.Warn("mpv: event reader: failed to observe pause property", "err", err)
		m.handleConnectionFailure(ctx, client, fmt.Errorf("event reader: observe pause: %w", err))
		return
	}
	// Subscribe to path as well, for its initial value rather than for the
	// changes: mpv answers an observe by emitting the property's current value
	// on this stream, ahead of whatever it emits next. That first
	// property-change is the one observable that places the file mpv is playing
	// in the same sequence as the start-files around it, which is what
	// reconcileStartedLoad needs and what a command reply cannot give.
	if err := writeObserve(conn, observeIDPath, propPath); err != nil {
		slog.Warn("mpv: event reader: failed to observe path property", "err", err)
		m.handleConnectionFailure(ctx, client, fmt.Errorf("event reader: observe path: %w", err))
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
	// reader connected (race during reconnect / session restore). That same
	// unobserved start-file is what adoptCurrentLoad accounts for: the
	// end-file correlation below would otherwise still name the load
	// generation the previous reader last saw a file start in. It retires the
	// expectations from before the boundary captured above and keeps the ones
	// belonging to replacements admitted since — this wait on the callbacks is
	// itself part of that window.
	//
	// What adoption cannot settle raises the stream's barrier. An expectation
	// recorded before this dial and still live afterwards may name a start-file
	// mpv broadcast to a socket this reader never had, and only mpv's own report
	// of the file it is playing tells that apart from a start still to come.
	// Until that report is read the stream holds events instead of dispatching
	// them, so a post-dial start cannot consume the older expectation first.
	stream := eventStream{m: m, holdingForPath: m.adoptCurrentLoad(obs)}
	m.emitCurrentTrackChanged()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var event ipcAsyncEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Event == "" {
			continue
		}
		stream.handle(event)
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

// The mpv async events the reader acts on. Everything else mpv emits on this
// stream is ignored.
const (
	eventStartFile      = "start-file"
	eventEndFile        = "end-file"
	eventPropertyChange = "property-change"
)

// eventStream turns one event connection's async events into Manager state
// transitions, in the order mpv emitted them.
//
// It exists for the one ordering the scan loop cannot express by itself. A
// connection that came up while a load was in flight adopts an expectation it
// may or may not have read the start-file for, and only mpv's own report of the
// file it is playing tells those apart. That report is emitted in answer to the
// path subscription, so it arrives behind every event mpv emitted between the
// dial and the subscription taking effect — including a start-file belonging to
// a load issued after the dial, which would consume the older expectation
// positionally and leave both loads stamped with each other's generation. While
// the barrier is up the stream buffers instead of dispatching, and replays once
// the report has settled the adoption.
type eventStream struct {
	m *Manager

	// gaplessStart carries the one fact the previous event settled and the next
	// one needs: whether mpv is continuing into an entry that was already in its
	// playlist. Only this type can settle it, because only here are the two
	// events still adjacent — anything read from the Manager once the start-file
	// is dispatched describes a state a replacement may have moved on from while
	// both events sat buffered on the socket.
	gaplessStart bool

	// holdingForPath is the adoption barrier, raised by adoptCurrentLoad only
	// when an expectation older than this connection outlived it. A stream with
	// nothing left to reconcile never buffers, which is what keeps readers whose
	// mpv never answers a subscription from stalling.
	holdingForPath bool

	// held is what arrived while the barrier was up, in arrival order.
	// heldStarts counts the start-files among them: that count is how
	// reconcileStartedLoad separates the expectations mpv's report already
	// accounts for from the ones this stream is holding the events for.
	held       []ipcAsyncEvent
	heldStarts int
}

// handle takes one decoded event, either dispatching it or holding it behind
// the adoption barrier.
func (s *eventStream) handle(event ipcAsyncEvent) {
	if !s.holdingForPath {
		s.dispatch(event)
		return
	}
	if event.Event == eventPropertyChange && event.Name == propPath {
		s.lowerBarrier(event.stringData())
		return
	}
	if event.Event == eventStartFile {
		s.heldStarts++
	}
	s.held = append(s.held, event)
}

// lowerBarrier settles the adoption against playing — mpv's name for the file it
// is playing, ordered against the events held behind it — and then dispatches
// those events. The barrier never goes back up: every later path
// property-change describes mpv after starts this stream has already dispatched,
// so it no longer says anything about which expectations are still outstanding.
func (s *eventStream) lowerBarrier(playing string) {
	s.m.reconcileStartedLoad(playing, s.heldStarts)

	held := s.held
	s.holdingForPath, s.held, s.heldStarts = false, nil, 0
	for _, event := range held {
		s.dispatch(event)
	}
}

func (s *eventStream) dispatch(event ipcAsyncEvent) {
	switch event.Event {
	case eventStartFile:
		s.startFile()
	case eventEndFile:
		s.endFile(event.Reason)
	case eventPropertyChange:
		s.propertyChange(event)
	}
}

func (s *eventStream) startFile() {
	// Stamp the entry mpv is entering with the generation that emitted this
	// event, before anything below can block. Events are dispatched in the order
	// mpv emitted them, so an end-file dispatched later is always the end of an
	// entry stamped at or before this point: comparing its stamp against the live
	// generation is what tells a genuine transition from one a replacement
	// already superseded.
	s.m.noteFileStarted(s.gaplessStart)
	s.gaplessStart = false

	// Immediately sync the position so that a crash right after track start
	// resumes at the correct file (elapsed≈0) rather than the previous track's
	// last persisted position.
	if err := s.m.SyncPosition(); err != nil {
		slog.Warn("mpv: sync position failed on track start", "err", err)
	}

	// Notify the TUI that the active track has changed. Resolve the current
	// track from the in-memory queue using currentIndex so the TUI can update
	// the status bar and persist the correct hash.
	s.m.emitCurrentTrackChanged()

	// Safety net: ensure the next track is always prefetched after a file
	// starts. Covers mpv restarts (loadFileAt) and any edge case where prefetch
	// was not triggered elsewhere. Idempotent.
	safego.Go("player.prefetchNext", s.m.prefetchNext)
}

func (s *eventStream) endFile(reason string) {
	if reason != "eof" {
		// "stop", "replace", "quit", "error" — do not auto-advance. mpv stopped
		// the entry rather than playing it out, so nothing follows it gaplessly:
		// the next start-file answers a load.
		s.gaplessStart = false
		return
	}
	// The whole transition lives in handleEndOfFile: it makes the state decision
	// here, on the event goroutine, and hands everything that can block to one
	// ordered background operation.
	s.gaplessStart = s.m.handleEndOfFile()
}

// propertyChange handles the subscriptions' ongoing values. propPath is
// deliberately absent: its only use is the initial report the barrier consumes,
// and handle routes that one before dispatch ever sees it.
func (s *eventStream) propertyChange(event ipcAsyncEvent) {
	if event.Name != propPause {
		return
	}
	paused, ok, err := event.boolData()
	if err != nil || !ok {
		return
	}
	s.m.mu.Lock()
	s.m.lastPaused = paused
	fn := s.m.callbacks.OnPauseChanged
	s.m.mu.Unlock()
	if fn != nil {
		fn(paused)
	}
}
