package library

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sglavoie/muse/internal/safego"
)

// extractWithBinaryRetry calls extractor.Extract for the given files. If the
// call fails (e.g. invalid JSON caused by a corrupt file poisoning the batch),
// the batch is split in half and each half is retried recursively. When a
// single-file batch fails, that file is skipped so at most one track loses
// metadata per corrupt file.
func extractWithBinaryRetry(ctx context.Context, extractor MetadataExtractor, files []HashedFile) []Track {
	if len(files) == 0 {
		return nil
	}
	tracks, err := extractor.Extract(ctx, files)
	if err == nil {
		for i := range tracks {
			// Trim whitespace-only metadata so it's treated the same as
			// missing tags (broken taggers occasionally write " " or "\t").
			tracks[i].Title = strings.TrimSpace(tracks[i].Title)
			tracks[i].Artist = strings.TrimSpace(tracks[i].Artist)
			tracks[i].Album = strings.TrimSpace(tracks[i].Album)
			tracks[i].Genre = strings.TrimSpace(tracks[i].Genre)
			if tracks[i].Title == "" {
				tracks[i].Title = filenameStem(tracks[i].Path)
			}
			if tracks[i].Artist == "" {
				tracks[i].Artist = UnknownArtist
			}
		}
		return tracks
	}
	if ctx.Err() != nil {
		// Scan was cancelled: bail without generating filename fallbacks for
		// the remaining files. The caller is shutting down and any partial
		// results would just be discarded.
		return nil
	}
	if len(files) == 1 {
		// Corrupt single file: fall back to filename-derived metadata so the
		// track is still included in the library.
		slog.Warn("metadata extraction failed, using filename fallback",
			"path", files[0].Path,
			"error", err,
		)
		return []Track{{
			Path:      files[0].Path,
			Hash:      files[0].Hash,
			Artist:    UnknownArtist,
			Title:     filenameStem(files[0].Path),
			ModTime:   files[0].ModTime,
			SizeBytes: files[0].SizeBytes,
		}}
	}
	mid := len(files) / 2
	left := extractWithBinaryRetry(ctx, extractor, files[:mid])
	right := extractWithBinaryRetry(ctx, extractor, files[mid:])
	return append(left, right...)
}

// ExtractBatches reads HashedFile values from in, groups them into batches, and
// calls extractor.Extract on each batch via binary-search retry. Track values
// are sent to the returned channel, which is closed once all batches have been
// processed. If a batch produces invalid JSON due to a corrupt file,
// binary-search subdivision isolates the corrupt file so that at most one track
// loses metadata per corrupt file.
//
// An extractor that implements concurrentExtractor has its batches extracted
// by that many workers at once, in smaller batches of concurrentBatchSize so
// even a modest scan spreads across them; tracks are then emitted in no
// particular order across batches. Any other extractor is driven by a single
// worker in batches of exiftoolBatchSize (~500).
func ExtractBatches(ctx context.Context, extractor MetadataExtractor, in <-chan HashedFile) <-chan Track {
	workers, batchSize := 1, exiftoolBatchSize
	if c, ok := extractor.(concurrentExtractor); ok {
		workers, batchSize = max(1, c.BatchConcurrency()), concurrentBatchSize
	}
	return extractBatchesN(ctx, extractor, in, workers, batchSize)
}

// extractBatchesN is ExtractBatches with the worker count and batch size made
// explicit, so tests can exercise the concurrent path with any extractor.
func extractBatchesN(ctx context.Context, extractor MetadataExtractor, in <-chan HashedFile, workers, batchSize int) <-chan Track {
	out := make(chan Track, runtime.NumCPU())
	batches := make(chan []HashedFile)

	go func() {
		defer safego.Recover("library.ExtractBatches.batcher")
		defer close(batches)
		batchHashed(ctx, in, batches, batchSize)
	}()

	var wg sync.WaitGroup
	// live counts the workers that have not yet died to a panic. The last one to
	// go has to drain batches itself, or the batcher would block forever and
	// stop draining `in` for the stages upstream.
	var live atomic.Int64
	live.Store(int64(workers))
	for range workers {
		wg.Go(func() {
			// The extractor shells out and parses third-party JSON, so this is
			// the likeliest panic site in the scan pipeline. A panic loses this
			// worker's batch; the remaining workers carry on with the rest.
			defer safego.RecoverTo("library.extractWorker", func(error) {
				if live.Add(-1) > 0 {
					return
				}
				for range batches { //nolint:revive // draining to unblock the batcher
				}
			})
			for batch := range batches {
				if ctx.Err() != nil {
					continue // drain batches so the batcher can exit
				}
				slog.Debug("processing metadata batch", "size", len(batch))
				tracks := extractWithBinaryRetry(ctx, extractor, batch)
				slog.Info("scan batch complete", "tracks", len(tracks))
				emitTracks(ctx, out, tracks)
			}
		})
	}

	go func() {
		// close(out) stays deferred: a recovered closer that left out open
		// would block the scan bridge forever, which is worse than the crash
		// it replaces.
		defer safego.Recover("library.ExtractBatches.closer")
		defer close(out)
		wg.Wait()
	}()
	return out
}

// batchHashed groups values from in into batches of batchSize and sends each
// one to batches. Once ctx is cancelled it stops sending but keeps draining in,
// so the upstream hash-worker goroutines blocked on `out <- HashedFile{...}`
// can exit and close it; returning early would leak the entire pipeline (hash
// workers + walker).
func batchHashed(ctx context.Context, in <-chan HashedFile, batches chan<- []HashedFile, batchSize int) {
	batch := make([]HashedFile, 0, batchSize)
	send := func() bool {
		if len(batch) == 0 {
			return true
		}
		select {
		case batches <- batch:
		case <-ctx.Done():
			return false
		}
		batch = make([]HashedFile, 0, batchSize)
		return true
	}
	for hf := range in {
		batch = append(batch, hf)
		if len(batch) >= batchSize && !send() {
			break
		}
	}
	if ctx.Err() == nil {
		send()
	}
	for range in { //nolint:revive // draining to unblock the hash workers
	}
}

// emitTracks sends tracks to out, giving up as soon as ctx is cancelled.
func emitTracks(ctx context.Context, out chan<- Track, tracks []Track) {
	for _, t := range tracks {
		select {
		case out <- t:
		case <-ctx.Done():
			return
		}
	}
}
