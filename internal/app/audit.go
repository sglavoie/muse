package app

import (
	"context"
	"log/slog"
	"math"
	"runtime"
	"sync"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// DurationReader reads tracks needing duration verification.
type DurationReader interface {
	ReadUnverifiedDurations() ([]db.UnverifiedTrack, error)
}

// DurationVerifier updates or confirms track durations.
type DurationVerifier interface {
	VerifyTrackDuration(hash string, durationMs int64) error
	MarkDurationVerified(hash string) error
}

// probeFn is the function used to measure a file's duration in seconds.
// Tests may replace this with a stub to avoid spawning real ffprobe processes.
var (
	probeFnMu sync.RWMutex
	probeFn   = library.FFProbeDuration
)

// RunDurationAudit spawns a background goroutine that ffprobe-verifies all
// tracks where duration_verified=0. Corrections are sent to the returned
// channel; the channel is closed when the audit completes.
func RunDurationAudit(ctx context.Context, reader DurationReader, writer DurationVerifier) <-chan library.DurationFix {
	out := make(chan library.DurationFix)

	go func() {
		defer close(out)

		tracks, err := reader.ReadUnverifiedDurations()
		if err != nil {
			slog.Warn("duration audit: read unverified durations", "err", err)
			return
		}
		if len(tracks) == 0 {
			slog.Info("duration audit complete", "checked", 0, "corrected", 0)
			return
		}

		work := make(chan db.UnverifiedTrack, len(tracks))
		for _, t := range tracks {
			work <- t
		}
		close(work)

		type result struct {
			hash       string
			durationMs int64
			verified   bool // true = confirm existing duration; false = correct it
		}

		results := make(chan result, len(tracks))
		n := runtime.NumCPU()

		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				for t := range work {
					// Bail out promptly on cancellation rather than burning
					// CPU on remaining work items during shutdown.
					if ctx.Err() != nil {
						return
					}
					probeFnMu.RLock()
					probe := probeFn
					probeFnMu.RUnlock()
					dur, err := probe(ctx, t.Path)
					if err != nil {
						slog.Warn("duration audit: ffprobe failed, skipping", "path", t.Path, "err", err)
						continue
					}
					probedMs := int64(math.Round(dur * 1000))
					diff := probedMs - t.DurationMs
					if diff < 0 {
						diff = -diff
					}
					if diff > 2000 {
						results <- result{hash: t.Hash, durationMs: probedMs}
					} else {
						results <- result{hash: t.Hash, verified: true}
					}
				}
			})
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		checked := 0
		corrected := 0
		for r := range results {
			checked++
			if r.verified {
				if err := writer.MarkDurationVerified(r.hash); err != nil {
					slog.Warn("duration audit: mark verified", "hash", r.hash, "err", err)
				}
				continue
			}
			if err := writer.VerifyTrackDuration(r.hash, r.durationMs); err != nil {
				slog.Warn("duration audit: verify track duration", "hash", r.hash, "err", err)
				continue
			}
			corrected++
			select {
			case out <- library.DurationFix{Hash: r.hash, DurationMs: r.durationMs}:
			case <-ctx.Done():
				return
			}
		}

		slog.Info("duration audit complete", "checked", checked, "corrected", corrected)
	}()

	return out
}
