package main

import (
	"bufio"
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

func TestCommandListener_DoesNotRemoveNonSocketPath(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, ctrlSocketName)
	const contents = "not a socket"
	if err := os.WriteFile(sockPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err == nil {
		ln.Close()
		t.Fatal("expected startCommandListener to reject a non-socket control path")
	}

	got, readErr := os.ReadFile(sockPath)
	if readErr != nil {
		t.Fatalf("control path should remain in place: %v", readErr)
	}
	if string(got) != contents {
		t.Fatalf("control path contents = %q, want %q", string(got), contents)
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

func TestCommandListener_CloseDoesNotRemoveReplacedNonSocketPath(t *testing.T) {
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
	const contents = "replacement file"
	if err := os.Remove(sockPath); err != nil {
		t.Fatalf("remove socket path before replacement: %v", err)
	}
	if err := os.WriteFile(sockPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write replacement file: %v", err)
	}

	if err := ln.Close(); err != nil && !strings.Contains(err.Error(), "exists and is not a socket") {
		t.Fatalf("Close error = %v, want non-socket error", err)
	}

	got, readErr := os.ReadFile(sockPath)
	if readErr != nil {
		t.Fatalf("replacement file should remain in place: %v", readErr)
	}
	if string(got) != contents {
		t.Fatalf("replacement contents = %q, want %q", string(got), contents)
	}
}

func TestCommandListener_CloseDoesNotRemoveReplacedSocketPath(t *testing.T) {
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
	if err := os.Remove(sockPath); err != nil {
		t.Fatalf("remove socket path before replacement: %v", err)
	}
	replacement, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen on replacement socket: %v", err)
	}
	defer replacement.Close()

	if err := ln.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if info, err := os.Stat(sockPath); err != nil {
		t.Fatalf("replacement socket should remain in place: %v", err)
	} else if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("replacement path mode = %v, want socket", info.Mode())
	}
}

func TestCommandListener_CloseIsIdempotent(t *testing.T) {
	dir := shortTempDir(t)
	received := make(chan tui.RemoteActionMsg, 1)
	p := newTestProgram(received)
	go p.Run() //nolint:errcheck
	defer p.Quit()

	ln, err := startCommandListener(p, dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := ln.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("second Close should ignore already-closed listener and missing socket file: %v", err)
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

	select {
	case msg := <-received:
		t.Errorf("unexpected action dispatched: %q", msg.ActionName)
	case <-time.After(200 * time.Millisecond):
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
	_, _ = fmt.Fprintln(conn, "")
	_, _ = fmt.Fprintln(conn, "   ")
	_, _ = fmt.Fprintln(conn, "next")
	conn.Close()

	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("got action %q, want %q", msg.ActionName, "next")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RemoteActionMsg after empty lines")
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

	select {
	case msg := <-received:
		if msg.ActionName != "next" {
			t.Errorf("first dispatched action = %q, want next", msg.ActionName)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no RemoteActionMsg dispatched at all")
	}
}

// TestStartCommandListener_SocketPermissions verifies that the control socket
// is created with mode 0600 so a permissive umask cannot expose it to other
// users on a multi-user host.
func TestStartCommandListener_SocketPermissions(t *testing.T) {
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

	info, err := os.Stat(filepath.Join(dir, ctrlSocketName))
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	const wantPerm = os.FileMode(0o600)
	if got := info.Mode().Perm(); got != wantPerm {
		t.Errorf("socket permissions = %o, want %o", got, wantPerm)
	}
}
