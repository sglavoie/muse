package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// socketPath is short: a Unix socket path is capped near 104 bytes on macOS,
// which t.TempDir's nested names can exceed.
func socketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "musec")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

func echoDispatch(req Request) <-chan Reply {
	replies := make(chan Reply, 1)
	replies <- Reply{OK: true, Message: req.Op}
	return replies
}

// serve opens and serves a server that the test closes on cleanup.
func serve(t *testing.T, dispatch Dispatch) *Server {
	t.Helper()
	server, err := Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if err := server.Serve(dispatch); err != nil {
		t.Fatal(err)
	}
	return server
}

type client struct {
	conn    net.Conn
	scanner *bufio.Scanner
}

func dial(t *testing.T, path string) *client {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &client{conn: conn, scanner: bufio.NewScanner(conn)}
}

func (c *client) send(t *testing.T, line string) {
	t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
}

func (c *client) next(t *testing.T) map[string]any {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if !c.scanner.Scan() {
		t.Fatalf("no line: %v", c.scanner.Err())
	}
	var got map[string]any
	if err := json.Unmarshal(c.scanner.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", c.scanner.Text(), err)
	}
	return got
}

// expectEOF asserts the server ended the connection.
func (c *client) expectEOF(t *testing.T) {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if c.scanner.Scan() {
		t.Fatalf("expected EOF, got %q", c.scanner.Text())
	}
	if err := c.scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestServerRepliesToEachRequestByID(t *testing.T) {
	server := serve(t, echoDispatch)

	info, err := os.Stat(server.Path())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %o, want 600", perm)
	}

	c := dial(t, server.Path())
	c.send(t, `{"id":7,"op":"next"}`)
	if got := c.next(t); got["reply"] != float64(7) || got["ok"] != true || got["message"] != "next" {
		t.Fatalf("reply = %v", got)
	}

	c.send(t, `not json`)
	if got := c.next(t); got["ok"] != false {
		t.Fatalf("malformed request answered %v", got)
	}
}

func TestServerTimesOutASilentTUI(t *testing.T) {
	server := serve(t, func(Request) <-chan Reply { return make(chan Reply) })
	server.replyTimeout = 20 * time.Millisecond

	c := dial(t, server.Path())
	c.send(t, `{"id":1,"op":"state"}`)
	if got := c.next(t); got["ok"] != false || got["message"] != "muse did not answer in time" {
		t.Fatalf("reply = %v", got)
	}
}

// TestServerReplaysHelloThenState: a new connection hears the hello first, then
// the latest state — never an older one.
func TestServerReplaysHelloThenState(t *testing.T) {
	server := serve(t, echoDispatch)
	server.SetHello(Hello{PID: 42, Version: "dev", Protocol: ProtocolVersion})
	server.Publish(Event{Event: EventState, State: &State{Playback: PlaybackPaused}})
	server.Publish(Event{Event: EventState, State: &State{Playback: PlaybackPlaying}})
	server.Publish(Event{Event: EventNotify, Notify: &Notification{Title: "not replayed"}})

	c := dial(t, server.Path())
	hello := c.next(t)
	if hello["event"] != EventHello || hello["hello"].(map[string]any)["pid"] != float64(42) {
		t.Fatalf("first line = %v, want hello", hello)
	}
	state := c.next(t)
	if state["event"] != EventState || state["state"].(map[string]any)["playback"] != PlaybackPlaying {
		t.Fatalf("second line = %v, want the latest state", state)
	}

	// Nothing else was replayed: the next line is the answer to a request.
	c.send(t, `{"id":2,"op":"state"}`)
	if got := c.next(t); got["reply"] != float64(2) {
		t.Fatalf("third line = %v, want the reply", got)
	}
}

// TestServerPublishBeforeServe: the model publishes state before the program
// exists, so a state published before Serve is still what a host hears first.
func TestServerPublishBeforeServe(t *testing.T) {
	server, err := Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	server.Publish(Event{Event: EventState, State: &State{Playback: PlaybackPlaying, Elapsed: 3}})
	server.SetHello(Hello{PID: 1, Version: "dev", Protocol: ProtocolVersion})
	if err := server.Serve(echoDispatch); err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(echoDispatch); err == nil {
		t.Fatal("a second Serve succeeded")
	}

	c := dial(t, server.Path())
	if got := c.next(t); got["event"] != EventHello {
		t.Fatalf("first line = %v, want hello", got)
	}
	if got := c.next(t); got["event"] != EventState || got["state"].(map[string]any)["elapsed"] != float64(3) {
		t.Fatalf("second line = %v, want the state published before Serve", got)
	}
}

func TestServerPublishesEventsToOpenConnections(t *testing.T) {
	server := serve(t, echoDispatch)

	c := dial(t, server.Path())
	// The connection is registered once the server has answered on it.
	c.send(t, `{"id":1,"op":"state"}`)
	c.next(t)

	server.Publish(Event{Event: EventNotify, Notify: &Notification{Title: "Song", Subtitle: "Artist"}})
	if got := c.next(t); got["event"] != EventNotify || got["notify"].(map[string]any)["title"] != "Song" {
		t.Fatalf("event = %v", got)
	}
}

// TestServerDrainRefusesRequests: after Drain every connection hears quitting,
// requests are refused without reaching the TUI, and a late connection hears
// quitting as part of its replay.
func TestServerDrainRefusesRequests(t *testing.T) {
	dispatched := make(chan Request, 4)
	server := serve(t, func(req Request) <-chan Reply {
		dispatched <- req
		return echoDispatch(req)
	})

	early := dial(t, server.Path())
	early.send(t, `{"id":1,"op":"next"}`)
	early.next(t)
	<-dispatched

	server.Drain()
	server.Drain()
	if got := early.next(t); got["event"] != EventQuitting {
		t.Fatalf("event = %v, want quitting", got)
	}
	early.send(t, `{"id":2,"op":"next"}`)
	if got := early.next(t); got["reply"] != float64(2) || got["ok"] != false || got["message"] != quittingMessage {
		t.Fatalf("reply after Drain = %v", got)
	}
	select {
	case req := <-dispatched:
		t.Fatalf("request %v reached the TUI after Drain", req)
	default:
	}

	late := dial(t, server.Path())
	if got := late.next(t); got["event"] != EventQuitting {
		t.Fatalf("late connection heard %v first, want quitting", got)
	}
}

// TestServerCloseFlushesThenEnds: the quitting line queued by Drain reaches the
// host before Close ends the connection, so EOF follows it.
func TestServerCloseFlushesThenEnds(t *testing.T) {
	server := serve(t, echoDispatch)
	c := dial(t, server.Path())
	c.send(t, `{"id":1,"op":"state"}`)
	c.next(t)

	server.Drain()
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if got := c.next(t); got["event"] != EventQuitting {
		t.Fatalf("event = %v, want quitting", got)
	}
	c.expectEOF(t)

	if err := server.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	if _, err := os.Stat(server.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket still present after Close: %v", err)
	}
	server.Publish(Event{Event: EventState, State: &State{}})
	server.Drain()
}

func TestServerCloseUnblocksBlockedWriter(t *testing.T) {
	server := serve(t, echoDispatch)
	host := dial(t, server.Path())

	var c *conn
	deadline := time.Now().Add(2 * time.Second)
	for c == nil {
		server.mu.Lock()
		for registered := range server.conns {
			c = registered
		}
		server.mu.Unlock()
		if c == nil {
			if time.Now().After(deadline) {
				t.Fatal("connection never registered")
			}
			time.Sleep(time.Millisecond)
		}
	}

	// A line far larger than any socket buffer cannot be written while the
	// host reads nothing, so once the writer takes it off the queue it is
	// blocked in Write until something interrupts it.
	huge := strings.Repeat("x", 8<<20)
	server.Publish(Event{Event: EventState, State: &State{Track: &Track{Title: huge}}})
	for len(c.out) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("writer never took the oversized line")
		}
		time.Sleep(time.Millisecond)
	}
	server.Drain()

	closed := make(chan error, 1)
	start := time.Now()
	go func() { closed <- server.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(flushTimeout + 2*time.Second):
		t.Fatal("Close stayed blocked behind a writer whose host stopped reading")
	}
	if elapsed := time.Since(start); elapsed > flushTimeout+time.Second {
		t.Fatalf("Close took %v, want about %v", elapsed, flushTimeout)
	}

	// Close waits for every reader and writer, so none is left and the
	// connection is gone.
	server.mu.Lock()
	remaining := len(server.conns)
	server.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("%d connections remain after Close", remaining)
	}
	select {
	case <-c.done:
	default:
		t.Fatal("connection not ended after Close")
	}
	_ = host.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.Copy(io.Discard, host.conn); err != nil {
		t.Fatalf("host did not see the connection end: %v", err)
	}
}

func TestServerCloseWithoutServe(t *testing.T) {
	server, err := Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(echoDispatch); err == nil {
		t.Fatal("Serve after Close succeeded")
	}
}

func TestServerServeRequiresDispatch(t *testing.T) {
	server, err := Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()
	if err := server.Serve(nil); err == nil {
		t.Fatal("Serve(nil) succeeded")
	}
}

func TestServerReplacesAStaleSocketButNotAFile(t *testing.T) {
	path := socketPath(t)
	first, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	// A crashed session leaves its socket file; the next one replaces it.
	_ = first.listener.Close()
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skip("this platform removed the socket with its listener")
	}
	second, err := Listen(path)
	if err != nil {
		t.Fatalf("a stale socket blocked the next session: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path); err == nil {
		t.Fatal("Listen replaced a regular file")
	}
}

func TestServerRejectsAnOverlongLine(t *testing.T) {
	server := serve(t, echoDispatch)
	c := dial(t, server.Path())
	big := make([]byte, maxLineBytes+1)
	for i := range big {
		big[i] = 'x'
	}
	go func() { _, _ = c.conn.Write(append(big, '\n')) }()
	// Linux may reset a socket closed with unread input; macOS returns EOF.
	// Either rejects the oversized request, but a timeout or reply must fail.
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if c.scanner.Scan() {
		t.Fatalf("expected connection closure, got %q", c.scanner.Text())
	}
	if err := c.scanner.Err(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("expected EOF or connection reset, got %v", err)
	}
}
