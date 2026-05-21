package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui"
)

var shortTempDirCounter uint64

// shortTempDir creates a short relative temp dir inside the worktree so Unix
// socket paths stay below macOS's 104-byte limit.
func shortTempDir(t *testing.T) string {
	t.Helper()

	base := ".socket-tests"
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(base, strconv.FormatUint(atomic.AddUint64(&shortTempDirCounter, 1), 10))
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
		_ = os.Remove(base)
	})
	return dir
}

// noopModel is a minimal bubbletea model for remote tests. It records received
// RemoteActionMsgs and quits after the first one.
type noopModel struct {
	received chan tui.RemoteActionMsg
}

func (m noopModel) Init() tea.Cmd { return nil }

func (m noopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if r, ok := msg.(tui.RemoteActionMsg); ok {
		select {
		case m.received <- r:
		default:
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m noopModel) View() tea.View { return tea.NewView("") }

// newTestProgram creates a headless bubbletea program for socket tests.
func newTestProgram(received chan tui.RemoteActionMsg) *tea.Program {
	return tea.NewProgram(
		noopModel{received: received},
		tea.WithoutRenderer(),
		tea.WithInput(strings.NewReader("")),
		tea.WithoutSignals(),
	)
}

func TestCommandListener_ValidAction(t *testing.T) {
	dir := shortTempDir(t)
	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := net.DialTimeout("unix", filepath.Join(dir, ctrlSocketName), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintln(conn, "toggle-play")
	conn.Close()

	select {
	case msg := <-received:
		if msg.ActionName != "toggle-play" {
			t.Errorf("got action %q, want %q", msg.ActionName, "toggle-play")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RemoteActionMsg")
	}
}

func TestCommandListener_CreatesMissingRuntimeDir(t *testing.T) {
	runtimeDir := filepath.Join(shortTempDir(t), "runtime", "muse")
	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := net.DialTimeout("unix", filepath.Join(runtimeDir, ctrlSocketName), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintln(conn, "next")
	conn.Close()

	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("got action %q, want %q", msg.ActionName, "next")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RemoteActionMsg from newly created runtime dir")
	}
}

func TestCommandListener_CloseRemovesSocketFile(t *testing.T) {
	dir := shortTempDir(t)
	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err != nil {
		t.Fatal(err)
	}

	sockPath := filepath.Join(dir, ctrlSocketName)
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("stat socket before close: %v", err)
	}

	if err := ln.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket file should be removed on close, got err=%v", err)
	}
}

func TestCommandListener_UnknownAction(t *testing.T) {
	dir := shortTempDir(t)
	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := net.DialTimeout("unix", filepath.Join(dir, ctrlSocketName), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintln(conn, "not-a-real-action")
	conn.Close()

	// Unknown actions must not be dispatched to the program.
	select {
	case msg := <-received:
		t.Errorf("unexpected action dispatched: %q", msg.ActionName)
	case <-time.After(200 * time.Millisecond):
		// success: nothing dispatched
	}
}

func TestCommandListener_EmptyLine(t *testing.T) {
	dir := shortTempDir(t)
	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := net.DialTimeout("unix", filepath.Join(dir, ctrlSocketName), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Several blank lines followed by a valid action.
	_, _ = fmt.Fprintln(conn, "")
	_, _ = fmt.Fprintln(conn, "   ")
	_, _ = fmt.Fprintln(conn, "next")
	conn.Close()

	// The valid action after the empty lines must still be dispatched.
	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("got action %q, want %q", msg.ActionName, "next")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RemoteActionMsg after empty lines")
	}
}

// fakeListener wraps a real net.Listener and injects transient errors on the
// first errCount calls to Accept before delegating to the real listener.
type fakeListener struct {
	real     net.Listener
	errCount int
}

func (f *fakeListener) Accept() (net.Conn, error) {
	if f.errCount > 0 {
		f.errCount--
		return nil, errors.New("transient accept error")
	}
	return f.real.Accept()
}

func (f *fakeListener) Close() error   { return f.real.Close() }
func (f *fakeListener) Addr() net.Addr { return f.real.Addr() }

func TestCommandListener_TransientAcceptError(t *testing.T) {
	sockPath := filepath.Join(shortTempDir(t), "test.sock")
	realLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer realLn.Close()

	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	// Inject two transient errors before the loop sees a real connection.
	fake := &fakeListener{real: realLn, errCount: 2}
	go acceptLoop(p, fake)

	// The loop must survive the injected errors and still accept connections.
	conn, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		t.Fatal("accept loop stopped after transient errors:", err)
	}
	_, _ = fmt.Fprintln(conn, "next")
	conn.Close()

	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("got %q, want %q", msg.ActionName, "next")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out: accept loop did not survive transient errors")
	}
}

// alwaysErrListener is a net.Listener whose Accept always returns an error,
// simulating a permanently broken socket (e.g. deleted socket file).
type alwaysErrListener struct{ addr net.Addr }

func (a *alwaysErrListener) Accept() (net.Conn, error) {
	return nil, errors.New("permanent accept error")
}
func (a *alwaysErrListener) Close() error   { return nil }
func (a *alwaysErrListener) Addr() net.Addr { return a.addr }

func TestAcceptLoop_CircuitBreaker(t *testing.T) {
	// Use a real Unix addr so Addr() returns something non-nil.
	sockPath := filepath.Join(shortTempDir(t), "circuit.sock")
	realLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	addr := realLn.Addr()
	realLn.Close()

	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	done := make(chan struct{})
	go func() {
		// Use zero backoff so the test completes quickly.
		acceptLoopWithBackoff(p, &alwaysErrListener{addr: addr}, 0)
		close(done)
	}()

	select {
	case <-done:
		// acceptLoop exited after maxConsecutiveAcceptErrs — success.
	case <-time.After(5 * time.Second):
		t.Fatal("acceptLoop did not exit after reaching the circuit-breaker limit")
	}
}

func TestRunRemoteAction(t *testing.T) {
	// Arrange the runtime dir so that startCommandListener and runRemoteAction
	// agree on the socket path. config.ResolveRuntimeDir() returns
	// $XDG_RUNTIME_DIR/muse, so we set XDG_RUNTIME_DIR=baseDir and pass
	// baseDir/muse as runtimeDir to startCommandListener.
	baseDir := shortTempDir(t)
	runtimeDir := filepath.Join(baseDir, "muse")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", baseDir)

	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if err := runRemoteAction("next"); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("got %q, want %q", msg.ActionName, "next")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote action")
	}
}

func TestRunRemoteAction_UnknownAction(t *testing.T) {
	// Set XDG_RUNTIME_DIR to a temp dir with no server running.
	// Before the fix, runRemoteAction would attempt a network connection and
	// return a misleading "connect to muse ... (is muse running?)" error.
	// After the fix, it fails immediately with a clear "unknown action" error
	// without touching the network.
	baseDir := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", baseDir)

	err := runRemoteAction("not-a-real-action")
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("expected error mentioning unknown action, got: %v", err)
	}
}

func TestRunRemoteAction_NoAck(t *testing.T) {
	baseDir := shortTempDir(t)
	runtimeDir := filepath.Join(baseDir, "muse")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", baseDir)

	sockPath := filepath.Join(runtimeDir, ctrlSocketName)
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		if got := strings.TrimSpace(line); got != "next" {
			serverErr <- fmt.Errorf("got %q, want %q", got, "next")
			return
		}
		serverErr <- nil
	}()

	err = runRemoteAction("next")
	if err == nil {
		t.Fatal("expected error when listener closes without acknowledgment")
	}
	if !strings.Contains(err.Error(), "no acknowledgment received") {
		t.Fatalf("expected missing acknowledgment error, got %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote listener to receive action")
	}
}

// TestCommandListener_MultipleCommandsRefreshDeadline verifies that the read
// deadline is refreshed after each successful command so a connection that
// is idle for a moment between commands is not cut off by the original
// connection-wide timeout. Before the fix the deadline was set once when the
// connection opened, so any command sent more than remoteReadTimeout after
// the first byte arrived would be silently dropped on the server side.
//
// The test sends three commands separated by gaps shorter than the timeout
// but whose total elapsed time exceeds it. We verify each ack on the wire,
// which is the only state observable from the client without disturbing the
// listener — the noopModel used elsewhere quits after the first dispatch and
// therefore cannot prove multi-command delivery.
func TestCommandListener_MultipleCommandsRefreshDeadline(t *testing.T) {
	dir := shortTempDir(t)
	// counterModel records every received action without quitting so the
	// listener stays alive for the duration of the test.
	received := make(chan tui.RemoteActionMsg, 8)
	p := tea.NewProgram(
		multiActionModel{received: received},
		tea.WithoutRenderer(),
		tea.WithInput(strings.NewReader("")),
		tea.WithoutSignals(),
	)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	conn, err := net.DialTimeout("unix", filepath.Join(dir, ctrlSocketName), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	const (
		gap      = 3 * time.Second
		commands = 3
	)
	if gap >= remoteReadTimeout {
		t.Fatalf("test invariant: gap (%v) must be less than remoteReadTimeout (%v)", gap, remoteReadTimeout)
	}
	if (commands-1)*gap <= remoteReadTimeout {
		t.Fatalf("test invariant: total elapsed (%v) must exceed remoteReadTimeout (%v) so this test detects a missing refresh", (commands-1)*gap, remoteReadTimeout)
	}

	for i := range commands {
		if i > 0 {
			time.Sleep(gap)
		}
		if _, err := fmt.Fprintln(conn, "next"); err != nil {
			t.Fatalf("write action %d: %v", i, err)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read ack %d: %v", i, err)
		}
		if strings.TrimSpace(line) != "ok" {
			t.Fatalf("ack %d = %q, want ok", i, strings.TrimSpace(line))
		}
	}

	// Sanity check: at least one RemoteActionMsg should have been dispatched.
	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("first dispatched action = %q, want next", msg.ActionName)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no RemoteActionMsg dispatched at all")
	}
}

// multiActionModel records every RemoteActionMsg it receives but never quits,
// so the bubbletea program stays alive across multiple commands.
type multiActionModel struct {
	received chan tui.RemoteActionMsg
}

func (m multiActionModel) Init() tea.Cmd { return nil }
func (m multiActionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if r, ok := msg.(tui.RemoteActionMsg); ok {
		select {
		case m.received <- r:
		default:
		}
	}
	return m, nil
}
func (m multiActionModel) View() tea.View { return tea.NewView("") }

// TestAcceptLoop_CircuitBreakerClosesListener verifies that when the circuit
// breaker trips, the listener is closed so the Unix socket file is removed and
// subsequent dials fail fast instead of hanging against a dead listener.
func TestAcceptLoop_CircuitBreakerClosesListener(t *testing.T) {
	sockPath := filepath.Join(shortTempDir(t), "cb-close.sock")
	realLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	// Wrap with unixSocketListener so Close also removes the socket file.
	ln := &unixSocketListener{Listener: realLn, path: sockPath}

	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	// trackingListener wraps the real listener so Accept always errors (to
	// trigger the breaker) but Close behaviour is observable.
	tracking := &alwaysErrListenerWithClose{inner: ln}

	done := make(chan struct{})
	go func() {
		acceptLoopWithBackoff(p, tracking, 0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("acceptLoop did not exit after circuit breaker")
	}

	if atomic.LoadInt32(&tracking.closeCalls) == 0 {
		t.Error("expected listener.Close() to be invoked when the circuit breaker trips")
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("socket file should be removed after circuit breaker; got stat err=%v", err)
	}
}

// alwaysErrListenerWithClose mirrors alwaysErrListener but tracks Close calls
// and delegates Close to a real listener so socket cleanup can be verified.
type alwaysErrListenerWithClose struct {
	inner      net.Listener
	closeCalls int32
}

func (a *alwaysErrListenerWithClose) Accept() (net.Conn, error) {
	return nil, errors.New("permanent accept error")
}

func (a *alwaysErrListenerWithClose) Close() error {
	atomic.AddInt32(&a.closeCalls, 1)
	return a.inner.Close()
}

func (a *alwaysErrListenerWithClose) Addr() net.Addr {
	return a.inner.Addr()
}

func TestRunRemoteAction_AckDelayWithinReadTimeout(t *testing.T) {
	baseDir := shortTempDir(t)
	runtimeDir := filepath.Join(baseDir, "muse")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", baseDir)

	sockPath := filepath.Join(runtimeDir, ctrlSocketName)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		if got := strings.TrimSpace(line); got != "next" {
			serverErr <- fmt.Errorf("got %q, want %q", got, "next")
			return
		}

		time.Sleep(3 * time.Second)
		if _, err := fmt.Fprintln(conn, "ok"); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	if err := runRemoteAction("next"); err != nil {
		t.Fatalf("expected delayed acknowledgment to succeed, got %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("timed out waiting for remote listener")
	}
}
