package player

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
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

	mu      sync.Mutex
	entries []string
	current int // playlist position of the current entry; -1 when idle
	log     []ipcTestCommand
	fails   map[string]*fakeFailure
	gates   map[string]*fakeGate
	// eventHold, while non-nil, parks the async event stream: mpv keeps
	// accepting commands and keeps emitting, but the manager's event reader
	// sees nothing until the hold is released.
	eventHold chan struct{}

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

	f := &fakePlaylist{
		sock:    shortSockPath(t, name),
		current: -1,
		fails:   make(map[string]*fakeFailure),
		gates:   make(map[string]*fakeGate),
		events:  make(chan string, 64),
		done:    make(chan struct{}),
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
		if !streaming && req.Command.stringIs(0, cmdObserveProperty) {
			streaming = true
			go f.stream(conn, &writeMu)
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

// handle records the command, applies any barrier or forced failure, and then
// mutates the modelled playlist.
func (f *fakePlaylist) handle(req ipcTestRequest) ipcTestResponse {
	cmd := req.Command
	if len(cmd) == 0 {
		return successHandler(req)
	}
	key := commandKey(cmd)

	if !f.record(key, cmd) {
		return errorResponse(req, "shutting down")
	}

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
	case <-f.done:
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

// emit pushes raw event lines onto the async stream. Events are dropped when
// nothing is draining them, so a test that never subscribes cannot wedge the
// server behind a full channel.
func (f *fakePlaylist) emit(events ...string) {
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

// subscribe opens an event connection to the fake the way the Manager's event
// reader does and returns the decoded event names ("start-file",
// "end-file:eof", …) in arrival order.
func (f *fakePlaylist) subscribe(t *testing.T) <-chan string {
	t.Helper()
	conn, err := net.Dial("unix", f.sock)
	if err != nil {
		t.Fatalf("subscribe: dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	req, err := json.Marshal(ipcRequest{Command: []any{cmdObserveProperty, 1, propPause}})
	if err != nil {
		t.Fatalf("subscribe: marshal: %v", err)
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		t.Fatalf("subscribe: write: %v", err)
	}

	out := make(chan string, 32)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var ev ipcAsyncEvent
			if json.Unmarshal(scanner.Bytes(), &ev) != nil || ev.Event == "" {
				continue // a command reply, not an async event
			}
			name := ev.Event
			if ev.Reason != "" {
				name += ":" + ev.Reason
			}
			out <- name
		}
	}()
	return out
}

// nextEvent returns the next event name from a subscription, failing on timeout.
func nextEvent(t *testing.T, events <-chan string) string {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("event stream closed")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an event")
		return ""
	}
}

// newFakePlaylistClient returns a raw IPC client connected to the fake, for
// tests that exercise the fake itself rather than a Manager.
func newFakePlaylistClient(t *testing.T, f *fakePlaylist) *IPCClient {
	t.Helper()
	client, err := NewIPCClient(f.sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// mustSend sends a command and fails the test if the fake rejects it.
func mustSend(t *testing.T, c *IPCClient, args ...any) {
	t.Helper()
	if _, err := c.SendCommand(args...); err != nil {
		t.Fatalf("SendCommand%v: %v", args, err)
	}
}

// newFakePlaylistManager wires a Manager to a fake playlist through a real IPC
// client, with no-op callbacks installed so the event reader can run.
func newFakePlaylistManager(t *testing.T, name string) (*Manager, *fakePlaylist) {
	t.Helper()
	f := newFakePlaylist(t, name)

	m := NewManager(f.sock)
	m.launchFn = func(context.Context, string) (*exec.Cmd, error) { return nil, nil }
	t.Cleanup(func() { _ = m.Close() })

	client, err := NewIPCClient(f.sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	m.SetEventCallbacks(EventCallbacks{
		OnStatus:        func(string) {},
		OnMissingFile:   func(string) {},
		OnPlaybackEnded: func() {},
		OnTrackChanged:  func(_, _, _, _ string, _ int) {},
		OnPauseChanged:  func(bool) {},
	})
	return m, f
}

// runEventReader starts the manager's event reader against the fake and waits
// until mpv has seen its pause subscription, so a test can call finishCurrent
// knowing the resulting events are observed.
//
// The fake fans its event stream out to a single consumer at a time: do not also
// call subscribe while the reader is running, or the events are split between
// them.
func runEventReader(t *testing.T, m *Manager, f *fakePlaylist) {
	t.Helper()
	go m.startEventReader(m.ctx, nil)
	f.waitFor(t, "the event reader to observe the pause property", func() bool {
		return f.count(func(cmd ipcTestCommand) bool {
			return cmd.stringIs(0, cmdObserveProperty)
		}) > 0
	})
}

// playbackEndedSignal rewires the manager's callbacks so the test can observe
// OnPlaybackEnded. The returned channel is buffered, so a fire that nothing is
// waiting on is still recorded.
func playbackEndedSignal(m *Manager) <-chan struct{} {
	ended := make(chan struct{}, 4)
	m.SetEventCallbacks(EventCallbacks{
		OnStatus:      func(string) {},
		OnMissingFile: func(string) {},
		OnPlaybackEnded: func() {
			select {
			case ended <- struct{}{}:
			default:
			}
		},
		OnTrackChanged: func(_, _, _, _ string, _ int) {},
		OnPauseChanged: func(bool) {},
	})
	return ended
}

// commandOrder returns the position of the first received command matching pred,
// or -1 when none does. Use it to assert the order two playlist operations
// reached mpv in.
func (f *fakePlaylist) commandOrder(pred func(ipcTestCommand) bool) int {
	for i, cmd := range f.commands() {
		if pred(cmd) {
			return i
		}
	}
	return -1
}

// isAppendOf matches the gapless prefetch append of a specific file.
func isAppendOf(path string) func(ipcTestCommand) bool {
	return func(cmd ipcTestCommand) bool {
		got, _ := cmd.stringAt(1)
		return isLoadfileAppend(cmd) && got == path
	}
}

// queueOf builds a queue of real temp files with sequential queue IDs.
func queueOf(t *testing.T, n int) []QueueTrack {
	t.Helper()
	dir := t.TempDir()
	q := make([]QueueTrack, n)
	for i := range q {
		path := fmt.Sprintf("%s/t%d.flac", dir, i)
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("write queue file: %v", err)
		}
		q[i] = QueueTrack{Path: path, Hash: fmt.Sprintf("h%d", i), QueueID: int64(i + 1)}
	}
	return q
}

// installQueue seeds the manager's queue and current index directly, bypassing
// the asynchronous prefetch SetQueue would schedule.
func installQueue(m *Manager, queue []QueueTrack, current int, gapless bool) {
	m.mu.Lock()
	m.queue = queue
	m.currentIndex = current
	m.gaplessEnabled = gapless
	m.mu.Unlock()
}

// paths projects the queue onto the file paths at the given indices, in order.
func paths(queue []QueueTrack, indices ...int) []string {
	out := make([]string, 0, len(indices))
	for _, i := range indices {
		out = append(out, queue[i].Path)
	}
	return out
}

// prefetchTracking reads the manager's prefetch bookkeeping under the lock,
// together with the playlist generation it must match to remain promotable.
func prefetchTracking(m *Manager) (prefetchState, uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prefetchSnapshotLocked(), m.playlistGen
}

// assertPrefetch checks the tracked prefetch points at wantIndex and is stamped
// with the live playlist generation — the condition the EOF handler requires
// before promoting it.
func assertPrefetch(t *testing.T, m *Manager, queue []QueueTrack, wantIndex int) {
	t.Helper()
	got, gen := prefetchTracking(m)
	if got.index != wantIndex {
		t.Fatalf("prefetchedIndex = %d, want %d", got.index, wantIndex)
	}
	if wantIndex < 0 {
		return
	}
	if got.path != queue[wantIndex].Path {
		t.Fatalf("prefetchedPath = %q, want %q", got.path, queue[wantIndex].Path)
	}
	if got.gen != gen {
		t.Fatalf("prefetchedGen = %d, want playlistGen %d", got.gen, gen)
	}
}
