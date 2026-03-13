package player

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
)

// shortSockPath returns a short socket path under /tmp to stay within the
// 104-character macOS limit for Unix domain socket paths.
func shortSockPath(t *testing.T, name string) string {
	t.Helper()
	return fmt.Sprintf("/tmp/muse-test-%d-%s.sock", os.Getpid(), name)
}

// fakeServer starts a minimal mpv-like JSON-IPC server on a Unix domain
// socket. For every newline-delimited JSON request it receives it writes back
// a success response carrying the same request_id.
func fakeServer(t *testing.T, socketPath string) {
	t.Helper()

	os.Remove(socketPath) // clean up any stale socket
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("fakeServer: listen: %v", err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(socketPath)
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleConn(conn)
		}
	}()
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req map[string]any
		if err := dec.Decode(&req); err != nil {
			return
		}
		rid := req["request_id"]
		_ = enc.Encode(map[string]any{
			"error":      "success",
			"data":       nil,
			"request_id": rid,
		})
	}
}

func newTestClient(t *testing.T) *IPCClient {
	t.Helper()
	sock := shortSockPath(t, "mpv")
	fakeServer(t, sock)

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestSendCommand_LoadFile(t *testing.T) {
	c := newTestClient(t)
	resp, err := c.SendCommand("loadfile", "/tmp/track.flac", "replace")
	if err != nil {
		t.Fatalf("loadfile: unexpected error: %v", err)
	}
	if resp.Error != "success" {
		t.Fatalf("loadfile: want error=success, got %q", resp.Error)
	}
}

func TestSendCommand_CyclePause(t *testing.T) {
	c := newTestClient(t)
	resp, err := c.SendCommand("cycle", "pause")
	if err != nil {
		t.Fatalf("cycle pause: unexpected error: %v", err)
	}
	if resp.Error != "success" {
		t.Fatalf("cycle pause: want error=success, got %q", resp.Error)
	}
}

func TestSendCommand_Seek(t *testing.T) {
	c := newTestClient(t)

	for _, delta := range []float64{5, -5, 30, -30} {
		resp, err := c.SendCommand("seek", delta, "relative")
		if err != nil {
			t.Fatalf("seek %v: unexpected error: %v", delta, err)
		}
		if resp.Error != "success" {
			t.Fatalf("seek %v: want error=success, got %q", delta, resp.Error)
		}
	}
}

func TestSendCommand_AddVolume(t *testing.T) {
	c := newTestClient(t)

	for _, delta := range []float64{5, -5} {
		resp, err := c.SendCommand("add", "volume", delta)
		if err != nil {
			t.Fatalf("add volume %v: unexpected error: %v", delta, err)
		}
		if resp.Error != "success" {
			t.Fatalf("add volume %v: want error=success, got %q", delta, resp.Error)
		}
	}
}

func TestSendCommand_RequestIDRoundTrip(t *testing.T) {
	c := newTestClient(t)

	// Fire several commands and verify each gets back a valid (success) response.
	commands := [][]any{
		{"loadfile", "/a.flac", "replace"},
		{"cycle", "pause"},
		{"seek", 5.0, "relative"},
		{"add", "volume", 5.0},
	}
	for _, cmd := range commands {
		resp, err := c.SendCommand(cmd...)
		if err != nil {
			t.Errorf("command %v: %v", cmd, err)
			continue
		}
		if resp.Error != "success" {
			t.Errorf("command %v: want success, got %q", cmd, resp.Error)
		}
	}
}

func TestSendCommand_MPVErrorReturnsErrPlaybackFailed(t *testing.T) {
	sock := shortSockPath(t, "mpv-err")

	os.Remove(sock) // clean up any stale socket
	// Server that always returns an error response.
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(sock)
	})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for {
			var req map[string]any
			if err := dec.Decode(&req); err != nil {
				return
			}
			_ = enc.Encode(map[string]any{
				"error":      "invalid parameter",
				"data":       nil,
				"request_id": req["request_id"],
			})
		}
	}()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	defer client.Close()

	_, err = client.SendCommand("seek", 999.0, "absolute")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isErrPlaybackFailed(err) {
		t.Fatalf("expected ErrPlaybackFailed, got %v", err)
	}
}

func isErrPlaybackFailed(err error) bool {
	// Walk the error chain.
	for err != nil {
		if err == ErrPlaybackFailed {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
