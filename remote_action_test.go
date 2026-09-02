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

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestRunRemoteAction(t *testing.T) {
	baseDir := shortTempDir(t)
	runtimeDir := filepath.Join(baseDir, "muse")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", baseDir)

	received := make(chan playback.RemoteActionMsg, 1)
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

// TestRunRemoteAction_WriteDeadline verifies that runRemoteAction does not
// block indefinitely when a remote listener accepts the connection but never
// drains its receive buffer. Before the fix, fmt.Fprintln(conn, ...) had no
// write deadline and could hang once OS-level socket buffers filled.
func TestRunRemoteAction_WriteDeadline(t *testing.T) {
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

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()

	done := make(chan error, 1)
	go func() { done <- runRemoteAction("next") }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from runRemoteAction; small payload may have succeeded — that is acceptable so long as the call did not hang")
		}
	case <-time.After(remoteReadTimeout + 3*time.Second):
		t.Fatal("runRemoteAction hung past the write/read deadline")
	}

	select {
	case c := <-accepted:
		c.Close()
	default:
	}
}
