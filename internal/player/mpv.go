package player

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os/exec"
	"sort"
	"strings"
	"sync"
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

	// activating is true while activateConnection has installed the client but
	// has not yet finished restoring pre-crash state and replaying the queued
	// commands. During this window public commands keep enqueueing (via admit /
	// setFilter) even though m.client is non-nil, so a concurrent user command
	// cannot execute against the fresh client and then be overwritten by the
	// restore sequence, nor overtake an earlier queued command. It stays true for
	// the entire drainQueue replay and is cleared only once the queue is empty.
	// The activation goroutine itself uses the ungated ipc() helper, so restore
	// and replay still reach mpv directly.
	activating bool

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

	// liveness is the Manager's logical playback state: whether a queue
	// traversal is still in progress, or has intentionally ended because
	// stop-after-current fired or the queue ran out at EOF. It is deliberately
	// not derivable from mpv's idle-active property (process health) nor from
	// currentIndex, which keeps naming the row that finished. Best-effort
	// prefetch consults it before an append-play that could start audio; see
	// liveness.go. Guarded by mu.
	liveness playbackLiveness

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

	// restartMaxDelayOverride overrides restartMaxDelay when non-zero.
	// Test-only knob to keep the exponential backoff cap small.
	restartMaxDelayOverride time.Duration

	// restartMaxAttemptsOverride overrides restartMaxAttempts when positive.
	// Test-only knob so give-up behaviour can be exercised in a few attempts.
	restartMaxAttemptsOverride int

	// socketPollTimeoutOverride overrides socketPollTimeout when non-zero.
	// Test-only knob so a socket that never comes up times out quickly.
	socketPollTimeoutOverride time.Duration

	// lastTrackPath and lastElapsed hold the most recently persisted playback
	// position, used to resume playback after an auto-restart.
	lastTrackPath string
	lastElapsed   float64

	// lastPaused tracks whether playback was paused when the connection was
	// last active, so auto-restart can preserve the pause state instead of
	// unconditionally resuming.
	lastPaused bool

	// lastVolume and volumeKnown track the last volume set through the Manager
	// so the restart supervisor can restore it after mpv reconnects. volumeKnown
	// stays false until an absolute volume is applied, because a relative-only
	// adjustment cannot establish an absolute baseline.
	lastVolume  float64
	volumeKnown bool

	// repeatMode is the last repeat mode ("none"/"all"/"one") applied through
	// SetRepeat. Empty means SetRepeat has never been called, so the restart
	// supervisor leaves mpv's default in place. Guarded by mu.
	repeatMode string

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

	// prefetchedIdentity is the shuffle traversal identity of the prefetched
	// track. Empty when nothing is prefetched or the prefetch is a plain
	// sequential one (no shuffle identity). It lets the EOF handler survive a
	// visible queue reorder — the identity is re-resolved to a live index — and
	// reject a stale prefetch whose row was removed or whose path changed. It is
	// also the identity that shuffle history commits to only once the EOF
	// transition to it is confirmed (prefetch resolves the successor without
	// consuming it).
	prefetchedIdentity string

	// completedSlotPending records that mpv's playlist still holds, at
	// position 0, the entry that just finished: a gapless transition moved
	// mpv onto its successor, but the EOF maintenance has not yet removed the
	// completed slot. While it is set the playlist is one entry longer than
	// the bookkeeping assumes, so the slot-1 addressing that prefetch appends
	// and removals rely on does not hold and no successor may be appended.
	//
	// It is cleared by a successful completed-slot removal and by anything
	// that resets mpv's playlist wholesale (an accepted `loadfile ...
	// replace`, a fresh connection, a subprocess restart). Guarded by mu.
	completedSlotPending bool

	// prefetchedGen is the playlist generation the tracked prefetch was
	// appended into (see playlistGen). A prefetch recorded in an older
	// generation refers to a playlist mpv has since replaced, so the EOF
	// handler must not promote it.
	prefetchedGen uint64

	// prefetchedLoadGen is the load generation the tracked prefetch was
	// appended into (see loadGen). It is what says whether the successor sits
	// behind the entry the event reader currently believes is playing, which
	// playlistGen cannot answer: a replacement window bumps loadGen before mpv
	// answers, so an append made inside one belongs to a load whose start-file
	// has not been read yet. Guarded by mu.
	prefetchedLoadGen uint64

	// playlistMu is the playlist-transition lock. Every queue-owned mutation
	// of mpv's internal playlist — the `loadfile ... replace` that starts a
	// queue entry, the `loadfile ... append-play` that prefetches its
	// successor, and the `playlist-remove` that drops a stale prefetch or a
	// completed entry — runs while it is held, together with the Go-side
	// bookkeeping that mirrors mpv's playlist. It replaces the narrower
	// prefetch mutex, which serialized appends against each other but not
	// against replacements.
	//
	// Lock ordering: playlistMu is always acquired BEFORE mu. Never take
	// playlistMu while holding mu.
	playlistMu sync.Mutex

	// playlistGen counts queue-owned playlist replacements. It is bumped
	// under mu whenever mpv accepts a `loadfile ... replace` (and whenever a
	// fresh mpv connection is activated, since that process starts with an
	// empty playlist). Prefetch tracking is stamped with the generation it
	// was appended into so a successor resolved against a superseded playlist
	// can be recognized and discarded instead of falsely advanced into.
	playlistGen uint64

	// replacing records the queue-owned playlist replacements that are in
	// flight: a replacement transaction has published (or is about to
	// publish) the traversal state it is making current, but mpv has not yet
	// confirmed the `loadfile ... replace` that acts on it. While it is set,
	// an end-file/eof describes the entry the replacement is discarding, not
	// the state the rest of the Manager already reflects. Guarded by mu.
	replacing replacementState

	// loadGen counts the queue-owned playlist replacements that have been
	// started, where playlistGen counts the ones mpv has accepted. It is
	// bumped when a replacement window opens — before any traversal state is
	// published and before the loadfile is sent — and given back when a window
	// closes without a load having landed.
	//
	// It is what correlates an end-file/eof with the entry that produced it
	// once the replacement has been acknowledged. playlistGen cannot: it
	// advances only after mpv answers, and the window closes with it, so
	// between them nothing distinguishes an EOF the old entry emitted before
	// the answer (and that sat buffered on the event socket) from one the new
	// entry emitted after it. Guarded by mu.
	loadGen uint64

	// startedLoadGen is the load generation of the file mpv last reported
	// starting. The event socket is ordered, so the end-file of an entry is
	// always read before the start-file of whatever replaced it: at the moment
	// an end-file is handled, this names the generation the entry that ended
	// belongs to. A mismatch with loadGen therefore means a replacement has
	// superseded that entry, whether or not its command is still in flight.
	//
	// It is the generation that *emitted* the start-file, not the one that
	// happened to be live when the reader dispatched it. Events can sit
	// buffered on the socket across a whole replacement, so those are different
	// generations, and reading the later one would re-arm an entry a
	// replacement has already discarded. pendingLoadStarts and
	// gaplessSuccessors are what supply the emitting generation.
	//
	// Zero means the correlation is disarmed: either the event reader has not
	// observed a file start yet, or no replacement has ever been started, so
	// there is no superseding load an end-file could belong to the wrong side
	// of. Guarded by mu.
	startedLoadGen uint64

	// pendingLoadStarts is the FIFO of load generations whose `loadfile ...
	// replace` has been sent but whose start-file the event reader has not
	// consumed yet. mpv answers loads in the order they were sent, so the front
	// of this queue names the generation of the next load-induced start-file
	// however far behind the reader has fallen. Guarded by mu.
	pendingLoadStarts []uint64

	// gaplessSuccessors counts the entries appended behind the file mpv is
	// reported to be playing (generation startedLoadGen) that it may still
	// start on its own. It is what distinguishes the two kinds of start-file:
	// a nonzero count at an end-file/eof means the start-file that follows is
	// the same generation moving into its prefetched successor, not the arrival
	// of a newer load. Guarded by mu.
	gaplessSuccessors int

	// Go-owned shuffle traversal state, all guarded by mu. When
	// shuffleEnabled is true, Next/Previous consult this state instead of
	// plain visible-order index arithmetic; mpv's own shuffle property is
	// never used.
	shuffleEnabled bool
	shuffleHistory []string // identities visited, in play order
	shufflePos     int      // cursor into shuffleHistory; -1 when empty
	shuffleUnseen  []string // identities not yet visited, in permuted order

	// shuffleExcluded holds the identities the current cycle has dropped as
	// unplayable — missing on disk, or removed from the queue between resolution
	// and load. They are neither played nor pending, so nothing in the traversal
	// pools records them, and reconciliation would otherwise treat them as
	// brand-new identities and schedule them again on the next unrelated append
	// or reorder. Cleared only when a genuinely fresh cycle starts (shuffle
	// enabled, explicit reanchor, cycle restart) or shuffle is turned off.
	shuffleExcluded map[string]bool

	// permute returns a permutation of [0,n) used to randomize the unseen
	// pool. Production wires math/rand/v2; tests inject a fixed permutation.
	// Nil falls back to deterministic identity order (see permuteLocked).
	permute func(n int) []int
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
		currentIndex:    -1,
		prefetchedIndex: -1,
		shufflePos:      -1,
		permute:         rand.Perm,
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
