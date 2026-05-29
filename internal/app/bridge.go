package app

import (
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const (
	scanChannelBuffer       = 256
	watchChannelBuffer      = 64
	retainedBatchMultiplier = 2
)

// UpsertBatcher batches library track upserts to the database.
type UpsertBatcher interface {
	UpsertBatch([]library.Track) error
}

// MissingMarker marks a single track as missing in the database by hash.
type MissingMarker interface {
	MarkTrackMissing(hash string) error
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
	if idx, ok := f.indexByHash[t.Hash]; ok {
		f.batch[idx] = t
		if len(f.batch) >= f.size {
			f.drain()
		}
		return
	}
	if len(f.batch) >= f.maxRetained {
		f.drain()
	}
	if len(f.batch) >= f.maxRetained {
		slog.Warn("batch backlog full; dropping track until retry succeeds", "hash", t.Hash, "path", t.Path, "count", len(f.batch), "limit", f.maxRetained)
		return
	}
	f.batch = append(f.batch, t)
	f.indexByHash[t.Hash] = len(f.batch) - 1
	if len(f.batch) >= f.size {
		f.drain()
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

func (f *batchFlusher) drain() {
	if len(f.batch) == 0 {
		return
	}
	if err := f.flush(f.batch); err != nil {
		slog.Warn("batch flush", "err", err, "count", len(f.batch))
		// Preserve the batch so a later drain can retry the same items.
		return
	}
	if f.populate != nil {
		if err := f.populate(f.batch); err != nil {
			slog.Warn("batch genre populate", "err", err, "count", len(f.batch))
		}
	}
	f.batch = f.batch[:0]
	clear(f.indexByHash)
}

// BridgeScanChannel reads tracks from raw, forwards each to the returned
// channel, and batches upserts via writer. The returned channel is closed
// when raw is exhausted and any remaining batch has been flushed.
// If writer also implements GenrePopulator, PopulateTrackGenres is called
// after each batch flush to keep the track_genres junction table current.
func BridgeScanChannel(raw <-chan library.Track, writer UpsertBatcher, batchSize int) <-chan library.Track {
	var populate func([]library.Track) error
	if gp, ok := writer.(GenrePopulator); ok {
		populate = gp.PopulateTrackGenres
	}
	ch := make(chan library.Track, scanChannelBuffer)
	go func() {
		defer close(ch)
		f := newBatchFlusher(batchSize, writer.UpsertBatch, populate)
		for t := range raw {
			t = ensureCreatedAt(t)
			ch <- t
			f.add(t)
		}
		f.drain()
	}()
	return ch
}

// BridgeWatchChannel reads watch events from raw, forwards each to the
// returned channel, and persists changes: new/modified tracks are batched
// for upsert; deleted tracks are marked missing in the database via marker.
// hashByPath maps file paths to content hashes for deletion lookups. The
// returned channel is closed when raw is exhausted.
// If writer also implements GenrePopulator, PopulateTrackGenres is called
// after each batch flush to keep the track_genres junction table current.
func BridgeWatchChannel(raw <-chan library.WatchEvent, writer UpsertBatcher, marker MissingMarker, hashByPath map[string]string, batchSize int) <-chan library.WatchEvent {
	// Copy the map so the goroutine owns its own mutable state and does not
	// race with callers that may still read the original map.
	localMap := make(map[string]string, len(hashByPath))
	pathByHash := make(map[string]string, len(hashByPath))
	for k, v := range hashByPath {
		localMap[k] = v
		pathByHash[v] = k
	}

	var populate func([]library.Track) error
	if gp, ok := writer.(GenrePopulator); ok {
		populate = gp.PopulateTrackGenres
	}

	ch := make(chan library.WatchEvent, watchChannelBuffer)
	go func() {
		defer close(ch)
		f := newBatchFlusher(batchSize, writer.UpsertBatch, populate)
		for ev := range raw {
			if ev.Deleted {
				ch <- ev
				if hash, ok := localMap[ev.Path]; ok {
					f.remove(hash)
					if err := marker.MarkTrackMissing(hash); err != nil {
						slog.Warn("watcher: mark deleted track missing", "path", ev.Path, "err", err)
					}
					delete(localMap, ev.Path)
					if pathByHash[hash] == ev.Path {
						delete(pathByHash, hash)
					}
				}
				f.drain()
			} else {
				ev.Track = ensureCreatedAt(ev.Track)
				ch <- ev
				if oldHash := localMap[ev.Track.Path]; oldHash != "" && oldHash != ev.Track.Hash {
					f.remove(oldHash)
					if err := marker.MarkTrackMissing(oldHash); err != nil {
						slog.Warn("watcher: mark replaced track missing", "path", ev.Track.Path, "err", err)
					}
					delete(pathByHash, oldHash)
				}
				f.add(ev.Track)
				// Keep the lookup map in sync so that a file created after
				// startup and subsequently deleted can still be resolved to
				// its content hash for the MarkTrackMissing call above.
				if oldPath := pathByHash[ev.Track.Hash]; oldPath != "" && oldPath != ev.Track.Path {
					delete(localMap, oldPath)
				}
				localMap[ev.Track.Path] = ev.Track.Hash
				pathByHash[ev.Track.Hash] = ev.Track.Path
			}
		}
		f.drain()
	}()
	return ch
}
