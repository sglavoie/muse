package player

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

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

	posResp, err := c.SendCommand(cmdGetProperty, propTimePos)
	if err != nil {
		return 0, 0, err
	}
	elapsed, ok := posResp.Data.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("mpv: time-pos: expected float64, got %T", posResp.Data)
	}

	durResp, err := c.SendCommand(cmdGetProperty, propDuration)
	if err != nil {
		return 0, 0, err
	}
	total, ok = durResp.Data.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("mpv: duration: expected float64, got %T", durResp.Data)
	}

	return elapsed, total, nil
}

// SyncPosition queries mpv for the current file path and playback position,
// then persists them via SetCurrentPosition for crash-resume. Safe to call
// when mpv is idle — errors are silently ignored.
func (m *Manager) SyncPosition() error {
	c, err := m.ipc()
	if err != nil {
		return err
	}

	pathResp, err := c.SendCommand(cmdGetProperty, propPath)
	if err != nil {
		return err
	}
	path, _ := pathResp.Data.(string)

	posResp, err := c.SendCommand(cmdGetProperty, propTimePos)
	if err != nil {
		return err
	}
	elapsed, _ := posResp.Data.(float64)

	if path != "" {
		m.SetCurrentPosition(path, elapsed)
	}
	return nil
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

	if m.launchFn == nil {
		return fmt.Errorf("mpv: launchFn is nil; use NewManager to create a Manager")
	}

	cmd, err := m.launchFn(m.ctx, m.socketPath)
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
	fn := m.callbacks.OnStatus
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
			if err := cmd.Process.Kill(); err != nil {
				slog.Debug("mpv kill after timeout", "err", err)
			}
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
			m.backoffReset.Store(true)
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

	slog.Warn("mpv: socket connection timed out", "socket", m.socketPath, "timeout", socketPollTimeout)
	m.mu.Lock()
	m.cmdQueue = nil
	fn := m.callbacks.OnStatus
	m.mu.Unlock()
	if fn != nil {
		fn("mpv: connection timed out")
	}
}

// watchAndRestart loops until the manager is closed, restarting mpv whenever
// the health-check goroutine signals a disconnect on disconnectCh. It applies
// exponential backoff between consecutive failures and gives up after
// restartMaxAttempts attempts without a successful reconnect.
func (m *Manager) watchAndRestart() {
	initialDelay := m.restartBaseDelay
	if initialDelay == 0 {
		initialDelay = restartInitialDelay
	}

	var consecutiveFailures int
	for {
		select {
		case <-m.rootCtx.Done():
			return
		case <-m.disconnectCh:
			consecutiveFailures++
			if consecutiveFailures > restartMaxAttempts {
				m.mu.RLock()
				fn := m.callbacks.OnStatus
				m.mu.RUnlock()
				if fn != nil {
					fn("mpv: gave up after too many restart attempts")
				}
				return
			}
			if consecutiveFailures > 1 {
				delay := min(initialDelay*time.Duration(1<<(consecutiveFailures-2)), restartMaxDelay)
				select {
				case <-m.rootCtx.Done():
					return
				case <-time.After(delay):
				}
			}
			m.handleRestart()
			if m.backoffReset.Swap(false) {
				consecutiveFailures = 0
			}
		}
	}
}

// handleRestart performs one auto-restart cycle: emits a status notification,
// cancels the old subprocess context, launches a new mpv process, queues a
// resume loadfile command, and begins polling for the new socket.
func (m *Manager) handleRestart() {
	m.mu.RLock()
	fn := m.callbacks.OnStatus
	disconnFn := m.callbacks.OnDisconnected
	path := m.lastTrackPath
	elapsed := m.lastElapsed
	m.mu.RUnlock()

	if disconnFn != nil {
		disconnFn()
	}
	if fn != nil {
		fn("mpv disconnected, restarting…")
	}

	m.mu.Lock()
	m.cancel()
	newCtx, newCancel := context.WithCancel(m.rootCtx)
	m.ctx = newCtx
	m.cancel = newCancel
	m.cmdQueue = []func() error{}
	m.prefetchedIndex = -1
	m.prefetchedPath = ""
	m.mu.Unlock()

	if err := os.Remove(m.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("player: remove socket", "path", m.socketPath, "err", err)
	}

	if m.launchFn == nil {
		if fn != nil {
			fn("mpv: restart failed: launchFn is nil")
		}
		return
	}

	newCmd, err := m.launchFn(newCtx, m.socketPath)
	if err != nil {
		if fn != nil {
			fn("mpv: restart failed: " + err.Error())
		}
		return
	}
	m.mu.Lock()
	m.cmd = newCmd
	m.mu.Unlock()

	if path != "" {
		if err := m.enqueue(func() error {
			return m.loadFileAt(path, elapsed)
		}); err != nil {
			slog.Warn("mpv: failed to enqueue resume command", "err", err)
		}
	}

	go m.pollUntilReady(newCtx)
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
	_, err = c.SendCommand(cmdLoadfile, path, "replace", 0, fmt.Sprintf("start=%g,pause=yes", elapsedSecs))
	return err
}

// loadFileAt sends mpv a loadfile command with a start= option to resume
// playback at the given elapsed time (in seconds).
func (m *Manager) loadFileAt(path string, elapsed float64) error {
	c, err := m.ipc()
	if err != nil {
		return err
	}
	_, err = c.SendCommand(cmdLoadfile, path, "replace", 0, fmt.Sprintf("start=%g,pause=no", elapsed))
	return err
}
