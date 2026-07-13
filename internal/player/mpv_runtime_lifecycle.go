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

// Close cancels the mpv subprocess and closes the IPC connection.
func (m *Manager) Close() error {
	if m.rootCancel != nil {
		m.rootCancel()
	} else if m.cancel != nil {
		m.cancel()
	}

	m.mu.Lock()
	c := m.client
	m.client = nil
	// Nil the cmdQueue so any caller invoking Player methods after Close
	// receives ErrNotConnected immediately rather than silently appending
	// to a queue that will never be drained.
	m.cmdQueue = nil
	// Clear activating so a Close racing an in-flight activation cannot leave the
	// gate stuck; with cmdQueue nil, admit now returns ErrNotConnected.
	m.activating = false
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

// pollUntilReady performs the initial connection after Start: it waits for the
// mpv socket, then installs the client and replays any queued commands. If the
// socket never comes up (and the manager is not shutting down) it enters the
// shared restart retry path, which relaunches mpv exactly like a mid-session
// outage. It returns immediately when the context is cancelled (e.g. by Close).
func (m *Manager) pollUntilReady(ctx context.Context) {
	client, err := m.connectSocket(ctx)
	if err == nil {
		m.activateConnection(ctx, client, nil)
		return
	}
	if ctx.Err() != nil {
		// Manager is shutting down; nothing to install or recover.
		return
	}

	slog.Warn("mpv: initial socket connection timed out; entering restart retry path",
		"socket", m.socketPath, "timeout", m.effectiveSocketPollTimeout())
	m.recoverOutage()
}
