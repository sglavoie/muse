package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// Scale properties of the refill path.
//
// The planner's own cost is benchmarked in internal/radio. What can only be
// checked here is what the refill asks storage for and where it runs: a batch
// planned from a 10,000-track library must not issue more reads than one planned
// from 1,000 tracks, and neither may run on the update loop, because everything
// the user does — including turning the DJ off — waits behind it.

// djScaleTracks generates size candidates with both pools populated and enough
// artist spread that the per-artist cap stays in play.
func djScaleTracks(size int) []library.Track {
	recent := djFixedNow.AddDate(0, 0, -2).Unix()
	genres := []string{"Rock", "Jazz", "Folk", "Ambient"}
	tracks := make([]library.Track, size)
	for i := range size {
		hash := fmt.Sprintf("%064x", i)
		t := library.Track{
			Hash:     hash,
			Path:     "/scale/" + hash + ".flac",
			Artist:   fmt.Sprintf("Artist %03d", i%120),
			Title:    fmt.Sprintf("Track %06d", i),
			Album:    fmt.Sprintf("Album %04d", i%240),
			Genre:    genres[i%len(genres)],
			Duration: 180 + float64(i%90),
		}
		// Half familiar, half never played, so neither pool can starve and the
		// hydration budget has to be split between them.
		if i%2 == 0 {
			t.PlayCount = 3 + i%9
			t.LastPlayedAt = recent
		}
		tracks[i] = t
	}
	return tracks
}

// djScaleModel returns an active session over a generated library, drained to
// the point where a refill fires, with the first track playing.
func djScaleModel(size int, reader recommendationReader, writer *stubDJQueueWriter) Model {
	tracks := djScaleTracks(size)
	opts := []Option{WithLibraryTracks(tracks), WithRecommendationReader(reader)}
	if writer != nil {
		opts = append(opts, WithDJQueueWriter(writer))
	}
	m := New(context.Background(), opts...)
	seed := tracks[0]
	m.radio.enabled = true
	m.queue.Entries = []queue.Entry{{
		ID: 1, Hash: seed.Hash, Path: seed.Path, Artist: seed.Artist, Title: seed.Title,
	}}
	m.playback.currentIndex = 0
	m.playback.currentHash = seed.Hash
	m.playback.state = playback.StatePlaying
	m.dj.active = true
	m.dj.seed = radio.CaptureSeed(seed, nil, radio.FixedClock(djFixedNow))
	return m
}

// TestDJQueryCountIsIndependentOfLibrarySize records the per-batch read budget
// at two library scales.
//
// Three reads is the whole budget: completion rates for the hydration targets,
// this hour's genre affinities, and the seed's co-occurrence. The hash list those
// rates are asked for is bounded by the per-pool hydration budget, so it too has
// to be identical at both sizes. Anything that grew with the library would mean a
// user's refills got slower simply by owning more music.
func TestDJQueryCountIsIndependentOfLibrarySize(t *testing.T) {
	type evidence struct {
		hashes  int
		reads   int
		elapsed time.Duration
	}
	seen := make(map[int]evidence, 2)

	for _, size := range []int{1_000, 10_000} {
		t.Run(fmt.Sprintf("%d tracks", size), func(t *testing.T) {
			reader := newDJRecReader()
			m := djScaleModel(size, reader, nil)
			in := m.captureDJInputs(djFixedNow)
			req := djTestRequest()
			req.Seed = m.dj.seed

			start := time.Now()
			result, signals, err := djPlanBatch(in, req, radio.Allocation{}, reader, djFixedNow)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("planning %d candidates failed: %v", size, err)
			}
			if len(result.Tracks) != radioBatchSize {
				t.Fatalf("planned %d tracks from %d candidates, want a full batch of %d",
					len(result.Tracks), size, radioBatchSize)
			}
			if signals.Degraded() {
				t.Fatalf("a reader that answered everything reported degradation: %v", signals.Failures())
			}

			completion, hours, coOcc := reader.snapshot()
			reads := len(completion) + hours + len(coOcc)
			if len(completion) != 1 || hours != 1 || len(coOcc) != 1 {
				t.Fatalf("reads per batch: %d completion, %d hour, %d co-occurrence; want 1 each",
					len(completion), hours, len(coOcc))
			}
			if max := 2*djHydrationPerGroup + 1; len(completion[0]) > max {
				t.Fatalf("hydrated %d hashes, over the %d ceiling for two pools", len(completion[0]), max)
			}
			seen[size] = evidence{hashes: len(completion[0]), reads: reads, elapsed: elapsed}
			t.Logf("%d candidates: %d reads, %d hydrated hashes, planned in %s",
				size, reads, len(completion[0]), elapsed)
		})
	}

	small, large := seen[1_000], seen[10_000]
	if small.reads != large.reads {
		t.Errorf("read count grew with the library: %d at 1k, %d at 10k", small.reads, large.reads)
	}
	if small.hashes != large.hashes {
		t.Errorf("hydrated hash count grew with the library: %d at 1k, %d at 10k", small.hashes, large.hashes)
	}
}

// blockingRecReader holds the first completion-rate read open until it is
// released, which parks a planning run mid-flight on purpose.
type blockingRecReader struct {
	*djCountingRecReader
	entered chan struct{}
	release chan struct{}
}

func newBlockingRecReader() *blockingRecReader {
	return &blockingRecReader{
		djCountingRecReader: newDJRecReader(),
		entered:             make(chan struct{}, 1),
		release:             make(chan struct{}),
	}
}

func (r *blockingRecReader) ReadCompletionRates(hashes []string) (map[string]float64, error) {
	select {
	case r.entered <- struct{}{}:
	default:
	}
	<-r.release
	return r.djCountingRecReader.ReadCompletionRates(hashes)
}

// TestDJPlanningStaysOffTheUpdateLoop is the two halves of the same contract.
//
// Launching a refill must not plan anything: startRadioEnqueueCmd returns a
// command, and until the runtime runs it no query has been issued. And while that
// command is parked inside a behavioral read, the update loop must still answer —
// turning the DJ off is the gesture a user reaches for precisely when a refill is
// taking too long, so it cannot be the one gesture that waits for it.
func TestDJPlanningStaysOffTheUpdateLoop(t *testing.T) {
	reader := newBlockingRecReader()
	writer := &stubDJQueueWriter{}
	m := djScaleModel(10_000, reader, writer)

	m, cmd := m.startRadioEnqueueCmd()
	if cmd == nil {
		t.Fatal("no refill launched from a drained queue")
	}
	if completion, hours, coOcc := reader.snapshot(); len(completion) != 0 || hours != 0 || len(coOcc) != 0 {
		t.Fatalf("planning ran on the update loop: %d completion, %d hour, %d co-occurrence reads already issued",
			len(completion), hours, len(coOcc))
	}
	if !m.radio.enqueuePending {
		t.Fatal("the launched refill was not recorded as pending")
	}

	done := make(chan RadioBatchEnqueuedMsg, 1)
	go func() {
		msg, _ := cmd().(RadioBatchEnqueuedMsg)
		done <- msg
	}()

	select {
	case <-reader.entered:
	case <-time.After(10 * time.Second):
		close(reader.release)
		t.Fatal("the refill never reached its behavioral read")
	}

	// The plan is now parked. The loop has to stay usable.
	start := time.Now()
	next, _, _ := m.radioToggle()
	elapsed := time.Since(start)
	cancelled := next.(Model)
	if elapsed > 2*time.Second {
		close(reader.release)
		t.Fatalf("turning the DJ off waited %s on the in-flight plan", elapsed)
	}
	if cancelled.radio.enabled {
		t.Error("the toggle left radio enabled")
	}
	if cancelled.radio.enqueuePending {
		t.Error("the toggle left the superseded refill pending")
	}
	t.Logf("cancelled a parked 10k-candidate refill in %s", elapsed)

	close(reader.release)
	msg := <-done

	// The parked plan still finishes and still prepared its rows hidden; the
	// cancelled model must reject them rather than publish them late.
	before := len(cancelled.queue.Entries)
	cancelled, _ = cancelled.handleRadioBatchEnqueuedMsg(msg)
	if got := len(cancelled.queue.Entries); got != before {
		t.Errorf("the late batch entered a cancelled session's queue: %d entries, want %d", got, before)
	}
	if len(writer.accepted) != 0 {
		t.Errorf("a cancelled batch was published: %v", writer.accepted)
	}
	if len(writer.prepared) != 1 || len(writer.rejected) != 1 {
		t.Errorf("prepared %d and rejected %d batches, want one prepared and the same one rejected",
			len(writer.prepared), len(writer.rejected))
	}
}
