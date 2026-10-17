package player

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os/exec"
	"sort"
	"strings"
)

// NewManager creates a Manager with the given socket path.
// The caller is responsible for removing any stale socket file before calling
// Start; app.Run handles this in step 2 (after the PID lockfile is acquired).
func NewManager(socketPath string) *Manager {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	m := &Manager{
		socketPath:      socketPath,
		rootCtx:         rootCtx,
		rootCancel:      rootCancel,
		ctx:             ctx,
		cancel:          cancel,
		disconnectCh:    make(chan error, 1),
		callbacksReady:  make(chan struct{}),
		audioFilters:    make(map[string]string),
		currentIndex:    -1,
		prefetchedIndex: -1,
		shufflePos:      -1,
		permute:         rand.Perm,
	}
	m.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		m.mu.RLock()
		extra := m.extraFlags
		m.mu.RUnlock()
		cmd := exec.CommandContext(ctx, "mpv", launchArgs(socketPath, extra)...)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("mpv: start: %w", err)
		}
		return cmd, nil
	}
	return m
}

// SocketPath returns the resolved Unix domain socket path.
func (m *Manager) SocketPath() string {
	return m.socketPath
}

// SetEventCallbacks stores the callback struct and unblocks the event reader.
// Call once after all callbacks are wired. Subsequent calls update the
// callbacks but do not re-signal readiness.
func (m *Manager) SetEventCallbacks(cb EventCallbacks) {
	m.mu.Lock()
	m.callbacks = cb
	m.mu.Unlock()
	m.callbacksOnce.Do(func() { close(m.callbacksReady) })
}

// SetCurrentPosition persists the current playback position for use in crash
// recovery. Call this periodically while a track is playing so that
// auto-restart can resume at the correct offset. Negative elapsed values and
// non-finite values are clamped to 0 so the resume loadfile does not seek to
// a bogus offset.
func (m *Manager) SetCurrentPosition(path string, elapsed float64) {
	if elapsed < 0 || math.IsNaN(elapsed) || math.IsInf(elapsed, 0) {
		elapsed = 0
	}
	m.mu.Lock()
	m.lastTrackPath = path
	m.lastElapsed = elapsed
	m.mu.Unlock()
}

// SetMissingMarker sets the MissingMarker used to persist missing-file state
// to the database. Call before playback begins.
func (m *Manager) SetMissingMarker(marker MissingMarker) {
	m.mu.Lock()
	m.marker = marker
	m.mu.Unlock()
}

// SetPermuteFunc injects the permutation generator used to randomize the
// shuffle unseen pool. Production wires math/rand/v2 in NewManager; tests use
// this to supply a fixed permutation for deterministic traversal assertions.
func (m *Manager) SetPermuteFunc(fn func(n int) []int) {
	m.mu.Lock()
	m.permute = fn
	m.mu.Unlock()
}

// DisconnectCh returns a channel that receives the first error surfaced by the
// health-check goroutine when an IPC socket disconnect is detected. The caller
// must not close the channel.
func (m *Manager) DisconnectCh() <-chan error {
	return m.disconnectCh
}

// SetExtraFlags sets additional command-line flags appended to the mpv
// subprocess after --idle=yes and --input-ipc-server. Call before Start.
func (m *Manager) SetExtraFlags(flags []string) {
	m.mu.Lock()
	m.extraFlags = make([]string, len(flags))
	copy(m.extraFlags, flags)
	m.mu.Unlock()
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

// admit runs send against the live mpv client, or enqueues it for in-order
// replay when mpv is not connected or the connection is still activating. The
// gate check and the enqueue happen under a single lock acquisition, so a
// command that observes the activation gate can never then race the queue to nil
// and be spuriously rejected: while activating is true the queue is guaranteed
// non-nil (only Close nils it, and Close clears activating in the same locked
// step). A queued send is replayed by drainQueue against the restored client
// through the ungated ipc() helper, so it never re-enters this gate and cannot
// recursively re-enqueue itself while activation is still in progress. Returns
// ErrNotConnected only when the manager was never started or has been closed.
func (m *Manager) admit(send func(*IPCClient) error) error {
	return m.admitGated(send, false)
}

// admitPlaylist is admit for queue-owned playlist mutations. It behaves
// identically except that a send deferred to the reconnect queue re-acquires the
// playlist-transition lock before it reaches mpv, so a drainQueue replay can
// never interleave a queued playlist mutation with a live playlist transaction.
// The caller must hold m.playlistMu (the lock is released before the replay runs
// because the replay happens on the activation goroutine, long after the
// enqueueing transaction has completed).
func (m *Manager) admitPlaylist(send func(*IPCClient) error) error {
	return m.admitGated(send, true)
}

// admitGated is the shared body of admit and admitPlaylist. When playlistOwned
// is true the queued replay wraps send in the playlist-transition lock.
func (m *Manager) admitGated(send func(*IPCClient) error, playlistOwned bool) error {
	m.mu.Lock()
	c := m.client
	if c != nil && !m.activating {
		m.mu.Unlock()
		return send(c)
	}
	if m.cmdQueue == nil {
		m.mu.Unlock()
		return ErrNotConnected
	}
	m.cmdQueue = append(m.cmdQueue, m.replayClosure(send, playlistOwned))
	fn := m.callbacks.OnStatus
	m.mu.Unlock()
	if fn != nil {
		fn("Connecting to mpv…")
	}
	return nil
}

// replayClosure wraps send for deferred replay by drainQueue. It resolves the
// client through the ungated ipc() helper so a replay never re-enters the
// activation gate, and a queue-owned playlist mutation re-acquires the
// playlist-transition lock first so the replay cannot interleave with a live
// playlist transaction.
func (m *Manager) replayClosure(send func(*IPCClient) error, playlistOwned bool) func() error {
	return func() error {
		if playlistOwned {
			m.playlistMu.Lock()
			defer m.playlistMu.Unlock()
		}
		c, err := m.ipc()
		if err != nil {
			return err
		}
		return send(c)
	}
}

// DebugInfo queries mpv for diagnostic properties and returns them as a
// string map. Individual property failures set the value to "N/A" and do not
// stop collection. Returns an empty map (not nil) with all values "N/A" when
// mpv is not connected.
func (m *Manager) DebugInfo() (map[string]string, error) {
	result := map[string]string{
		"codec":         "N/A",
		"sample_rate":   "N/A",
		"bitrate":       "N/A",
		"audio_device":  "N/A",
		"path":          "N/A",
		"audio_filters": "",
	}

	c, err := m.ipc()
	if err != nil {
		return result, err
	}

	props := []struct {
		key  string
		prop string
	}{
		{"codec", "audio-codec-name"},
		{"sample_rate", "audio-params/samplerate"},
		{"bitrate", "audio-bitrate"},
		{"audio_device", "audio-device"},
		{"path", propPath},
	}
	for _, p := range props {
		resp, err := c.SendCommand(cmdGetProperty, p.prop)
		if err == nil {
			if value, ok, formatErr := resp.formattedData(); formatErr == nil && ok {
				result[p.key] = value
			}
		}
	}

	// Collect active audio filter names.
	m.mu.RLock()
	var filters []string
	for name := range m.audioFilters {
		filters = append(filters, name)
	}
	m.mu.RUnlock()
	sort.Strings(filters)
	result["audio_filters"] = strings.Join(filters, ", ")

	return result, nil
}
