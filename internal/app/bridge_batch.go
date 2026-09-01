package app

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/library"
)

// retainedBatchMultiplier bounds how far a batch may grow past the size cap
// while its upsert keeps failing, before further tracks are dropped.
const retainedBatchMultiplier = 2

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
