package player

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

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
	m, f := newUnwiredFakePlaylistManager(t, name)
	m.SetEventCallbacks(noopEventCallbacks())
	return m, f
}

// newUnwiredFakePlaylistManager is newFakePlaylistManager with the callbacks
// left unwired, so startEventReader parks on callbacksReady — after its event
// connection is established and before it adopts the current load.
//
// That park is the only deterministic handle on the interval between the two,
// which is the window in which a live replacement's start-file is already
// observable but not yet accounted for. Call SetEventCallbacks with
// noopEventCallbacks to release it.
func newUnwiredFakePlaylistManager(t *testing.T, name string) (*Manager, *fakePlaylist) {
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
	return m, f
}

// noopEventCallbacks is the callback set the fake-playlist tests install: every
// hook wired, none of them doing anything.
func noopEventCallbacks() EventCallbacks {
	return EventCallbacks{
		OnStatus:        func(string) {},
		OnMissingFile:   func(string) {},
		OnPlaybackEnded: func() {},
		OnTrackChanged:  func(_, _, _, _ string, _ int, _ int64) {},
		OnPauseChanged:  func(bool) {},
	}
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
		OnTrackChanged: func(_, _, _, _ string, _ int, _ int64) {},
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

// prefetchSettleTimeout bounds the wait in assertPrefetch. It matches waitFor's
// deadline: the tracking is committed one lock acquisition after the append mpv
// already answered, so anything approaching this bound is a real failure rather
// than a slow scheduler.
const prefetchSettleTimeout = 2 * time.Second

// assertPrefetch waits until the tracked prefetch points at wantIndex stamped
// with the live playlist generation — the condition the EOF handler requires
// before promoting it — and fails with the last state it observed if that never
// becomes true.
//
// It waits rather than samples because the two facts a test synchronizes on are
// committed in that order, not together: the fake mutates its modelled playlist
// inside handleLoadfile, before the IPC reply is even written, while
// appendPrefetchLocked records the tracking only after SendCommand returns. A
// test that waits on the fake's playlist and then reads the bookkeeping is
// therefore legitimately able to observe the in-between state and see
// prefetchedIndex = -1. The production ordering is deliberate and unchanged —
// the tracking must only ever describe an append mpv has already accepted — so
// the synchronization belongs here.
//
// wantIndex < 0 stays a single sample. Waiting for the tracking to become empty
// would turn "nothing may be prefetched here" into "nothing is prefetched by the
// time something else clears it", which is the assertion those call sites exist
// to make.
func assertPrefetch(t *testing.T, m *Manager, queue []QueueTrack, wantIndex int) {
	t.Helper()
	if wantIndex < 0 {
		if got, _ := prefetchTracking(m); got.index != wantIndex {
			t.Fatalf("prefetchedIndex = %d, want %d", got.index, wantIndex)
		}
		return
	}

	got, gen, ok := waitForPrefetch(m, queue, wantIndex)
	if ok {
		return
	}
	switch {
	case got.index != wantIndex:
		t.Fatalf("prefetchedIndex = %d, want %d after waiting %s for the tracking to commit",
			got.index, wantIndex, prefetchSettleTimeout)
	case got.path != queue[wantIndex].Path:
		t.Fatalf("prefetchedPath = %q, want %q", got.path, queue[wantIndex].Path)
	default:
		t.Fatalf("prefetchedGen = %d, want playlistGen %d", got.gen, gen)
	}
}

// waitForPrefetch polls the manager's prefetch bookkeeping until it names
// wantIndex with the live playlist generation, and returns the last state it
// observed together with whether that ever happened. The caller owns the
// diagnostic, so the last sample is always the one it reports on.
func waitForPrefetch(m *Manager, queue []QueueTrack, wantIndex int) (prefetchState, uint64, bool) {
	deadline := time.Now().Add(prefetchSettleTimeout)
	for {
		got, gen := prefetchTracking(m)
		if got.index == wantIndex && got.path == queue[wantIndex].Path && got.gen == gen {
			return got, gen, true
		}
		if !time.Now().Before(deadline) {
			return got, gen, false
		}
		time.Sleep(2 * time.Millisecond)
	}
}
