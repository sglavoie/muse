package player

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

// EventCallbacks bundles the optional notification callbacks that the Manager
// fires in response to mpv events. All fields are optional; nil callbacks are
// silently skipped. Pass the struct to SetEventCallbacks once all callbacks are
// wired — the call also unblocks the event reader.
type EventCallbacks struct {
	// OnStatus is called with transient status messages (e.g. "Connecting to mpv…").
	OnStatus func(string)
	// OnMissingFile is called with the file path when a missing track is detected.
	OnMissingFile func(string)
	// OnPlaybackEnded is called when auto-advance exhausts the queue.
	OnPlaybackEnded func()
	// OnTrackChanged is called when a new track starts (mpv "start-file" event).
	// It receives the content hash, artist, title, playlist source, and queue index.
	OnTrackChanged func(hash, artist, title, source string, index int)
	// OnPauseChanged is called when mpv's pause property changes.
	OnPauseChanged func(paused bool)
	// OnDisconnected is called at the start of handleRestart, before reconnection begins.
	OnDisconnected func()
}

// Manager manages an mpv subprocess and its JSON-IPC connection.
// It implements the Player interface. Commands issued after Start() but before
// the IPC socket becomes ready are queued and replayed on connect. Commands
// issued before Start() return ErrNotConnected immediately.
type Manager struct {
	socketPath string
	extraFlags []string // additional CLI flags appended to the mpv command

	mu            sync.RWMutex
	callbacksOnce sync.Once
	client        *IPCClient     // nil until the socket is connected
	cmdQueue      []func() error // nil = not started; non-nil = started, may have pending cmds
	callbacks     EventCallbacks // event notification callbacks; set via SetEventCallbacks

	// queue is the ordered set of tracks available for playback. Set via
	// SetQueue so that PlayFromIndex can resolve an index to a QueueTrack.
	queue []QueueTrack

	// currentIndex is the queue index of the most recently successfully
	// started track. Used by Next/Previous and the event reader for
	// auto-advance.
	currentIndex int

	// stopAfterCurrent, when true, causes the event reader to fire
	// OnPlaybackEnded instead of auto-advancing on the next EOF.
	// The flag is cleared atomically after it fires.
	stopAfterCurrent bool

	// marker persists missing-file state to the database. Set via
	// SetMissingMarker. When nil, PlayFromIndex uses a no-op marker.
	marker MissingMarker

	// disconnectCh receives the first error when the IPC socket is detected
	// as disconnected by the health-check goroutine. The channel is buffered
	// (size 1) so the goroutine never blocks.
	disconnectCh chan error

	// audioFilters tracks enabled lavfi filter expressions keyed by name
	// (e.g. "loudnorm" → "loudnorm"). rebuildAudioFilters combines them
	// into a single af property string.
	audioFilters map[string]string

	// pingInterval controls how often the health-check goroutine pings mpv.
	// Zero uses the production default (200 ms). Set a shorter value in tests.
	pingInterval time.Duration

	// restartBaseDelay overrides restartInitialDelay when non-zero.
	// Set a shorter value in tests to avoid slow backoff waits.
	restartBaseDelay time.Duration

	// backoffReset is set to true by pollUntilReady when the IPC socket
	// connects successfully, signalling watchAndRestart to reset its
	// consecutive-failure counter.
	backoffReset atomic.Bool

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

	// callbacksReady is closed by SetEventCallbacks once the callback
	// struct has been stored. startEventReader blocks on this channel
	// before entering the scan loop to prevent events from firing before
	// the callbacks are wired.
	callbacksReady chan struct{}

	// launchFn starts the mpv subprocess. Tests replace this with a no-op
	// that returns (nil, nil) while providing their own fake socket server.
	launchFn func(ctx context.Context, socketPath string) (*exec.Cmd, error)

	// gaplessEnabled tracks whether gapless playback is active; when false
	// the prefetch mechanism is skipped entirely.
	gaplessEnabled bool

	// prefetchedIndex is the queue index of the track currently appended to
	// mpv's internal playlist as position 2 (the "next" track). -1 means
	// nothing is prefetched.
	prefetchedIndex int

	// prefetchedPath is the file path of the prefetched track, used for
	// staleness checks so prefetchNext is idempotent.
	prefetchedPath string
}

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
		prefetchedIndex: -1,
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
// auto-restart can resume at the correct offset.
func (m *Manager) SetCurrentPosition(path string, elapsed float64) {
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
		{"path", "path"},
	}
	for _, p := range props {
		resp, err := c.SendCommand("get_property", p.prop)
		if err == nil {
			if s, ok := resp.Data.(string); ok {
				result[p.key] = s
			} else if resp.Data != nil {
				result[p.key] = fmt.Sprintf("%v", resp.Data)
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
