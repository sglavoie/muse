package app

import (
	"context"
	"log/slog"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/safego"
)

// bridgeScanChannel is the context-aware scan bridge. It forwards every track
// from raw to the returned channel and batches upserts via writer, draining any
// remaining batch when raw is exhausted or ctx is cancelled. If writer also
// implements GenrePopulator, PopulateTrackGenres is called after each batch
// flush to keep the track_genres junction table current. A batch whose
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
	// a caller may pass a plain UpsertBatcher.
	marker, _ := writer.(MissingMarker)
	ch := make(chan library.Track, scanChannelBuffer)
	done := make(chan struct{})
	go func() {
		// Both closes stay deferred: the TUI ranges over ch and LoadLibrary
		// waits on done, so a recovered bridge must still release both.
		defer safego.Recover("app.bridgeScanChannel")
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
