package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui"
)

// shortTempDir creates a temp dir under /tmp to keep Unix socket paths short
// enough for macOS's 104-byte path limit (t.TempDir paths are too long).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "muse-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
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
