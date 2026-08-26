package player

import (
	"bufio"
	"bytes"
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
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	RequestID uint64          `json:"request_id"`
	Event     string          `json:"event,omitempty"`
}

type ipcAsyncEvent struct {
	Event  string          `json:"event"`
	Reason string          `json:"reason,omitempty"`
	Name   string          `json:"name,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

func (r *ipcResponse) isNullData() bool {
	trimmed := bytes.TrimSpace(r.Data)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func (r *ipcResponse) float64Data() (float64, bool, error) {
	if r.isNullData() {
		return 0, false, nil
	}
	var value float64
	if err := json.Unmarshal(r.Data, &value); err != nil {
		return 0, false, fmt.Errorf("expected float64 or nil, got %s", bytes.TrimSpace(r.Data))
	}
	return value, true, nil
}

func (r *ipcResponse) stringData() (string, bool, error) {
	if r.isNullData() {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(r.Data, &value); err != nil {
		return "", false, fmt.Errorf("expected string or nil, got %s", bytes.TrimSpace(r.Data))
	}
	return value, true, nil
}

func (r *ipcResponse) boolData() (bool, bool, error) {
	if r.isNullData() {
		return false, false, nil
	}
	var value bool
	if err := json.Unmarshal(r.Data, &value); err != nil {
		return false, false, fmt.Errorf("expected bool or nil, got %s", bytes.TrimSpace(r.Data))
	}
	return value, true, nil
}

func (r *ipcResponse) formattedData() (string, bool, error) {
	if r.isNullData() {
		return "", false, nil
	}
	trimmed := bytes.TrimSpace(r.Data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		return r.stringData()
	}
	return string(trimmed), true, nil
}

func (e *ipcAsyncEvent) isNullData() bool {
	trimmed := bytes.TrimSpace(e.Data)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func (e *ipcAsyncEvent) boolData() (bool, bool, error) {
	if e.isNullData() {
		return false, false, nil
	}
	var value bool
	if err := json.Unmarshal(e.Data, &value); err != nil {
		return false, false, fmt.Errorf("expected bool or nil, got %s", bytes.TrimSpace(e.Data))
	}
	return value, true, nil
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

// Close closes the underlying connection. It closes the connection first to
// unblock any reader blocked on ReadBytes in SendCommand, then sets the closed
// flag under the mutex so subsequent SendCommand calls return immediately.
// net.Conn.Close is safe to call concurrently with Read/Write.
func (c *IPCClient) Close() error {
	err := c.conn.Close()
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return err
}

// GetPropertyString reads a string property from mpv. The second return value
// reports whether mpv answered with a non-null value; it is false when the
// property exists but is unset. Errors are propagated unchanged, so a caller
// can distinguish an unavailable property (ErrPlaybackFailed) from a transport
// failure (ErrIPCTransport).
func (c *IPCClient) GetPropertyString(name string) (string, bool, error) {
	resp, err := c.SendCommand(cmdGetProperty, name)
	if err != nil {
		return "", false, err
	}
	return resp.stringData()
}

// GetPropertyBool reads a boolean property from mpv. It follows the same
// conventions as GetPropertyString.
func (c *IPCClient) GetPropertyBool(name string) (bool, bool, error) {
	resp, err := c.SendCommand(cmdGetProperty, name)
	if err != nil {
		return false, false, err
	}
	return resp.boolData()
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
		return nil, fmt.Errorf("%w: connection closed", ErrIPCTransport)
	}

	c.counter++
	id := c.counter
	req := ipcRequest{Command: args, RequestID: id}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %w", ErrIPCTransport, err)
	}
	data = append(data, '\n')
	slog.Debug("ipc send", "msg", strings.TrimRight(string(data), "\n"))

	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("%w: write: %w", ErrIPCTransport, err)
	}

	// Read lines until we find the matching response.
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(ipcReadTimeout)); err != nil {
			return nil, fmt.Errorf("%w: set deadline: %w", ErrIPCTransport, err)
		}
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("%w: read: %w", ErrIPCTransport, err)
		}
		slog.Debug("ipc recv", "msg", strings.TrimRight(string(line), "\n"))

		var resp ipcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Malformed line — skip.
			continue
		}

		if resp.Event != "" {
			continue
		}

		if resp.RequestID != id {
			// Response for a different request — skip (shouldn't happen in
			// single-caller use but is safe to ignore).
			continue
		}

		if resp.Error != "" && resp.Error != "success" {
			return &resp, fmt.Errorf("%w: %s", ErrPlaybackFailed, resp.Error)
		}
		return &resp, nil
	}
}
