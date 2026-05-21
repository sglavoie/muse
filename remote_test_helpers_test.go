package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui"
)

var shortTempDirCounter uint64

// shortTempDir creates a short temp dir inside the worktree so Unix socket
// paths stay below macOS's 104-byte limit. The returned path is absolute so
// callers can use it as $XDG_RUNTIME_DIR (the XDG spec — and our xdgEnv
// helper — ignore non-absolute values).
func shortTempDir(t *testing.T) string {
	t.Helper()

	base := ".socket-tests"
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(base, strconv.FormatUint(atomic.AddUint64(&shortTempDirCounter, 1), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
		_ = os.Remove(base)
	})
	return abs
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

// alwaysErrListener is a net.Listener whose Accept always returns an error,
// simulating a permanently broken socket (e.g. deleted socket file).
type alwaysErrListener struct{ addr net.Addr }

func (a *alwaysErrListener) Accept() (net.Conn, error) {
	return nil, errors.New("permanent accept error")
}

func (a *alwaysErrListener) Close() error   { return nil }
func (a *alwaysErrListener) Addr() net.Addr { return a.addr }

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
