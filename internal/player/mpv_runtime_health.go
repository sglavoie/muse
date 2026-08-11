package player

import (
	"context"
	"log/slog"
	"time"
)

// healthCheck runs in a goroutine after the IPC socket connects. It pings mpv
// at regular intervals; on the first ping failure it hands the error to the
// shared connection-failure transition, which detaches the client and signals
// the restart supervisor.
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
			if _, err := client.SendCommand("get_version"); err != nil {
				m.handleConnectionFailure(ctx, client, err)
				return
			}
		}
	}
}

// handleConnectionFailure is the single generation-guarded disconnect
// transition shared by the health-check goroutine and the dedicated event
// reader. Whichever goroutine first observes the still-installed command client
// closes it, detaches it, re-enables command queueing, and signals the restart
// supervisor exactly once; a second caller for the same generation sees
// m.client != client and does nothing, so simultaneous health and event
// failures cannot launch duplicate restarts.
//
// Failures from a canceled or superseded generation are ignored: if ctx is
// already done, a restart has replaced this connection and a late failure from
// the old generation must not resurrect the queue or restart a healthy
// replacement. A nil client means there is no installed generation to fail
// (only reachable from isolated unit tests), so it is likewise a no-op.
func (m *Manager) handleConnectionFailure(ctx context.Context, client *IPCClient, err error) {
	if client == nil {
		return
	}

	// Stale/canceled generation: the subprocess context was already replaced by
	// a restart, so this failure belongs to a dead generation.
	select {
	case <-ctx.Done():
		return
	default:
	}

	// If the manager is shutting down, do not resurrect the cmdQueue (Close set
	// it to nil) — doing so would make subsequent Player calls silently enqueue
	// commands that will never be replayed instead of returning ErrNotConnected.
	shuttingDown := false
	if m.rootCtx != nil {
		select {
		case <-m.rootCtx.Done():
			shuttingDown = true
		default:
		}
	}

	m.mu.Lock()
	if m.client != client {
		// Another goroutine already handled this generation's failure, or the
		// client was already replaced by a newer generation. Do nothing.
		m.mu.Unlock()
		return
	}
	m.client = nil
	if !shuttingDown {
		m.cmdQueue = []func() error{}
	}
	m.mu.Unlock()

	_ = client.Close()

	slog.Info("mpv disconnected", "err", err)
	if !shuttingDown {
		select {
		case m.disconnectCh <- err:
		default:
		}
	}
}
