package player

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/config"
)

const mpvSockName = "mpv.sock"

// Compile-time check that Manager implements Player.
var _ Player = (*Manager)(nil)

// Manager manages an mpv subprocess and its JSON-IPC connection.
// It implements the Player interface. Commands issued after Start() but before
// the IPC socket becomes ready are queued and replayed on connect. Commands
// issued before Start() return ErrNotConnected immediately.
type Manager struct {
	socketPath string

	mu       sync.RWMutex
	client   *IPCClient     // nil until the socket is connected
	cmdQueue []func() error // nil = not started; non-nil = started, may have pending cmds
	statusFn func(string)   // optional callback for transient status messages

	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager creates a Manager with a socket path resolved via XDG dirs.
// It removes any stale socket file left by a previously crashed session.
func NewManager() (*Manager, error) {
	dirs, err := config.ResolveDirs()
	if err != nil {
		return nil, fmt.Errorf("mpv: resolve XDG dirs: %w", err)
	}

	socketPath := filepath.Join(dirs.Runtime, mpvSockName)

	if err := cleanStaleSocket(socketPath); err != nil {
		return nil, fmt.Errorf("mpv: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		socketPath: socketPath,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// SocketPath returns the resolved Unix domain socket path.
func (m *Manager) SocketPath() string {
	return m.socketPath
}

// SetStatusFunc sets an optional callback invoked with transient status
// messages (e.g. "Connecting to mpv…"). Call before Start.
func (m *Manager) SetStatusFunc(fn func(string)) {
	m.mu.Lock()
	m.statusFn = fn
	m.mu.Unlock()
}

// Start launches the mpv subprocess with --idle=yes and --input-ipc-server
// pointing to the socket path, then polls in the background until the socket
// accepts connections.
func (m *Manager) Start() error {
	if err := os.MkdirAll(filepath.Dir(m.socketPath), 0o700); err != nil {
		return fmt.Errorf("mpv: create runtime dir: %w", err)
	}

	m.cmd = exec.CommandContext(m.ctx, "mpv",
		"--idle=yes",
		"--input-ipc-server="+m.socketPath,
	)
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("mpv: start: %w", err)
	}

	m.mu.Lock()
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	go m.pollUntilReady()
	return nil
}

// enqueue stores cmd for replay once the IPC socket connects and emits a
// status message. Returns ErrNotConnected if Start has not been called.
func (m *Manager) enqueue(cmd func() error) error {
	m.mu.Lock()
	if m.cmdQueue == nil {
		m.mu.Unlock()
		return ErrNotConnected
	}
	m.cmdQueue = append(m.cmdQueue, cmd)
	fn := m.statusFn
	m.mu.Unlock()

	if fn != nil {
		fn("Connecting to mpv…")
	}
	return nil
}

// Close cancels the mpv subprocess and closes the IPC connection.
func (m *Manager) Close() error {
	m.cancel()

	m.mu.Lock()
	c := m.client
	m.client = nil
	m.mu.Unlock()

	if c != nil {
		_ = c.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Wait()
	}
	return nil
}

// pollUntilReady dials the mpv socket at regular intervals until it succeeds
// or the Manager context is cancelled.
func (m *Manager) pollUntilReady() {
	const (
		pollInterval = 50 * time.Millisecond
		timeout      = 10 * time.Second
	)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		client, err := NewIPCClient(m.socketPath)
		if err == nil {
			m.mu.Lock()
			m.client = client
			queue := m.cmdQueue
			m.cmdQueue = nil
			m.mu.Unlock()

			for _, cmd := range queue {
				_ = cmd()
			}
			return
		}

		time.Sleep(pollInterval)
	}
}

// ipc returns the IPC client if connected, or ErrNotConnected.
func (m *Manager) ipc() (*IPCClient, error) {
	m.mu.RLock()
	c := m.client
	m.mu.RUnlock()
	if c == nil {
		return nil, ErrNotConnected
	}
	return c, nil
}

// Play resumes playback.
func (m *Manager) Play() error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(m.Play)
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("set_property", "pause", false)
	return err
}

// Pause pauses playback.
func (m *Manager) Pause() error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(m.Pause)
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("set_property", "pause", true)
	return err
}

// Stop stops playback.
func (m *Manager) Stop() error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(m.Stop)
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("stop")
	return err
}

// SeekRelative seeks forward (positive) or backward (negative) by seconds.
func (m *Manager) SeekRelative(seconds float64) error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.SeekRelative(seconds) })
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("seek", seconds, "relative")
	return err
}

// SetVolumeRelative adjusts volume by the given percentage delta.
func (m *Manager) SetVolumeRelative(delta float64) error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.SetVolumeRelative(delta) })
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("add", "volume", delta)
	return err
}

// Next advances to the next track in the playlist.
func (m *Manager) Next() error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(m.Next)
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("playlist-next")
	return err
}

// Previous returns to the previous track in the playlist.
func (m *Manager) Previous() error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(m.Previous)
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("playlist-prev")
	return err
}

// cleanStaleSocket removes the socket file at socketPath if it exists but no
// live process is listening on it. Returns an error if a live process owns it.
func cleanStaleSocket(socketPath string) error {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil // no socket file — nothing to do
	}

	// Try to connect: success means a live process owns the socket.
	conn, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("socket %s is already in use by a live mpv instance", socketPath)
	}

	// Connect failed — the socket file is an orphan from a previous crash.
	return os.Remove(socketPath)
}
