package player

import (
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

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

	// pendingEOFSettlement records an EOF fallback that could not yet stop the
	// invalid successor mpv had already begun playing. The liveness settlement
	// is withheld while this is true: a bounded removal retry may settle the
	// successor in place, otherwise the restart path consumes the flag only after
	// the old process has been reaped. pendingExhaustionReport says that the same
	// settlement also owes OnPlaybackEnded because traversal had run out. Both
	// are guarded by mu.
	pendingEOFSettlement    bool
	pendingExhaustionReport bool

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

	// shuttingDown is set by BeginShutdown once the process has decided to
	// exit, so the restart supervisor stops relaunching mpv. It is atomic
	// rather than guarded by mu because the supervisor reads it right before
	// launchFn, outside the lock.
	shuttingDown atomic.Bool

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

	// pendingLoadStarts is the FIFO of load-start expectations whose `loadfile
	// ... replace` has been sent but whose start-file the event reader has not
	// consumed yet. mpv answers loads in the order they were sent, so the front
	// of this queue names the generation of the next load-induced start-file
	// however far behind the reader has fallen.
	//
	// It is deliberately unbounded: the correlation is positional, so trimming
	// it to a fixed length would relabel every start-file still queued behind
	// the dropped entry. Its length is bounded by the reader's own backlog —
	// every entry is withdrawn on a rejected load, consumed by the start-file
	// answering it, or retired with the reader that was owed it. Guarded by mu.
	//
	// Each entry also carries the file its load makes current, so a reader coming
	// up mid-playback can recognise the expectation mpv answers with the entry it
	// reports playing. See reconcileStartedLoad.
	pendingLoadStarts []pendingLoadStart

	// loadStartSeq numbers the load-start expectations in the order they were
	// recorded, so an expectation can be placed against the instant an event
	// reader's connection came up. It is what makes the reader observation
	// boundary expressible: the generation cannot say when a load was issued —
	// it moves for reasons of its own and is handed back by a window that
	// loaded nothing — and the queue's length cannot either, since entries are
	// consumed and withdrawn from both ends. Guarded by mu.
	loadStartSeq uint64

	// sentLoadStartSeq is the highest loadStartSeq whose `loadfile ... replace`
	// mpv has answered. It is the observation boundary an event reader captures
	// when its connection comes up, and it is deliberately not loadStartSeq:
	// that number is assigned before the command is sent, so it records the
	// intent to load rather than anything about the event the load produces.
	//
	// mpv emits the start-file while it is executing the loadfile, before it
	// writes the reply, so an answered load is one whose start-file has already
	// fired — to whatever connections existed then. A load still in flight is
	// the opposite case: its event has not been emitted yet, so it will be
	// broadcast to the connection that just came up, and the expectation
	// recorded for it must survive that reader's adoption even though it was
	// recorded before the dial.
	//
	// It is a lower bound on what has started and never an upper one: a load mpv
	// is executing when the reader dials is unanswered too, and its start-file is
	// just as unobservable. Only mpv naming the file it is playing separates
	// those two, which is what reconcileStartedLoad does from the event stream
	// once the reader is scanning it. Guarded by mu.
	sentLoadStartSeq uint64

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

	// unreachedAdditions holds the identities of rows that entered the queue at
	// or before the sequential cursor, so plain currentIndex+1 arithmetic steps
	// straight past them even though the traversal never played them. It is the
	// shuffle-off counterpart of shuffleUnseen: with shuffle on, reconciliation
	// puts every brand-new identity into the unseen pool and position is
	// irrelevant. Recorded by SetQueue, consumed by the sequential resume, and
	// discarded once the traversal has moved past them. Guarded by mu; see
	// sequential_additions.go.
	unreachedAdditions map[string]bool

	// sequentialCompleted holds the identities of every row plain positional
	// traversal had already played when the cursor stepped back behind them —
	// which happens when a resume starts a row inserted behind an exhausted
	// cursor. The furthest of them still in the queue is the frontier:
	// everything up to it has already played, so from there on "the next row"
	// is the remaining insertions followed by whatever lies beyond the
	// frontier, never currentIndex+1. The whole completed prefix is recorded
	// rather than just its last row so that removing or replacing that row
	// leaves the boundary standing on the deepest survivor instead of dropping
	// it and handing the completed rows back as successors. Empty means the
	// cursor is the frontier and continuation is plain arithmetic. Guarded by
	// mu; see sequential_additions.go.
	sequentialCompleted map[string]bool

	// permute returns a permutation of [0,n) used to randomize the unseen
	// pool. Production wires math/rand/v2; tests inject a fixed permutation.
	// Nil falls back to deterministic identity order (see permuteLocked).
	permute func(n int) []int
}
