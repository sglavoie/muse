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

	// disconnectCh receives the first error when the IPC socket is detected
	// as disconnected by the health-check goroutine. The channel is buffered
	// (size 1) so the goroutine never blocks.
	disconnectCh chan error

	// pingInterval controls how often the health-check goroutine pings mpv.
	// Zero uses the production default (200 ms). Set a shorter value in tests.
	pingInterval time.Duration

	// lastTrackPath and lastElapsed hold the most recently persisted playback
	// position, used to resume playback after an auto-restart.
	lastTrackPath string
	lastElapsed   float64

	cmd    *exec.Cmd
	ctx    context.Context    // scoped to the current mpv subprocess; replaced on restart
	cancel context.CancelFunc // cancels the current subprocess context

	// rootCtx is the manager's lifetime context; it is never replaced.
	// Close() cancels rootCtx which cascades to the current subprocess ctx.
	rootCtx    context.Context
	rootCancel context.CancelFunc

	// launchFn starts the mpv subprocess. Tests replace this with a no-op
	// that returns (nil, nil) while providing their own fake socket server.
	launchFn func(ctx context.Context, socketPath string) (*exec.Cmd, error)
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

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	return &Manager{
		socketPath:   socketPath,
		rootCtx:      rootCtx,
		rootCancel:   rootCancel,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
		launchFn:     defaultLaunch,
	}, nil
}

// defaultLaunch starts the mpv subprocess with --idle flag and IPC server.
func defaultLaunch(ctx context.Context, socketPath string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "mpv",
		"--idle=yes",
		"--input-ipc-server="+socketPath,
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mpv: start: %w", err)
	}
	return cmd, nil
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

// SetCurrentPosition persists the current playback position for use in crash
// recovery. Call this periodically while a track is playing so that
// auto-restart can resume at the correct offset.
func (m *Manager) SetCurrentPosition(path string, elapsed float64) {
	m.mu.Lock()
	m.lastTrackPath = path
	m.lastElapsed = elapsed
	m.mu.Unlock()
}

// DisconnectCh returns a channel that receives the first error surfaced by the
// health-check goroutine when an IPC socket disconnect is detected. The caller
// must not close the channel.
func (m *Manager) DisconnectCh() <-chan error {
	return m.disconnectCh
}

// healthCheck runs in a goroutine after the IPC socket connects. It pings mpv
// at regular intervals; on the first ping failure it marks the client as
// disconnected and forwards the error to disconnectCh.
func (m *Manager) healthCheck(ctx context.Context, client *IPCClient) {
	interval := m.pingInterval
	if interval == 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := client.SendCommand("get_version")
			if err == nil {
				continue
			}

			// Ignore errors that occurred because the manager is shutting down.
			select {
			case <-ctx.Done():
				return
			default:
			}

			m.mu.Lock()
			if m.client == client {
				m.client = nil
				m.cmdQueue = []func() error{}
			}
			m.mu.Unlock()

			select {
			case m.disconnectCh <- err:
			default:
			}
			return
		}
	}
}

// Start launches the mpv subprocess with --idle=yes and --input-ipc-server
// pointing to the socket path, then polls in the background until the socket
// accepts connections.
func (m *Manager) Start() error {
	if err := os.MkdirAll(filepath.Dir(m.socketPath), 0o700); err != nil {
		return fmt.Errorf("mpv: create runtime dir: %w", err)
	}

	launchFn := m.launchFn
	if launchFn == nil {
		launchFn = defaultLaunch
	}

	cmd, err := launchFn(m.ctx, m.socketPath)
	if err != nil {
		return err
	}
	m.cmd = cmd

	m.mu.Lock()
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	go m.pollUntilReady(m.ctx)
	if m.rootCtx != nil {
		go m.watchAndRestart()
	}
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
	if m.rootCancel != nil {
		m.rootCancel()
	} else {
		m.cancel()
	}

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
// or the context is cancelled.
func (m *Manager) pollUntilReady(ctx context.Context) {
	const (
		pollInterval = 50 * time.Millisecond
		timeout      = 10 * time.Second
	)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
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
			go m.healthCheck(ctx, client)
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

// watchAndRestart loops until the manager is closed, restarting mpv whenever
// the health-check goroutine signals a disconnect on disconnectCh.
func (m *Manager) watchAndRestart() {
	for {
		select {
		case <-m.rootCtx.Done():
			return
		case <-m.disconnectCh:
			m.handleRestart()
		}
	}
}

// handleRestart performs one auto-restart cycle: emits a status notification,
// cancels the old subprocess context, launches a new mpv process, queues a
// resume loadfile command, and begins polling for the new socket.
func (m *Manager) handleRestart() {
	m.mu.RLock()
	fn := m.statusFn
	path := m.lastTrackPath
	elapsed := m.lastElapsed
	m.mu.RUnlock()

	if fn != nil {
		fn("mpv disconnected, restarting…")
	}

	// Cancel old subprocess context and create a fresh one derived from rootCtx.
	m.mu.Lock()
	m.cancel()
	newCtx, newCancel := context.WithCancel(m.rootCtx)
	m.ctx = newCtx
	m.cancel = newCancel
	m.cmdQueue = []func() error{} // non-nil = started, allow queueing
	m.mu.Unlock()

	// Remove any stale socket file left by the crashed process so the new
	// mpv instance can bind to the same path.
	_ = os.Remove(m.socketPath)

	launchFn := m.launchFn
	if launchFn == nil {
		launchFn = defaultLaunch
	}

	newCmd, err := launchFn(newCtx, m.socketPath)
	if err != nil {
		if fn != nil {
			fn("mpv: restart failed: " + err.Error())
		}
		return
	}
	m.cmd = newCmd

	// Queue a loadfile command to resume playback at the last persisted position.
	if path != "" {
		_ = m.enqueue(func() error {
			return m.loadFileAt(path, elapsed)
		})
	}

	go m.pollUntilReady(newCtx)
}

// loadFileAt sends mpv a loadfile command with a start= option to resume
// playback at the given elapsed time (in seconds).
func (m *Manager) loadFileAt(path string, elapsed float64) error {
	c, err := m.ipc()
	if err != nil {
		return err
	}
	_, err = c.SendCommand("loadfile", path, "replace", 0, fmt.Sprintf("start=%g", elapsed))
	return err
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
