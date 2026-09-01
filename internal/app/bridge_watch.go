package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/safego"
)

const (
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
		// The timer stops and both channel closes stay deferred so a recovered
		// panic leaves no armed timer and no blocked receiver.
		defer safego.Recover("app.bridgeWatchChannel")
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
