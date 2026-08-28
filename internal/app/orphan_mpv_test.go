package app

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortTestDir returns a directory short enough that a socket path inside it
// fits the 104-character limit macOS imposes on Unix domain socket paths, which
// t.TempDir() paths routinely exceed. It mirrors the helper the app_test package
// uses; this file is in package app because it exercises unexported functions.
func shortTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".orphantest-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestReapOrphanedMPV_SendsQuitToALiveSocket covers the case that motivated the
// reaper: a previous muse crashed, its `mpv --idle=yes` child survived (mpv is
// not killed when its parent dies) and is still listening on muse's IPC socket.
// Removing the socket file alone would leave that process running forever.
func TestReapOrphanedMPV_SendsQuitToALiveSocket(t *testing.T) {
	sock := filepath.Join(shortTestDir(t), "mpv.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		_ = conn.Close()
		// Stand in for mpv exiting on quit: closing the listener makes
		// subsequent dials fail exactly as a dead process would.
		_ = ln.Close()
		received <- string(buf[:n])
	}()

	start := time.Now()
	reapOrphanedMPV(sock)

	select {
	case got := <-received:
		if got != "{\"command\":[\"quit\"]}\n" {
			t.Errorf("sent %q, want the mpv quit IPC command", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the reaper never sent anything to the live socket")
	}

	// It must notice the exit and return promptly rather than burning the full
	// orphanExitTimeout.
	if elapsed := time.Since(start); elapsed >= orphanExitTimeout {
		t.Errorf("reaper took %v, want it to return as soon as the socket goes dead", elapsed)
	}
}

// TestReapOrphanedMPV_NoListenerIsANoop keeps the common path cheap: an ordinary
// stale socket file left by a clean-but-interrupted run has no process behind
// it, and a missing file is not an error either.
func TestReapOrphanedMPV_NoListenerIsANoop(t *testing.T) {
	dir := shortTestDir(t)

	deadSock := filepath.Join(dir, "dead.sock")
	ln, err := net.Listen("unix", deadSock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// Close the listener but leave the socket file behind — the exact shape of
	// a stale socket. Go unlinks the file on Close by default, so opt out.
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if _, err := os.Stat(deadSock); err != nil {
		t.Fatalf("stale socket file missing after close: %v", err)
	}

	for _, path := range []string{deadSock, filepath.Join(dir, "missing.sock"), filepath.Join(dir, "..")} {
		start := time.Now()
		reapOrphanedMPV(path)
		if elapsed := time.Since(start); elapsed > orphanExitTimeout {
			t.Errorf("reapOrphanedMPV(%q) blocked for %v, want a fast no-op", path, elapsed)
		}
	}
}

// TestRemoveStaleSocket_ReapsThenRemoves pins the ordering: the quit must reach
// the orphan while the socket still has a name, since unlinking first would make
// the live process unreachable and permanently unkillable through IPC.
func TestRemoveStaleSocket_ReapsThenRemoves(t *testing.T) {
	sock := filepath.Join(shortTestDir(t), "mpv.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	reached := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		_, _ = conn.Read(buf)
		_ = conn.Close()
		_ = ln.Close()
		close(reached)
	}()

	if err := removeStaleSocket(sock); err != nil {
		t.Fatalf("removeStaleSocket: %v", err)
	}

	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("removeStaleSocket unlinked the socket without reaching the listener first")
	}

	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket file still present after removeStaleSocket (stat err = %v)", err)
	}
}
