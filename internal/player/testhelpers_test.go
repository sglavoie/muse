package player

import (
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
// It receives the decoded request envelope and returns the response to send back.
type ipcTestCommand []json.RawMessage

func (c ipcTestCommand) String() string {
	data, err := json.Marshal([]json.RawMessage(c))
	if err != nil {
		return fmt.Sprintf("%q", []json.RawMessage(c))
	}
	return string(data)
}

func (c ipcTestCommand) stringAt(index int) (string, bool) {
	if index >= len(c) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(c[index], &value); err != nil {
		return "", false
	}
	return value, true
}

func (c ipcTestCommand) stringIs(index int, want string) bool {
	value, ok := c.stringAt(index)
	return ok && value == want
}

func (c ipcTestCommand) float64At(index int) (float64, bool) {
	if index >= len(c) {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(c[index], &value); err != nil {
		return 0, false
	}
	return value, true
}

func (c ipcTestCommand) boolAt(index int) (bool, bool) {
	if index >= len(c) {
		return false, false
	}
	var value bool
	if err := json.Unmarshal(c[index], &value); err != nil {
		return false, false
	}
	return value, true
}

func (c ipcTestCommand) hasStringContaining(substr string) bool {
	for index := range c {
		value, ok := c.stringAt(index)
		if ok && strings.Contains(value, substr) {
			return true
		}
	}
	return false
}

func recordCommand(mu *sync.Mutex, received *[]ipcTestCommand, cmd ipcTestCommand) {
	if len(cmd) == 0 {
		return
	}
	mu.Lock()
	*received = append(*received, append(ipcTestCommand(nil), cmd...))
	mu.Unlock()
}

type ipcTestRequest struct {
	Command   ipcTestCommand `json:"command"`
	RequestID uint64         `json:"request_id"`
}

type ipcTestResponse struct {
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data,omitempty"`
	RequestID uint64          `json:"request_id"`
}

type responseHandler func(req ipcTestRequest) ipcTestResponse

var ipcNullResponseData = json.RawMessage("null")

// successHandler echoes back a success response with the same request_id.
func successHandler(req ipcTestRequest) ipcTestResponse {
	return ipcTestResponse{
		Error:     "success",
		Data:      ipcNullResponseData,
		RequestID: req.RequestID,
	}
}

// errorHandler returns a factory that always replies with the given mpv error string.
func errorHandler(mpvError string) responseHandler {
	return func(req ipcTestRequest) ipcTestResponse { return errorResponse(req, mpvError) }
}

func successFloatResponse(req ipcTestRequest, value float64) ipcTestResponse {
	resp := successHandler(req)
	resp.Data = json.RawMessage(strconv.AppendFloat(nil, value, 'f', -1, 64))
	return resp
}

func successStringResponse(req ipcTestRequest, value string) ipcTestResponse {
	resp := successHandler(req)
	resp.Data = json.RawMessage(strconv.AppendQuote(nil, value))
	return resp
}

func errorResponse(req ipcTestRequest, mpvError string) ipcTestResponse {
	return ipcTestResponse{
		Error:     mpvError,
		Data:      ipcNullResponseData,
		RequestID: req.RequestID,
	}
}

// fakeServerWithHandler starts a minimal mpv-like JSON-IPC server on a Unix
// domain socket. For every newline-delimited JSON request it receives it calls
// handler and writes back the returned response. The listener is closed and the
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
