package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/config"
)

const mpvSockName = "mpv.sock"

const (
	defaultPingInterval    = 200 * time.Millisecond
	shutdownTimeout        = 3 * time.Second
	socketPollInterval     = 50 * time.Millisecond
	socketPollTimeout      = 10 * time.Second
	staleSocketDialTimeout = 200 * time.Millisecond
)

// baseFlags are always prepended to the mpv command line. They keep mpv in
// headless audio-only mode so it never registers with the macOS window server
// (which would create a Dock icon and app-switcher entry).
var baseFlags = []string{"--no-video", "--no-terminal", "--really-quiet"}

// Compile-time check that Manager implements Player.
var _ Player = (*Manager)(nil)

// noopMarker is a MissingMarker that silently discards all calls. It is used
// when PlayFromIndex is called before a real MissingMarker has been wired in.
type noopMarker struct{}

func (noopMarker) MarkTrackMissing(_ string) error     { return nil }
func (noopMarker) MarkQueueEntryMissing(_ int64) error { return nil }

// Manager manages an mpv subprocess and its JSON-IPC connection.
// It implements the Player interface. Commands issued after Start() but before
// the IPC socket becomes ready are queued and replayed on connect. Commands
// issued before Start() return ErrNotConnected immediately.
type Manager struct {
	socketPath string
	extraFlags []string // additional CLI flags appended to the mpv command

	mu       sync.RWMutex
	client   *IPCClient     // nil until the socket is connected
	cmdQueue []func() error // nil = not started; non-nil = started, may have pending cmds
	statusFn func(string)   // optional callback for transient status messages

	// queue is the ordered set of tracks available for playback. Set via
	// SetQueue so that PlayFromIndex can resolve an index to a QueueTrack.
	queue []QueueTrack

	// currentIndex is the queue index of the most recently successfully
	// started track. Used by Next/Previous and the event reader for
	// auto-advance.
	currentIndex int

	// marker persists missing-file state to the database. Set via
	// SetMissingMarker. When nil, PlayFromIndex uses a no-op marker.
	marker MissingMarker

	// missingFileFn is called with the file path when a missing track is
	// detected. Used to dispatch MissingTrackMsg into the Bubble Tea program.
	missingFileFn func(string)

	// playbackEndedFn is called when auto-advance exhausts the queue (i.e.
	// PlayFromIndex returns ErrPlaybackFailed after an end-file/eof event).
	// Used to dispatch PlaybackEndedMsg into the Bubble Tea program so that
	// the TUI can reset its play/pause toggle to the correct idle state.
	playbackEndedFn func()

	// trackChangedFn is called when a new track starts (mpv "start-file"
	// event). It receives the content hash, artist, title, and playlist
	// source of the track resolved from the in-memory queue. Used to
	// dispatch TrackChangedMsg into the Bubble Tea program so that the
	// status bar and session hash stay correct during auto-advance.
	trackChangedFn func(hash, artist, title, source string)

	// pauseChangedFn is called when mpv's pause property changes (e.g. via
	// hardware media keys routed by macOS). Used to dispatch PauseChangedMsg
	// into the Bubble Tea program so the TUI stays in sync with mpv's actual
	// pause state.
	pauseChangedFn func(paused bool)

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
// The caller is responsible for removing any stale socket file before calling
// Start; app.Run handles this in step 2 (after the PID lockfile is acquired).
func NewManager() (*Manager, error) {
	dirs, err := config.ResolveDirs()
	if err != nil {
		return nil, fmt.Errorf("mpv: resolve XDG dirs: %w", err)
	}

	socketPath := filepath.Join(dirs.Runtime, mpvSockName)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	m := &Manager{
		socketPath:   socketPath,
		rootCtx:      rootCtx,
		rootCancel:   rootCancel,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
	}
	m.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		m.mu.RLock()
		extra := m.extraFlags
		m.mu.RUnlock()
		args := append([]string{"--idle=yes", "--input-ipc-server=" + socketPath}, baseFlags...)
		args = append(args, extra...)
		cmd := exec.CommandContext(ctx, "mpv", args...)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("mpv: start: %w", err)
		}
		return cmd, nil
	}
	return m, nil
}

// defaultLaunch starts the mpv subprocess with --idle flag and IPC server.
func defaultLaunch(ctx context.Context, socketPath string) (*exec.Cmd, error) {
	args := append([]string{"--idle=yes", "--input-ipc-server=" + socketPath}, baseFlags...)
	cmd := exec.CommandContext(ctx, "mpv", args...)
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

// SetQueue replaces the ordered set of tracks used by PlayFromIndex. Call
// whenever the playback queue changes (e.g. after loading from the database).
func (m *Manager) SetQueue(entries []QueueTrack) {
	m.mu.Lock()
	m.queue = make([]QueueTrack, len(entries))
	copy(m.queue, entries)
	if m.currentIndex >= len(m.queue) {
		if len(m.queue) > 0 {
			m.currentIndex = len(m.queue) - 1
		} else {
			m.currentIndex = 0
		}
	}
	m.mu.Unlock()
}

// SetCurrentIndex sets the current queue index used by Next, Previous, and
// auto-advance. Call this during session restore after SetQueue so that
// navigation starts from the correct position rather than defaulting to 0.
// The index is clamped to [0, len(queue)-1]; if the queue is empty it is
// set to 0.
func (m *Manager) SetCurrentIndex(index int) {
	m.mu.Lock()
	if len(m.queue) == 0 {
		m.currentIndex = 0
	} else if index < 0 {
		m.currentIndex = 0
	} else if index >= len(m.queue) {
		m.currentIndex = len(m.queue) - 1
	} else {
		m.currentIndex = index
	}
	m.mu.Unlock()
}

// AppendToQueue appends a single entry to the in-memory queue used by
// PlayFromIndex. Call this after persisting the entry to the database so that
// the player and TUI queues stay in sync without a full SetQueue replacement.
func (m *Manager) AppendToQueue(entry QueueTrack) {
	m.mu.Lock()
	m.queue = append(m.queue, entry)
	m.mu.Unlock()
}

// RemoveFromQueue removes the queue entry at index from the in-memory queue
// and adjusts currentIndex atomically. If the removed index precedes
// currentIndex, currentIndex is decremented by one so it continues pointing
// at the same track. If the removed index equals currentIndex, currentIndex
// is left unchanged so that the next auto-advance (PlayFromIndex(currentIndex))
// resolves to what was previously index+1. Returns an error if index is
// negative or >= len(m.queue).
func (m *Manager) RemoveFromQueue(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.queue) {
		return fmt.Errorf("index %d out of range", index)
	}

	m.queue = append(m.queue[:index], m.queue[index+1:]...)
	if index < m.currentIndex {
		m.currentIndex--
	}
	// Clamp currentIndex so it never points past the end of the queue.
	// This can happen when the removed entry was at currentIndex and was
	// the last element (e.g. queue=[a,b,c], currentIndex=2, remove index 2
	// → queue=[a,b] but currentIndex is still 2).
	if len(m.queue) > 0 && m.currentIndex >= len(m.queue) {
		m.currentIndex = len(m.queue) - 1
	} else if len(m.queue) == 0 {
		m.currentIndex = 0
	}

	return nil
}

// SetMissingMarker sets the MissingMarker used to persist missing-file state
// to the database. Call before playback begins.
func (m *Manager) SetMissingMarker(marker MissingMarker) {
	m.mu.Lock()
	m.marker = marker
	m.mu.Unlock()
}

// SetMissingFileFunc sets a callback invoked with the file path whenever a
// missing track is detected during PlayQueueEntry. Use this to dispatch a
// MissingTrackMsg into the Bubble Tea program (e.g. p.Send).
func (m *Manager) SetMissingFileFunc(fn func(string)) {
	m.mu.Lock()
	m.missingFileFn = fn
	m.mu.Unlock()
}

// SetPlaybackEndedFunc sets a callback invoked when auto-advance exhausts the
// queue (PlayFromIndex returns ErrPlaybackFailed after an end-file/eof event).
// Use this to dispatch a PlaybackEndedMsg into the Bubble Tea program so the
// TUI space-bar toggle stays synchronized with mpv's actual idle state.
func (m *Manager) SetPlaybackEndedFunc(fn func()) {
	m.mu.Lock()
	m.playbackEndedFn = fn
	m.mu.Unlock()
}

// SetTrackChangedFunc sets a callback invoked when a new track starts playing
// (mpv "start-file" event). The callback receives the content hash, artist,
// title, and playlist source resolved from the in-memory queue at
// currentIndex. Use this to dispatch a TrackChangedMsg into the Bubble Tea
// program so that the status bar and session hash are updated on auto-advance.
func (m *Manager) SetTrackChangedFunc(fn func(hash, artist, title, source string)) {
	m.mu.Lock()
	m.trackChangedFn = fn
	m.mu.Unlock()
}

// SetPauseChangedFunc sets a callback invoked when mpv's pause property changes
// (e.g. via hardware media keys). The callback receives the new pause state.
// Use this to dispatch a PauseChangedMsg into the Bubble Tea program so the TUI
// stays in sync with mpv's actual pause state.
func (m *Manager) SetPauseChangedFunc(fn func(paused bool)) {
	m.mu.Lock()
	m.pauseChangedFn = fn
	m.mu.Unlock()
}

// ElapsedPosition queries mpv for the current playback position ("time-pos")
// and track duration ("duration"), returning them as seconds. Returns
// (0, 0, ErrNotConnected) when mpv is not yet connected, so callers can fall
// back gracefully. Satisfies the elapsedReader interface used by the TUI root
// model.
func (m *Manager) ElapsedPosition() (elapsed, total float64, err error) {
	c, ipcErr := m.ipc()
	if ipcErr != nil {
		return 0, 0, ipcErr
	}

	posResp, err := c.SendCommand("get_property", "time-pos")
	if err != nil {
		return 0, 0, err
	}
	elapsed, ok := posResp.Data.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("mpv: time-pos: expected float64, got %T", posResp.Data)
	}

	durResp, err := c.SendCommand("get_property", "duration")
	if err != nil {
		return 0, 0, err
	}
	total, ok = durResp.Data.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("mpv: duration: expected float64, got %T", durResp.Data)
	}

	return elapsed, total, nil
}

// PlayFromIndex starts playback from the queue entry at index. The queue must
// have been populated via SetQueue. Missing entries are skipped automatically:
// PlayFromIndex iterates forward from index until it finds a playable entry,
// calling missingFileFn for each skipped entry. Returns ErrPlaybackFailed if
// index is out of range or no playable entry exists from index onward.
func (m *Manager) PlayFromIndex(index int) error {
	m.mu.RLock()
	queue := m.queue
	marker := m.marker
	m.mu.RUnlock()

	if index < 0 || index >= len(queue) {
		return fmt.Errorf("%w: index %d out of range [0, %d)", ErrPlaybackFailed, index, len(queue))
	}

	mk := marker
	if mk == nil {
		mk = noopMarker{}
	}

	for i := index; i < len(queue); i++ {
		err := m.PlayQueueEntry(queue[i], mk)
		if err == nil {
			m.mu.Lock()
			m.currentIndex = i
			m.mu.Unlock()
			return nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return err
		}
		// ErrMissingFile: missingFileFn already called by PlayQueueEntry; try next.
	}
	return fmt.Errorf("%w: no playable entry starting at index %d", ErrPlaybackFailed, index)
}

// SyncPosition queries mpv for the current file path and playback position,
// then persists them via SetCurrentPosition for crash-resume. Safe to call
// when mpv is idle — errors are silently ignored.
func (m *Manager) SyncPosition() error {
	c, err := m.ipc()
	if err != nil {
		return err
	}

	pathResp, err := c.SendCommand("get_property", "path")
	if err != nil {
		return err
	}
	path, _ := pathResp.Data.(string)

	posResp, err := c.SendCommand("get_property", "time-pos")
	if err != nil {
		return err
	}
	elapsed, _ := posResp.Data.(float64)

	if path != "" {
		m.SetCurrentPosition(path, elapsed)
	}
	return nil
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
		interval = defaultPingInterval
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

			slog.Info("mpv disconnected", "err", err)
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

	m.mu.Lock()
	m.cmd = cmd
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
	cmd := m.cmd
	m.mu.Unlock()

	if c != nil {
		_ = c.Close()
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			slog.Warn("mpv process did not exit within timeout, killing")
			_ = cmd.Process.Kill()
		}
	}
	return nil
}

// pollUntilReady dials the mpv socket at regular intervals until it succeeds
// or the context is cancelled.
func (m *Manager) pollUntilReady(ctx context.Context) {
	deadline := time.Now().Add(socketPollTimeout)
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

			slog.Info("mpv connected", "socket", m.socketPath)
			for _, cmd := range queue {
				if err := cmd(); err != nil {
					slog.Warn("mpv: queued command failed on replay", "err", err)
				}
			}
			go m.healthCheck(ctx, client)
			go m.startEventReader(ctx)
			return
		}

		time.Sleep(socketPollInterval)
	}
}

// SetSocketPath overrides the socket path used by Start and the event reader.
// Call this before Start if the socket path must match an externally resolved
// value (e.g. from app.Run).
func (m *Manager) SetSocketPath(path string) {
	m.mu.Lock()
	m.socketPath = path
	m.mu.Unlock()
}

// SetExtraFlags sets additional command-line flags appended to the mpv
// subprocess after --idle=yes and --input-ipc-server. Call before Start.
func (m *Manager) SetExtraFlags(flags []string) {
	m.mu.Lock()
	m.extraFlags = make([]string, len(flags))
	copy(m.extraFlags, flags)
	m.mu.Unlock()
}

// startEventReader opens a dedicated read-only connection to the mpv socket
// and dispatches async events. It runs as a goroutine until ctx is cancelled
// or the connection drops. On an end-file event with reason "eof" it calls
// PlayFromIndex(currentIndex+1) to drive auto-advance.
func (m *Manager) startEventReader(ctx context.Context) {
	conn, err := net.Dial("unix", m.socketPath)
	if err != nil {
		return
	}
	defer conn.Close()

	// Close the connection when the context is cancelled so the scanner
	// unblocks and the goroutine exits cleanly.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	// Subscribe to mpv's pause property so we learn about external pause
	// toggles (e.g. AirPods hardware button via macOS media key routing).
	observeCmd, _ := json.Marshal(map[string]any{
		"command":    []any{"observe_property", 1, "pause"},
		"request_id": 0,
	})
	observeCmd = append(observeCmd, '\n')
	if _, err := conn.Write(observeCmd); err != nil {
		slog.Warn("mpv: event reader: failed to observe pause property", "err", err)
		return
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var raw map[string]any
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		event, _ := raw["event"].(string)
		switch event {
		case "start-file":
			// Immediately sync the position so that a crash right after track
			// start resumes at the correct file (elapsed≈0) rather than the
			// previous track's last persisted position.
			if err := m.SyncPosition(); err != nil {
				slog.Warn("mpv: sync position failed on track start", "err", err)
			}

			// Notify the TUI that the active track has changed. Resolve the
			// current track from the in-memory queue using currentIndex so
			// the TUI can update the status bar and persist the correct hash.
			m.mu.RLock()
			idx := m.currentIndex
			q := m.queue
			fn := m.trackChangedFn
			m.mu.RUnlock()
			if fn != nil && idx >= 0 && idx < len(q) {
				t := q[idx]
				fn(t.Hash, t.Artist, t.Title, t.Source)
			}
		case "end-file":
			reason, _ := raw["reason"].(string)
			if reason != "eof" {
				// "stop", "replace", "quit", "error" — do not auto-advance.
				continue
			}
			m.mu.RLock()
			idx := m.currentIndex
			m.mu.RUnlock()
			if err := m.PlayFromIndex(idx + 1); err != nil {
				// Queue exhausted: notify the TUI so it can reset the toggle.
				m.mu.RLock()
				fn := m.playbackEndedFn
				m.mu.RUnlock()
				if fn != nil {
					fn()
				}
			}
		case "property-change":
			name, _ := raw["name"].(string)
			if name != "pause" {
				continue
			}
			paused, ok := raw["data"].(bool)
			if !ok {
				continue
			}
			m.mu.RLock()
			fn := m.pauseChangedFn
			m.mu.RUnlock()
			if fn != nil {
				fn(paused)
			}
		}
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
	if err := os.Remove(m.socketPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("player: remove socket", "path", m.socketPath, "err", err)
	}

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
	m.mu.Lock()
	m.cmd = newCmd
	m.mu.Unlock()

	// Queue a loadfile command to resume playback at the last persisted position.
	if path != "" {
		if err := m.enqueue(func() error {
			return m.loadFileAt(path, elapsed)
		}); err != nil {
			slog.Warn("mpv: failed to enqueue resume command", "err", err)
		}
	}

	go m.pollUntilReady(newCtx)
}

// SetRepeat configures mpv's repeat behaviour. mode is the string form of
// RepeatMode: "none" disables looping, "all" loops the playlist, "one" loops
// the current file. The command is queued for replay if mpv is not yet
// connected.
func (m *Manager) SetRepeat(mode string) error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.SetRepeat(mode) })
	}
	if err != nil {
		return err
	}
	switch mode {
	case "all":
		if _, err = c.SendCommand("set_property", "loop-playlist", "inf"); err != nil {
			return err
		}
		_, err = c.SendCommand("set_property", "loop-file", "no")
	case "one":
		if _, err = c.SendCommand("set_property", "loop-file", "inf"); err != nil {
			return err
		}
		_, err = c.SendCommand("set_property", "loop-playlist", "no")
	default: // "none"
		if _, err = c.SendCommand("set_property", "loop-playlist", "no"); err != nil {
			return err
		}
		_, err = c.SendCommand("set_property", "loop-file", "no")
	}
	return err
}

// SetShuffle enables or disables mpv's shuffle mode. The command is queued
// for replay if mpv is not yet connected.
func (m *Manager) SetShuffle(enabled bool) error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.SetShuffle(enabled) })
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("set_property", "shuffle", enabled)
	return err
}

// LoadFilePaused loads the file at path into mpv, seeks to positionMs
// milliseconds from the start, and leaves mpv in the paused state. Used for
// session restore at startup. If mpv is not yet connected the command is
// queued for replay once the IPC socket connects.
func (m *Manager) LoadFilePaused(path string, positionMs int64) error {
	elapsedSecs := float64(positionMs) / 1000.0
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.LoadFilePaused(path, positionMs) })
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("loadfile", path, "replace", 0, fmt.Sprintf("start=%g,pause=yes", elapsedSecs))
	return err
}

// loadFileAt sends mpv a loadfile command with a start= option to resume
// playback at the given elapsed time (in seconds).
func (m *Manager) loadFileAt(path string, elapsed float64) error {
	c, err := m.ipc()
	if err != nil {
		return err
	}
	_, err = c.SendCommand("loadfile", path, "replace", 0, fmt.Sprintf("start=%g,pause=no", elapsed))
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

// Stop pauses playback and seeks to the beginning of the current track.
func (m *Manager) Stop() error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(m.Stop)
	}
	if err != nil {
		return err
	}
	if _, err = c.SendCommand("set_property", "pause", true); err != nil {
		return err
	}
	_, err = c.SendCommand("seek", 0, "absolute")
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

// SetVolumeAbsolute sets the volume to an absolute level (0–100). The command
// is queued for replay if mpv is not yet connected, so it is safe to call
// immediately after Start before the IPC socket is ready.
func (m *Manager) SetVolumeAbsolute(volume float64) error {
	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.SetVolumeAbsolute(volume) })
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("set_property", "volume", volume)
	return err
}

// Next advances to the next track in the queue using the Go-side index so
// that the Manager (not mpv's internal playlist) owns sequencing.
// Returns ErrNotConnected if Start has not been called.
func (m *Manager) Next() error {
	m.mu.RLock()
	idx := m.currentIndex
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	return m.PlayFromIndex(idx + 1)
}

// Previous returns to the previous track in the queue using the Go-side index.
// Returns ErrNotConnected if Start has not been called.
func (m *Manager) Previous() error {
	m.mu.RLock()
	idx := m.currentIndex
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	return m.PlayFromIndex(idx - 1)
}

// PlayQueueEntry stats the file at entry.Path before loading it into mpv.
// If the file is absent it marks the track and queue entry as missing via
// marker, emits a transient status message, and returns ErrMissingFile without
// touching mpv — the caller should skip to the next queue entry.
// If the file is present it sends a loadfile command to mpv (queued if not yet
// connected).
func (m *Manager) PlayQueueEntry(entry QueueTrack, marker MissingMarker) error {
	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		if err := marker.MarkTrackMissing(entry.Hash); err != nil {
			slog.Error("mpv: mark track missing", "hash", entry.Hash, "err", err)
		}
		if err := marker.MarkQueueEntryMissing(entry.QueueID); err != nil {
			slog.Error("mpv: mark queue entry missing", "queueID", entry.QueueID, "err", err)
		}

		m.mu.RLock()
		fn := m.statusFn
		mfn := m.missingFileFn
		m.mu.RUnlock()
		if fn != nil {
			fn("Missing file: " + entry.Path)
		}
		if mfn != nil {
			mfn(entry.Path)
		}
		return ErrMissingFile
	}

	c, err := m.ipc()
	if errors.Is(err, ErrNotConnected) {
		return m.enqueue(func() error { return m.PlayQueueEntry(entry, marker) })
	}
	if err != nil {
		return err
	}
	_, err = c.SendCommand("loadfile", entry.Path, "replace", 0, "pause=no")
	return err
}

// cleanStaleSocket removes the socket file at socketPath if it exists but no
// live process is listening on it. Returns an error if a live process owns it.
// Note: app.Run already removes any stale socket in step 2 (after acquiring
// the PID lockfile), so this function is not called during normal startup.
// It is retained for tests and for any caller that needs the liveness check.
func cleanStaleSocket(socketPath string) error {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil // no socket file — nothing to do
	}

	// Try to connect: success means a live process owns the socket.
	conn, err := net.DialTimeout("unix", socketPath, staleSocketDialTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("socket %s is already in use by a live mpv instance", socketPath)
	}

	// Connect failed — the socket file is an orphan from a previous crash.
	return os.Remove(socketPath)
}
