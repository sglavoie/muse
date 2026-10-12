package player

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

func (f *fakePlaylist) handlePlaylistRemove(req ipcTestRequest, cmd ipcTestCommand) ipcTestResponse {
	pos, ok := cmd.float64At(1)
	if !ok {
		return errorResponse(req, "playlist-remove: bad index")
	}
	n := int(pos)

	f.mu.Lock()
	defer f.mu.Unlock()
	if n < 0 || n >= len(f.entries) {
		return errorResponse(req, "playlist-remove: index out of range")
	}
	f.entries = append(f.entries[:n], f.entries[n+1:]...)
	switch {
	case n < f.current:
		f.current--
	case n == f.current:
		// mpv stops the removed entry; with --idle=yes the playlist goes idle.
		f.current = -1
	}
	if len(f.entries) == 0 {
		f.current = -1
	}
	return successHandler(req)
}

// finishCurrent models mpv reaching the end of the current file: it emits
// end-file/eof and either transitions gaplessly to the next playlist entry
// (emitting start-file) or leaves the playlist idle.
func (f *fakePlaylist) finishCurrent() {
	var pending []string
	f.mu.Lock()
	if f.current < 0 {
		f.mu.Unlock()
		return
	}
	pending = append(pending, eventLine("end-file", "eof"))
	if next := f.current + 1; next < len(f.entries) {
		f.current = next
		pending = append(pending, eventLine("start-file", ""))
	} else {
		f.current = -1
	}
	f.mu.Unlock()
	f.emit(pending...)
}

// notePathObserved models mpv answering an observe of `path`: besides the
// command reply, it emits the property's current value on the async stream,
// ahead of whatever it emits next. That first property-change is what tells a
// reader which file mpv was playing when its subscription landed.
//
// The value is sampled and queued in the same critical section that raises the
// flag, so a test that waits for the flag knows the event on the stream names
// the file mpv was playing before the wait returned — not one a command released
// afterwards has since loaded.
//
// A hold installed by holdPathSnapshot defers both, to the instant it is
// released: mpv samples the property when it gets round to the subscription, not
// when the subscription was written.
func (f *fakePlaylist) notePathObserved() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pathSnapshotArrived = true
	if f.pathSnapshotHold {
		return
	}
	f.emitPathSnapshotLocked()
}

// emitPathSnapshotLocked puts mpv's answer to the path subscription on the
// stream and marks it delivered.
func (f *fakePlaylist) emitPathSnapshotLocked() {
	f.emitLocked(pathEventLine(f.playingPathLocked()))
	f.pathObserved = true
}

// pathEventLine is the property-change mpv emits for `path`. An idle playlist
// has no current file, and mpv reports that unavailability as null data rather
// than as an empty string.
func pathEventLine(path string) string {
	if path == "" {
		return `{"event":"property-change","id":2,"name":"path","data":null}`
	}
	return fmt.Sprintf(`{"event":"property-change","id":2,"name":"path","data":%q}`, path)
}

// emit pushes raw event lines onto the async stream. Events are dropped when
// nothing is draining them, so a test that never subscribes cannot wedge the
// server behind a full channel.
func (f *fakePlaylist) emit(events ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emitLocked(events...)
}

// emitLocked is emit for a caller that already holds f.mu.
func (f *fakePlaylist) emitLocked(events ...string) {
	if f.dropUnsubscribed && f.subscribers == 0 {
		return
	}
	for _, e := range events {
		select {
		case f.events <- e:
		default:
		}
	}
}

func eventLine(name, reason string) string {
	if reason == "" {
		return fmt.Sprintf(`{"event":%q}`, name)
	}
	return fmt.Sprintf(`{"event":%q,"reason":%q}`, name, reason)
}

// --- test-facing controls and inspection ---

// failCommand makes every command with the given key fail with msg until
// clearFailure is called. Keys come from commandKey ("loadfile:replace",
// "loadfile:append-play", "playlist-remove").
func (f *fakePlaylist) failCommand(key, msg string) {
	f.mu.Lock()
	f.fails[key] = &fakeFailure{message: msg, remaining: -1}
	f.mu.Unlock()
}

// failCommandOnce makes only the next command with the given key fail.
func (f *fakePlaylist) failCommandOnce(key, msg string) {
	f.mu.Lock()
	f.fails[key] = &fakeFailure{message: msg, remaining: 1}
	f.mu.Unlock()
}

func (f *fakePlaylist) clearFailure(key string) {
	f.mu.Lock()
	delete(f.fails, key)
	f.mu.Unlock()
}

// block parks every command with the given key inside the fake until the
// returned gate is released. The gate is also released via t.Cleanup so a failed
// assertion cannot hang the suite.
func (f *fakePlaylist) block(t *testing.T, key string) *fakeGate {
	t.Helper()
	g := &fakeGate{arrived: make(chan struct{}, 8), released: make(chan struct{})}
	f.mu.Lock()
	f.gates[key] = g
	f.mu.Unlock()
	t.Cleanup(g.release)
	return g
}

// blockReply parks the *reply* to every command with the given key, after the
// fake has run it: mpv's playlist already holds the loaded file and its
// start-file has already been emitted, while the client is still blocked in the
// send. That interval is what separates the event from the answer, so a reader
// connecting inside it sees neither.
//
// The gate is also released via t.Cleanup so a failed assertion cannot hang the
// suite.
func (f *fakePlaylist) blockReply(t *testing.T, key string) *fakeGate {
	t.Helper()
	g := &fakeGate{arrived: make(chan struct{}, 8), released: make(chan struct{})}
	f.mu.Lock()
	f.replyGates[key] = g
	f.mu.Unlock()
	t.Cleanup(g.release)
	return g
}

// unblockReply removes the reply barrier for key and releases anything parked on
// it.
func (f *fakePlaylist) unblockReply(key string) {
	f.mu.Lock()
	g := f.replyGates[key]
	delete(f.replyGates, key)
	f.mu.Unlock()
	if g != nil {
		g.release()
	}
}

// holdEvents stops the async event stream at the socket: mpv still runs every
// command it is given and still emits, but nothing reaches the manager's event
// reader until the returned release is called. It models an event socket the
// reader has not drained yet, which is how an entry's end-file survives the
// acknowledgement of the replacement that discarded it.
//
// The release is idempotent and also runs via t.Cleanup, so a failed assertion
// cannot leave the stream parked.
func (f *fakePlaylist) holdEvents(t *testing.T) func() {
	t.Helper()
	hold := make(chan struct{})
	f.mu.Lock()
	f.eventHold = hold
	f.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			f.mu.Lock()
			f.eventHold = nil
			f.mu.Unlock()
			close(hold)
		})
	}
	t.Cleanup(release)
	return release
}

// dropUnsubscribedEvents makes the fake discard the events it emits while no
// client is relaying its async stream, which is how mpv behaves: an event goes
// to the connections that exist when it fires and is gone afterwards. Use it
// for a test whose premise is an event the manager's reader could not have
// observed — one emitted by a load that reached mpv before that reader
// connected — since the buffered stream would otherwise deliver it late.
func (f *fakePlaylist) dropUnsubscribedEvents() {
	f.mu.Lock()
	f.dropUnsubscribed = true
	f.mu.Unlock()
}

// waitForEventStream blocks until the fake is relaying its async stream to a
// connected client and that client's path subscription has been answered, so an
// event emitted after it returns is delivered rather than dropped, and the
// initial path value already on the stream describes mpv as it was before the
// caller went on. The observe-property command reaching the fake is not that
// barrier: the stream is attached only once that command has been answered.
func (f *fakePlaylist) waitForEventStream(t *testing.T) {
	t.Helper()
	f.waitFor(t, "a client to subscribe to the event stream", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.subscribers > 0 && f.pathObserved
	})
}

// holdPathSnapshot keeps mpv from answering the path subscription until the
// returned release is called, while everything else — commands, the events they
// emit, the rest of the stream — keeps running. It models mpv reaching a
// client's observe only after it has executed the loads queued ahead of it, so a
// start-file emitted in between is delivered ahead of the snapshot.
//
// The snapshot names the file mpv is playing at release, which is what mpv
// reports: the property is read when the subscription is processed. The release
// is idempotent and also runs via t.Cleanup, so a failed assertion cannot leave
// a reader waiting on a snapshot that never comes.
func (f *fakePlaylist) holdPathSnapshot(t *testing.T) func() {
	t.Helper()
	f.mu.Lock()
	f.pathSnapshotHold = true
	f.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.pathSnapshotHold = false
			if f.pathSnapshotArrived {
				f.emitPathSnapshotLocked()
			}
		})
	}
	t.Cleanup(release)
	return release
}

// waitForEventSubscriber blocks until the fake is relaying its async stream to a
// connected client and that client's path subscription has reached the fake, so
// an event emitted after it returns is delivered rather than dropped. Unlike
// waitForEventStream it does not require the subscription to have been answered,
// which is the point when the answer is held: the caller wants the window in
// which the reader is connected and the snapshot is still to come.
func (f *fakePlaylist) waitForEventSubscriber(t *testing.T) {
	t.Helper()
	f.waitFor(t, "a client to subscribe to the event stream", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.subscribers > 0 && f.pathSnapshotArrived
	})
}

// unblock removes the barrier for key and releases anything parked on it.
func (f *fakePlaylist) unblock(key string) {
	f.mu.Lock()
	g := f.gates[key]
	delete(f.gates, key)
	f.mu.Unlock()
	if g != nil {
		g.release()
	}
}

// entriesSnapshot returns the modelled playlist contents in order.
func (f *fakePlaylist) entriesSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.entries...)
}

// currentPos returns the playlist position mpv considers current, or -1 when the
// playlist is idle.
func (f *fakePlaylist) currentPos() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

// playingPath returns the path of the current entry, or "" when idle.
func (f *fakePlaylist) playingPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playingPathLocked()
}

// playingPathLocked is playingPath for a caller that already holds f.mu.
func (f *fakePlaylist) playingPathLocked() string {
	if f.current < 0 || f.current >= len(f.entries) {
		return ""
	}
	return f.entries[f.current]
}

// idle reports whether the playlist has no current entry (mpv's idle-active).
func (f *fakePlaylist) idle() bool { return f.currentPos() < 0 }

// commands returns every command the fake has received, in order.
func (f *fakePlaylist) commands() []ipcTestCommand {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ipcTestCommand(nil), f.log...)
}

// count returns how many received commands match pred.
func (f *fakePlaylist) count(pred func(ipcTestCommand) bool) int {
	n := 0
	for _, cmd := range f.commands() {
		if pred(cmd) {
			n++
		}
	}
	return n
}

// waitForEntries polls until the modelled playlist holds exactly want.
func (f *fakePlaylist) waitForEntries(t *testing.T, want []string) {
	t.Helper()
	f.waitFor(t, fmt.Sprintf("playlist %v", want), func() bool {
		return slices.Equal(f.entriesSnapshot(), want)
	})
}

// waitFor polls until pred holds, or fails the test with the modelled state.
func (f *fakePlaylist) waitFor(t *testing.T, desc string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; playlist=%v current=%d commands=%v",
		desc, f.entriesSnapshot(), f.currentPos(), f.commands())
}
