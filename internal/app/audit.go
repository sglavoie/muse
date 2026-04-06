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
var probeFn = library.FFProbeDuration

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
			fix       *library.DurationFix // non-nil means correction needed
			verified  bool                 // true means mark as verified without fix
		}

		results := make(chan result, len(tracks))
		n := runtime.NumCPU()

		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				for t := range work {
					dur, err := probeFn(ctx, t.Path)
					if err != nil {
						slog.Debug("duration audit: ffprobe failed, skipping", "path", t.Path, "err", err)
						continue
					}
					probedMs := int64(math.Round(dur * 1000))
					diff := probedMs - t.DurationMs
					if diff < 0 {
						diff = -diff
					}
					if diff > 2000 {
						results <- result{fix: &library.DurationFix{Hash: t.Hash, DurationMs: probedMs}}
					} else {
						results <- result{verified: true, fix: &library.DurationFix{Hash: t.Hash}}
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
			if r.fix != nil && r.verified {
				if err := writer.MarkDurationVerified(r.fix.Hash); err != nil {
					slog.Warn("duration audit: mark verified", "hash", r.fix.Hash, "err", err)
				}
			} else if r.fix != nil {
				if err := writer.VerifyTrackDuration(r.fix.Hash, r.fix.DurationMs); err != nil {
					slog.Warn("duration audit: verify track duration", "hash", r.fix.Hash, "err", err)
					continue
				}
				corrected++
				select {
				case out <- *r.fix:
				case <-ctx.Done():
					return
				}
			}
		}

		slog.Info("duration audit complete", "checked", checked, "corrected", corrected)
	}()

	return out
}
