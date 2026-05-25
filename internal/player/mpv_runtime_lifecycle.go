package player

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

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
	// Nil the cmdQueue so any caller invoking Player methods after Close
	// receives ErrNotConnected immediately rather than silently appending
	// to a queue that will never be drained.
	m.cmdQueue = nil
	cmd := m.cmd
	m.cmd = nil
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
