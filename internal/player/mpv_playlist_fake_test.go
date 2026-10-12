package player

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// fakePlaylist is a stateful stand-in for mpv's internal playlist, served over a
// real Unix socket so a Manager reaches it through the production IPC client.
//
// Unlike the always-success fake server it models the state the playlist
// transaction layer depends on:
//
//   - `loadfile P replace` empties the playlist and leaves P as its only, current
//     entry — so a prefetched successor is gone without any removal;
//   - `loadfile P append-play` appends P and, when the playlist is idle, starts
//     it, which is what turns a missed EOF into resumed playback instead of a
//     stall;
//   - `loadfile P append` appends without ever starting;
//   - `playlist-remove N` rejects out-of-range indices and shifts the current
//     position the way mpv does;
//   - finishCurrent models mpv reaching the end of a file: it emits end-file/eof
//     and either advances gaplessly to the next entry or goes idle.
//
// It also supports forced per-command failures and deterministic barriers, so a
// test can park one playlist command inside the fake and prove a concurrent one
// cannot interleave with it.
type fakePlaylist struct {
	sock string

	mu         sync.Mutex
	entries    []string
	current    int // playlist position of the current entry; -1 when idle
	log        []ipcTestCommand
	fails      map[string]*fakeFailure
	gates      map[string]*fakeGate
	replyGates map[string]*fakeGate
	// eventHold, while non-nil, parks the async event stream: mpv keeps
	// accepting commands and keeps emitting, but the manager's event reader
	// sees nothing until the hold is released.
	eventHold chan struct{}
	// subscribers counts the connections currently relaying the async event
	// stream, and dropUnsubscribed makes emit discard whatever fires while
	// that count is zero. mpv broadcasts an event to the clients connected at
	// the instant it fires, so a reader that comes up afterwards never sees
	// it; the fake otherwise buffers every event on one channel and would hand
	// that late reader the events emitted before it connected.
	subscribers      int
	dropUnsubscribed bool
	// pathObserved records that a client has subscribed to the path property
	// and the initial value that subscription emits is already on the stream.
	pathObserved bool
	// pathSnapshotHold defers that initial value instead of emitting it with the
	// subscription, and pathSnapshotArrived records that the subscription reached
	// the fake. mpv processes commands in the order it receives them, so a load
	// admitted after a client connects but before mpv gets to that client's
	// observe emits its start-file *ahead* of the snapshot; holding the snapshot
	// is how a test stages that ordering.
	pathSnapshotHold    bool
	pathSnapshotArrived bool

	events chan string
	done   chan struct{}
}

type fakeFailure struct {
	message   string
	remaining int // negative means "fail every time"
}

// fakeGate parks every command matching its key inside the fake until the test
// releases it, giving deterministic interleavings without sleeps.
type fakeGate struct {
	arrived  chan struct{}
	released chan struct{}
	once     sync.Once
}

// waitArrived blocks until a command has reached the gate.
func (g *fakeGate) waitArrived(t *testing.T, desc string) {
	t.Helper()
	select {
	case <-g.arrived:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s to reach the barrier", desc)
	}
}

// release lets every parked and future command through. Safe to call twice.
func (g *fakeGate) release() {
	g.once.Do(func() { close(g.released) })
}

// commandKey names the playlist operation a command performs. It is the key used
// for forced failures and barriers: "loadfile:replace", "loadfile:append-play",
// "playlist-remove", or the bare verb for anything else.
func commandKey(cmd ipcTestCommand) string {
	verb, ok := cmd.stringAt(0)
	if !ok {
		return ""
	}
	if verb == cmdLoadfile {
		if mode, ok := cmd.stringAt(2); ok {
			return verb + ":" + mode
		}
		return verb + ":replace" // mpv's default mode
	}
	return verb
}

// newFakePlaylist starts a fake playlist server on a fresh socket. The listener
// and every accepted connection are torn down via t.Cleanup.
func newFakePlaylist(t *testing.T, name string) *fakePlaylist {
	t.Helper()
	return newFakePlaylistAt(t, shortSockPath(t, name))
}

// newFakePlaylistAt starts a fake playlist server on a caller-supplied socket
// path, so a test can stand a second, empty one up where the first was after
// the restart supervisor has torn the old process down. Everything else matches
// newFakePlaylist.
func newFakePlaylistAt(t *testing.T, sock string) *fakePlaylist {
	t.Helper()

	f := &fakePlaylist{
		sock:       sock,
		current:    -1,
		fails:      make(map[string]*fakeFailure),
		gates:      make(map[string]*fakeGate),
		replyGates: make(map[string]*fakeGate),
		events:     make(chan string, 64),
		done:       make(chan struct{}),
	}

	os.Remove(f.sock)
	ln, err := net.Listen("unix", f.sock)
	if err != nil {
		t.Fatalf("fakePlaylist: listen: %v", err)
	}

	var conns sync.Map
	t.Cleanup(func() {
		close(f.done)
		_ = ln.Close()
		conns.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		os.Remove(f.sock)
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			conns.Store(conn, struct{}{})
			go func(conn net.Conn) {
				defer conn.Close()
				defer conns.Delete(conn)
				f.serve(conn)
			}(conn)
		}
	}()
	return f
}

// serve handles one client connection. A connection that subscribes to a
// property also becomes the async event stream, mirroring mpv, which multiplexes
// replies and events on the same socket.
func (f *fakePlaylist) serve(conn net.Conn) {
	var writeMu sync.Mutex
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	streaming := false
	defer func() {
		if streaming {
			f.mu.Lock()
			f.subscribers--
			f.mu.Unlock()
		}
	}()

	for {
		var req ipcTestRequest
		if err := dec.Decode(&req); err != nil {
			return
		}
		resp := f.handle(req)
		writeMu.Lock()
		err := enc.Encode(resp)
		writeMu.Unlock()
		if err != nil {
			return
		}
		if !req.Command.stringIs(0, cmdObserveProperty) {
			continue
		}
		if !streaming {
			streaming = true
			f.mu.Lock()
			f.subscribers++
			f.mu.Unlock()
			go f.stream(conn, &writeMu)
		}
		if req.Command.stringIs(2, propPath) {
			f.notePathObserved()
		}
	}
}

// stream relays emitted events to a subscribed connection as newline-delimited
// JSON, the shape the Manager's event reader scans.
func (f *fakePlaylist) stream(conn net.Conn, writeMu *sync.Mutex) {
	for {
		select {
		case <-f.done:
			return
		case event := <-f.events:
			if !f.awaitEventRelease() {
				return
			}
			writeMu.Lock()
			_, err := fmt.Fprintln(conn, event)
			writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// awaitEventRelease blocks while a hold is installed, so events emitted during
// the hold reach the reader only afterwards — and in the order they were
// emitted. It reports false when the fake shut down while parked.
func (f *fakePlaylist) awaitEventRelease() bool {
	f.mu.Lock()
	hold := f.eventHold
	f.mu.Unlock()
	if hold == nil {
		return true
	}
	select {
	case <-hold:
		return true
	case <-f.done:
		return false
	}
}

// handle records the command, applies any barrier or forced failure, mutates the
// modelled playlist, and finally waits out any barrier installed on the reply.
func (f *fakePlaylist) handle(req ipcTestRequest) ipcTestResponse {
	cmd := req.Command
	if len(cmd) == 0 {
		return successHandler(req)
	}
	key := commandKey(cmd)

	if !f.record(key, cmd) {
		return errorResponse(req, "shutting down")
	}

	resp := f.perform(req, cmd, key)

	// The reply is the last thing mpv does with a command: it has executed it and
	// emitted whatever the execution produced by now. Parking here therefore
	// models the interval a client spends still waiting on a load mpv has already
	// started playing.
	f.mu.Lock()
	gate := f.replyGates[key]
	f.mu.Unlock()
	if !awaitGate(gate, f.done) {
		return errorResponse(req, "shutting down")
	}
	return resp
}

// perform applies the forced failure for the command's key, if any, and
// otherwise mutates the modelled playlist.
func (f *fakePlaylist) perform(req ipcTestRequest, cmd ipcTestCommand, key string) ipcTestResponse {
	if msg, failed := f.takeFailure(key); failed {
		// mpv rejected the command, so the playlist is left untouched.
		return errorResponse(req, msg)
	}

	switch {
	case cmd.stringIs(0, cmdLoadfile):
		return f.handleLoadfile(req, cmd)
	case cmd.stringIs(0, cmdPlaylistRemove):
		return f.handlePlaylistRemove(req, cmd)
	default:
		return successHandler(req)
	}
}

// record logs the command and waits out any barrier installed for its key. It
// reports false when the fake shut down while the command was parked.
func (f *fakePlaylist) record(key string, cmd ipcTestCommand) bool {
	f.mu.Lock()
	f.log = append(f.log, append(ipcTestCommand(nil), cmd...))
	gate := f.gates[key]
	f.mu.Unlock()

	return awaitGate(gate, f.done)
}

// awaitGate announces a command's arrival at gate and blocks until it is
// released. A nil gate passes straight through; it reports false when done
// closed while the command was parked.
func awaitGate(gate *fakeGate, done <-chan struct{}) bool {
	if gate == nil {
		return true
	}
	select {
	case gate.arrived <- struct{}{}:
	default:
	}
	select {
	case <-gate.released:
		return true
	case <-done:
		return false
	}
}

func (f *fakePlaylist) takeFailure(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fail, ok := f.fails[key]
	if !ok || fail.remaining == 0 {
		return "", false
	}
	if fail.remaining > 0 {
		fail.remaining--
	}
	return fail.message, true
}

func (f *fakePlaylist) handleLoadfile(req ipcTestRequest, cmd ipcTestCommand) ipcTestResponse {
	path, ok := cmd.stringAt(1)
	if !ok {
		return errorResponse(req, "loadfile: missing path")
	}
	mode, ok := cmd.stringAt(2)
	if !ok {
		mode = "replace"
	}

	var pending []string
	f.mu.Lock()
	switch mode {
	case "replace":
		if f.current >= 0 {
			pending = append(pending, eventLine("end-file", "replace"))
		}
		f.entries = []string{path}
		f.current = 0
		pending = append(pending, eventLine("start-file", ""))
	case "append":
		f.entries = append(f.entries, path)
	case "append-play":
		f.entries = append(f.entries, path)
		if f.current < 0 {
			// mpv's append-play starts the appended entry when the playlist has
			// no current entry (idle-active). Plain append would park it instead.
			f.current = len(f.entries) - 1
			pending = append(pending, eventLine("start-file", ""))
		}
	default:
		f.mu.Unlock()
		return errorResponse(req, "loadfile: unsupported mode "+mode)
	}
	f.mu.Unlock()

	f.emit(pending...)
	return successHandler(req)
}
