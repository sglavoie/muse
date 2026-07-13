package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const (
	scanChannelBuffer       = 256
	watchChannelBuffer      = 64
	retainedBatchMultiplier = 2

	// watchIdleFlush is how long the watcher bridge waits for input to go
	// quiet before flushing an accumulated batch that has not yet reached the
	// size cap. It bounds the latency between a single change landing on disk
	// and its persistence without forcing a write per event.
	watchIdleFlush = 100 * time.Millisecond

	// watchRetryMin and watchRetryMax bound the exponential backoff applied
	// between retries of a batch whose upsert failed. The batch is retained and
	// retried at watchRetryMin, doubling up to watchRetryMax; new input,
	// deletions, or shutdown can trigger an earlier retry.
	watchRetryMin = 250 * time.Millisecond
	watchRetryMax = 5 * time.Second
)

// UpsertBatcher batches library track upserts to the database.
type UpsertBatcher interface {
	UpsertBatch([]library.Track) error
}

// MissingMarker marks a single track as missing in the database by hash.
type MissingMarker interface {
	MarkTrackMissing(hash string) error
}

// pathHashRegistry is the shared path↔hash state that watcher deletion
// handling consults to resolve a deleted file back to its content hash. Both
// the scan bridge (startup/rescan discoveries and moves) and the watch bridge
// (live changes and deletions) mutate it, so every access is guarded by mu.
//
// Seeding it from the cached DB rows and updating it from scan discoveries is
// what lets a track first seen by the startup scan be marked missing when a
// later live deletion arrives: the deletion lookup and the discovery now share
// one map instead of the watch bridge owning a private copy the scan never
// touched.
type pathHashRegistry struct {
	mu         sync.Mutex
	hashByPath map[string]string
	pathByHash map[string]string
}

// newPathHashRegistry returns a registry pre-seeded from seed (typically the
// cached DB rows' path→hash map). The seed map is copied, never retained or
// mutated, so callers keep sole ownership of it.
func newPathHashRegistry(seed map[string]string) *pathHashRegistry {
	r := &pathHashRegistry{
		hashByPath: make(map[string]string, len(seed)),
		pathByHash: make(map[string]string, len(seed)),
	}
	for path, hash := range seed {
		r.hashByPath[path] = hash
		r.pathByHash[hash] = path
	}
	return r
}

// register records that path currently holds the file identified by hash and
// keeps both directions of the mapping consistent. It returns replaced (with
// ok=true) when a *different* file previously occupied path, so the caller can
// mark the displaced hash missing. It also folds in the move case: if the same
// hash previously lived at a different path, that stale path mapping is dropped
// so a later deletion of the old path resolves to nothing (a move, not a
// removal). Re-registering the same (path, hash) is idempotent.
func (r *pathHashRegistry) register(path, hash string) (replaced string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.hashByPath[path]; old != "" && old != hash {
		replaced, ok = old, true
		delete(r.pathByHash, old)
	}
	if oldPath := r.pathByHash[hash]; oldPath != "" && oldPath != path {
		delete(r.hashByPath, oldPath)
	}
	r.hashByPath[path] = hash
	r.pathByHash[hash] = path
	return replaced, ok
}

// remove resolves path to its content hash and deletes the mapping in both
// directions, returning the hash (ok=true) so the caller can mark it missing.
// It returns ok=false when the path is unknown, which is a no-op deletion.
func (r *pathHashRegistry) remove(path string) (hash string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	hash, ok = r.hashByPath[path]
	if !ok {
		return "", false
	}
	delete(r.hashByPath, path)
	if r.pathByHash[hash] == path {
		delete(r.pathByHash, hash)
	}
	return hash, true
}

// GenrePopulator populates the track_genres junction table after an upsert
// batch. Implementations that satisfy this interface will have their
// PopulateTrackGenres method called automatically by the bridge after each
// batch flush.
type GenrePopulator interface {
	PopulateTrackGenres([]library.Track) error
}

// batchFlusher accumulates library.Track values and flushes them in batches
// to a caller-provided function (typically DB upsert). If populate is non-nil,
// it is called after each successful flush (e.g. to populate track_genres).
type batchFlusher struct {
	batch       []library.Track
	indexByHash map[string]int
	size        int
	maxRetained int
	flush       func([]library.Track) error
	populate    func([]library.Track) error
	// failed reports whether the most recent non-empty drain failed and left
	// the batch retained for a later retry. Callers consult it to schedule
	// backoff retries.
	failed bool
}

func newBatchFlusher(size int, flush func([]library.Track) error, populate func([]library.Track) error) *batchFlusher {
	if size < 1 {
		size = 1
	}
	return &batchFlusher{
		indexByHash: make(map[string]int, size*retainedBatchMultiplier),
		size:        size,
		maxRetained: size * retainedBatchMultiplier,
		flush:       flush,
		populate:    populate,
	}
}

func ensureCreatedAt(track library.Track) library.Track {
	if track.CreatedAt == 0 {
		track.CreatedAt = time.Now().Unix()
	}
	return track
}

func (f *batchFlusher) add(t library.Track) {
	// drain's error is intentionally ignored here: on failure the batch is
	// retained in place and f.failed is set, which the bridge consults to
	// schedule a backoff retry. add itself is a void accumulator.
	if idx, ok := f.indexByHash[t.Hash]; ok {
		f.batch[idx] = t
		if len(f.batch) >= f.size {
			_ = f.drain()
		}
		return
	}
	if len(f.batch) >= f.maxRetained {
		_ = f.drain()
	}
	if len(f.batch) >= f.maxRetained {
		slog.Warn("batch backlog full; dropping track until retry succeeds", "hash", t.Hash, "path", t.Path, "count", len(f.batch), "limit", f.maxRetained)
		return
	}
	f.batch = append(f.batch, t)
	f.indexByHash[t.Hash] = len(f.batch) - 1
	if len(f.batch) >= f.size {
		_ = f.drain()
	}
}

func (f *batchFlusher) remove(hash string) {
	idx, ok := f.indexByHash[hash]
	if !ok {
		return
	}
	f.batch = append(f.batch[:idx], f.batch[idx+1:]...)
	delete(f.indexByHash, hash)
	for i := idx; i < len(f.batch); i++ {
		f.indexByHash[f.batch[i].Hash] = i
	}
}

// drain flushes the accumulated batch. On success it clears the batch and
// reports nil; on failure it retains the batch (so a later drain can retry the
// same items) and reports the error. The failed flag mirrors the outcome so
// bridge callers can schedule backoff retries without inspecting the error.
func (f *batchFlusher) drain() error {
	if len(f.batch) == 0 {
		f.failed = false
		return nil
	}
	if err := f.flush(f.batch); err != nil {
		slog.Warn("batch flush", "err", err, "count", len(f.batch))
		// Preserve the batch so a later drain can retry the same items.
		f.failed = true
		return err
	}
	if f.populate != nil {
		if err := f.populate(f.batch); err != nil {
			slog.Warn("batch genre populate", "err", err, "count", len(f.batch))
		}
	}
	f.batch = f.batch[:0]
	clear(f.indexByHash)
	f.failed = false
	return nil
}

// BridgeScanChannel reads tracks from raw, forwards each to the returned
// channel, and batches upserts via writer. The returned channel is closed
// when raw is exhausted and any remaining batch has been flushed.
// If writer also implements GenrePopulator, PopulateTrackGenres is called
// after each batch flush to keep the track_genres junction table current.
//
// It is a thin compatibility wrapper over bridgeScanChannel using a background
// context; callers needing cancellation or completion signaling should use
// bridgeScanChannel directly.
func BridgeScanChannel(raw <-chan library.Track, writer UpsertBatcher, batchSize int) <-chan library.Track {
	ch, _ := bridgeScanChannel(context.Background(), raw, writer, newPathHashRegistry(nil), batchSize)
	return ch
}

// bridgeScanChannel is the context-aware scan bridge. It forwards every track
// from raw to the returned channel and batches upserts via writer, draining any
// remaining batch when raw is exhausted or ctx is cancelled. A batch whose
// upsert fails is retained and retried on the next size-cap drain or the final
// drain, mirroring the watcher bridge's retention without the idle/backoff
// timers (the diff walk is a bounded, one-shot producer).
//
// Outward sends are context-aware so a cancelled pipeline never blocks on a
// full buffer when no consumer remains; the in-flight track is still persisted
// before exit. The returned done channel closes when the goroutine exits,
// giving pipeline-lifecycle callers a completion signal to wait on.
//
// Every discovered track is registered in the shared path/hash registry so a
// file first seen by this scan can still be resolved to its content hash when a
// later live deletion reaches the watch bridge, which shares the same registry.
func bridgeScanChannel(ctx context.Context, raw <-chan library.Track, writer UpsertBatcher, registry *pathHashRegistry, batchSize int) (<-chan library.Track, <-chan struct{}) {
	if registry == nil {
		registry = newPathHashRegistry(nil)
	}
	var populate func([]library.Track) error
	if gp, ok := writer.(GenrePopulator); ok {
		populate = gp.PopulateTrackGenres
	}
	// marker is optional: LoadLibrary passes a LibraryWriter (which marks), while
	// the BridgeScanChannel compatibility wrapper may pass a plain UpsertBatcher.
	marker, _ := writer.(MissingMarker)
	ch := make(chan library.Track, scanChannelBuffer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(ch)
		f := newBatchFlusher(batchSize, writer.UpsertBatch, populate)
		// persistReplacement handles an in-place content replacement discovered
		// by the scan: register reports the *different* hash that previously
		// occupied this path, whose file is now gone. Its content will never be
		// re-upserted (a move re-points the same hash and never reaches this
		// branch), so mark it missing durably and drop any pending upsert for it
		// — mirroring the watch bridge's persistChange so the two bridges keep
		// the same displaced-identity semantics.
		persistReplacement := func(replaced string) {
			f.remove(replaced)
			if marker != nil {
				if err := marker.MarkTrackMissing(replaced); err != nil {
					slog.Warn("scan: mark replaced track missing", "hash", replaced, "err", err)
				}
			}
		}
		for {
			select {
			case <-ctx.Done():
				_ = f.drain()
				return
			case t, ok := <-raw:
				if !ok {
					_ = f.drain()
					return
				}
				t = ensureCreatedAt(t)
				// Register the discovery in the shared registry before
				// forwarding/persisting so a subsequent deletion of this path
				// resolves to its hash. A same-hash move is reconciled silently
				// by register; a different-hash replacement returns the displaced
				// hash so its now-orphaned row is marked missing durably.
				if replaced, ok := registry.register(t.Path, t.Hash); ok {
					persistReplacement(replaced)
				}
				select {
				case ch <- t:
				case <-ctx.Done():
					// No consumer remains; still persist this track and
					// flush before exiting so the batch is not lost.
					f.add(t)
					_ = f.drain()
					return
				}
				f.add(t)
			}
		}
	}()
	return ch, done
}

// BridgeWatchChannel reads watch events from raw, forwards each to the
// returned channel, and persists changes: new/modified tracks are batched
// for upsert; deleted tracks are marked missing in the database via marker.
// hashByPath maps file paths to content hashes for deletion lookups. The
// returned channel is closed when raw is exhausted.
// If writer also implements GenrePopulator, PopulateTrackGenres is called
// after each batch flush to keep the track_genres junction table current.
//
// It is a thin compatibility wrapper over bridgeWatchChannel using a
// background context; callers needing cancellation or completion signaling
// should use bridgeWatchChannel directly.
func BridgeWatchChannel(raw <-chan library.WatchEvent, writer UpsertBatcher, marker MissingMarker, hashByPath map[string]string, batchSize int) <-chan library.WatchEvent {
	ch, _ := bridgeWatchChannel(context.Background(), raw, writer, marker, newPathHashRegistry(hashByPath), batchSize)
	return ch
}

// bridgeWatchChannel is the context-aware watcher bridge. It forwards every
// watch event to the returned event channel and persists changed tracks to the
// database, flushing an accumulated batch when any of the following occurs:
//   - the batch reaches the size cap (batchSize),
//   - input goes quiet for watchIdleFlush,
//   - a deletion event arrives, or
//   - the input channel closes or ctx is cancelled.
//
// A batch whose upsert fails is retained and retried with exponential backoff
// between watchRetryMin and watchRetryMax; new input, deletions, and shutdown
// can trigger an earlier retry. CreatedAt normalization is applied before an
// event is forwarded, and genre population runs only after a successful upsert.
//
// The returned done channel is closed when the bridge goroutine exits (input
// exhausted or ctx cancelled), giving later pipeline-lifecycle callers a
// completion signal to wait on. Every timer is stopped on all exit paths.
//
// The path/hash registry is shared with the scan bridge (see LoadLibrary), so
// deletions resolve tracks discovered by either bridge; its own internal mutex
// keeps concurrent scan and watch observations race-free.
func bridgeWatchChannel(ctx context.Context, raw <-chan library.WatchEvent, writer UpsertBatcher, marker MissingMarker, registry *pathHashRegistry, batchSize int) (<-chan library.WatchEvent, <-chan struct{}) {
	if registry == nil {
		registry = newPathHashRegistry(nil)
	}

	var populate func([]library.Track) error
	if gp, ok := writer.(GenrePopulator); ok {
		populate = gp.PopulateTrackGenres
	}

	ch := make(chan library.WatchEvent, watchChannelBuffer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(ch)
		f := newBatchFlusher(batchSize, writer.UpsertBatch, populate)

		// idle flushes an accumulated sub-cap batch once input goes quiet;
		// retry drives the backoff between attempts on a retained failed batch.
		// Both start stopped and are stopped again on every exit path.
		idle := time.NewTimer(watchIdleFlush)
		idle.Stop()
		retry := time.NewTimer(watchRetryMin)
		retry.Stop()
		defer idle.Stop()
		defer retry.Stop()

		retryActive := false
		retryDelay := watchRetryMin

		armRetry := func() {
			retry.Reset(retryDelay)
			retryActive = true
			retryDelay *= 2
			if retryDelay > watchRetryMax {
				retryDelay = watchRetryMax
			}
		}
		disarmRetry := func() {
			retry.Stop()
			retryActive = false
			retryDelay = watchRetryMin
		}
		// attemptFlush drains any retained batch and reconciles the retry timer
		// with the outcome. Used by the idle, retry, deletion, closure, and
		// cancellation paths.
		attemptFlush := func() {
			// The outcome is consumed via f.failed below; the returned error is
			// the same signal, so ignoring it here is intentional.
			_ = f.drain()
			if f.failed {
				armRetry()
			} else {
				disarmRetry()
			}
		}

		// persistDeletion resolves the deleted path to its hash and marks that
		// hash missing, pruning any pending upsert for it from the batch. It is
		// the DB mutation half of deletion handling, split out so it runs whether
		// the outward forward completes or the send is abandoned on
		// cancellation — an event already pulled off raw must not be dropped
		// before its mutation is attempted (FR-006).
		persistDeletion := func(ev library.WatchEvent) {
			if hash, ok := registry.remove(ev.Path); ok {
				f.remove(hash)
				if err := marker.MarkTrackMissing(hash); err != nil {
					slog.Warn("watcher: mark deleted track missing", "path", ev.Path, "err", err)
				}
			}
		}
		// persistChange stages a changed/new track for upsert. register updates
		// the shared path/hash state (handling the move case internally) and
		// reports any different file it displaced from this path so the watch
		// bridge can mark that old hash missing and drop it from the pending
		// batch. Like persistDeletion, it runs on both the completed-forward and
		// the cancelled-send paths so an already-dequeued event is persisted.
		persistChange := func(ev library.WatchEvent) {
			if replaced, ok := registry.register(ev.Track.Path, ev.Track.Hash); ok {
				f.remove(replaced)
				if err := marker.MarkTrackMissing(replaced); err != nil {
					slog.Warn("watcher: mark replaced track missing", "path", ev.Track.Path, "err", err)
				}
			}
			f.add(ev.Track)
		}

		for {
			select {
			case <-ctx.Done():
				attemptFlush()
				return

			case ev, ok := <-raw:
				if !ok {
					attemptFlush()
					return
				}
				if ev.Deleted {
					select {
					case ch <- ev:
					case <-ctx.Done():
						// The event is already off raw; persist its mutation
						// before flushing and exiting so the in-flight deletion
						// is not lost when no consumer remains.
						persistDeletion(ev)
						attemptFlush()
						return
					}
					persistDeletion(ev)
					attemptFlush()
				} else {
					ev.Track = ensureCreatedAt(ev.Track)
					select {
					case ch <- ev:
					case <-ctx.Done():
						// The event is already off raw; stage and flush the
						// in-flight change before exiting so it is not lost.
						persistChange(ev)
						attemptFlush()
						return
					}
					persistChange(ev)
					// f.add flushes when the size cap is reached. Reconcile the
					// retry timer with that outcome without pushing out an
					// already-pending retry, then arm the idle timer so a
					// sub-cap batch still flushes once input goes quiet. A
					// pending failed batch is retried early by the idle timer.
					if f.failed {
						if !retryActive {
							armRetry()
						}
					} else {
						disarmRetry()
					}
					idle.Reset(watchIdleFlush)
				}

			case <-idle.C:
				attemptFlush()

			case <-retry.C:
				retryActive = false
				attemptFlush()
			}
		}
	}()
	return ch, done
}
