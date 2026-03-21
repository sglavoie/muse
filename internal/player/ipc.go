package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// ipcReadTimeout is the per-read deadline applied inside SendCommand.
// Using a variable instead of a const allows tests to set a shorter value.
var ipcReadTimeout = 5 * time.Second

// ipcRequest is the JSON envelope sent to mpv over the Unix domain socket.
type ipcRequest struct {
	Command   []any  `json:"command"`
	RequestID uint64 `json:"request_id"`
}

// ipcResponse is the JSON envelope received from mpv.
type ipcResponse struct {
	Error     string         `json:"error"`
	Data      any            `json:"data"`
	RequestID uint64         `json:"request_id"`
	Event     string         `json:"event,omitempty"`
	Raw       map[string]any `json:"-"`
}

// IPCClient is a thin JSON-IPC client for mpv's Unix domain socket protocol.
// Each SendCommand call is synchronous: it writes a request and reads lines
// until it finds the matching response (by request_id), buffering any async
// event lines it encounters along the way.
type IPCClient struct {
	mu      sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
	counter uint64
	closed  bool
}

// NewIPCClient dials the given Unix domain socket and returns a ready client.
func NewIPCClient(socketPath string) (*IPCClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("ipc: dial %s: %w", socketPath, err)
	}
	return &IPCClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Close closes the underlying connection. It sets a closed flag under the
// mutex so that in-flight SendCommand calls see the closure, then closes the
// connection outside the lock to unblock any reader blocked on ReadBytes.
func (c *IPCClient) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.conn.Close()
}

// SendCommand encodes args as an mpv JSON-IPC request, writes it to the
// socket, and returns the parsed response. It blocks until the response whose
// request_id matches the sent request is received.
//
// If mpv returns a non-"success" error field the call returns ErrPlaybackFailed.
func (c *IPCClient) SendCommand(args ...any) (*ipcResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("ipc: connection closed")
	}

	c.counter++
	id := c.counter
	req := ipcRequest{Command: args, RequestID: id}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ipc: marshal: %w", err)
	}
	data = append(data, '\n')
	slog.Debug("ipc send", "msg", strings.TrimRight(string(data), "\n"))

	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("ipc: write: %w", err)
	}

	// Read lines until we find the matching response.
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(ipcReadTimeout)); err != nil {
			return nil, fmt.Errorf("ipc: set deadline: %w", err)
		}
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("ipc: read: %w", err)
		}
		slog.Debug("ipc recv", "msg", strings.TrimRight(string(line), "\n"))

		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			// Malformed line — skip.
			continue
		}

		// Async event: has "event" key but no "request_id".
		if _, isEvent := raw["event"]; isEvent {
			continue
		}

		// Check the request_id. mpv encodes it as a JSON number (float64).
		rid, _ := raw["request_id"].(float64)
		if uint64(rid) != id {
			// Response for a different request — skip (shouldn't happen in
			// single-caller use but is safe to ignore).
			continue
		}

		resp := &ipcResponse{RequestID: id}
		if errVal, ok := raw["error"].(string); ok {
			resp.Error = errVal
		}
		resp.Data = raw["data"]

		if resp.Error != "" && resp.Error != "success" {
			return resp, fmt.Errorf("%w: %s", ErrPlaybackFailed, resp.Error)
		}
		return resp, nil
	}
}
