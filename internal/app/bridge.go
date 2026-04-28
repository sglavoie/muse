package app

import (
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const (
	scanChannelBuffer  = 256
	watchChannelBuffer = 64
)

// UpsertBatcher batches library track upserts to the database.
type UpsertBatcher interface {
	UpsertBatch([]library.Track) error
}

// MissingMarker marks a single track as missing in the database by hash.
type MissingMarker interface {
	MarkTrackMissing(hash string) error
}

// batchFlusher accumulates library.Track values and flushes them in batches
// to a caller-provided function (typically DB upsert).
type batchFlusher struct {
	batch []library.Track
	size  int
	flush func([]library.Track) error
}

func ensureCreatedAt(track library.Track) library.Track {
	if track.CreatedAt == 0 {
		track.CreatedAt = time.Now().Unix()
	}
	return track
}

func (f *batchFlusher) add(t library.Track) {
	f.batch = append(f.batch, t)
	if len(f.batch) >= f.size {
		f.drain()
	}
}

func (f *batchFlusher) drain() {
	if len(f.batch) == 0 {
		return
	}
	if err := f.flush(f.batch); err != nil {
		slog.Warn("batch flush", "err", err, "count", len(f.batch))
	}
	f.batch = f.batch[:0]
}

// BridgeScanChannel reads tracks from raw, forwards each to the returned
// channel, and batches upserts via writer. The returned channel is closed
// when raw is exhausted and any remaining batch has been flushed.
func BridgeScanChannel(raw <-chan library.Track, writer UpsertBatcher, batchSize int) <-chan library.Track {
	ch := make(chan library.Track, scanChannelBuffer)
	go func() {
		defer close(ch)
		f := &batchFlusher{size: batchSize, flush: writer.UpsertBatch}
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
func BridgeWatchChannel(raw <-chan library.WatchEvent, writer UpsertBatcher, marker MissingMarker, hashByPath map[string]string, batchSize int) <-chan library.WatchEvent {
	// Copy the map so the goroutine owns its own mutable state and does not
	// race with callers that may still read the original map.
	localMap := make(map[string]string, len(hashByPath))
	for k, v := range hashByPath {
		localMap[k] = v
	}

	ch := make(chan library.WatchEvent, watchChannelBuffer)
	go func() {
		defer close(ch)
		f := &batchFlusher{size: batchSize, flush: writer.UpsertBatch}
		for ev := range raw {
			if ev.Deleted {
				ch <- ev
				f.drain()
				if hash, ok := localMap[ev.Path]; ok {
					if err := marker.MarkTrackMissing(hash); err != nil {
						slog.Warn("watcher: mark deleted track missing", "path", ev.Path, "err", err)
					}
				}
			} else {
				ev.Track = ensureCreatedAt(ev.Track)
				ch <- ev
				f.add(ev.Track)
				// Keep the lookup map in sync so that a file created after
				// startup and subsequently deleted can still be resolved to
				// its content hash for the MarkTrackMissing call above.
				localMap[ev.Track.Path] = ev.Track.Hash
			}
		}
		f.drain()
	}()
	return ch
}
