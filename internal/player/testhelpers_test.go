package player

import (
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// shortSockPath returns a short socket path under the real temp directory to
// stay within the 104-character macOS limit for Unix domain socket paths.
func shortSockPath(t *testing.T, name string) string {
	t.Helper()
	dir := os.TempDir()
	if realDir, err := filepath.EvalSymlinks(dir); err == nil {
		dir = realDir
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create temp socket dir: %v", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("muse-test-%d-%s.sock", os.Getpid(), name))
	if len(path) >= 104 {
		t.Fatalf("test socket path too long for Unix domain socket: %q", path)
	}
	return path
}

func assertAudioFilters(t *testing.T, m *Manager, want map[string]string) {
	t.Helper()

	m.mu.RLock()
	defer m.mu.RUnlock()

	if !maps.Equal(m.audioFilters, want) {
		t.Fatalf("audioFilters = %v, want %v", m.audioFilters, want)
	}
}

// responseHandler is called for each request received by the fake server.
// It receives the decoded request envelope and returns the response map to send back.
type ipcTestRequest struct {
	Command   []any  `json:"command"`
	RequestID uint64 `json:"request_id"`
}

type responseHandler func(req ipcTestRequest) map[string]any

// successHandler echoes back a success response with the same request_id.
func successHandler(req ipcTestRequest) map[string]any {
	return map[string]any{
		"error":      "success",
		"data":       nil,
		"request_id": req.RequestID,
	}
}

// errorHandler returns a factory that always replies with the given mpv error string.
func errorHandler(mpvError string) responseHandler {
	return func(req ipcTestRequest) map[string]any {
		return map[string]any{
			"error":      mpvError,
			"data":       nil,
			"request_id": req.RequestID,
		}
	}
}

// fakeServerWithHandler starts a minimal mpv-like JSON-IPC server on a Unix
// domain socket. For every newline-delimited JSON request it receives it calls
// handler and writes back the returned map. The listener is closed and the
// socket file is removed via t.Cleanup.
func fakeServerWithHandler(t *testing.T, socketPath string, handler responseHandler) {
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
			go handleConn(conn, handler)
		}
	}()
}

// fakeServer starts a fake mpv JSON-IPC server that always returns success.
func fakeServer(t *testing.T, socketPath string) {
	t.Helper()
	fakeServerWithHandler(t, socketPath, successHandler)
}

// handleConn reads newline-delimited JSON requests from conn, calls handler
// for each, and writes back the response.
func handleConn(conn net.Conn, handler responseHandler) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req ipcTestRequest
		if err := dec.Decode(&req); err != nil {
			return
		}
		_ = enc.Encode(handler(req))
	}
}

// silentServer starts a fake server that accepts connections but never writes
// anything, simulating a hung mpv process for timeout testing.
func silentServer(t *testing.T, socketPath string) {
	t.Helper()
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("silentServer: listen: %v", err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(socketPath)
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open but never write anything.
			t.Cleanup(func() { conn.Close() })
		}
	}()
}

// newTestClient starts a fake success server and returns a connected IPCClient.
// Both the client and the listener are closed via t.Cleanup.
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
