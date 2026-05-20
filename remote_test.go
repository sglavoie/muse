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
