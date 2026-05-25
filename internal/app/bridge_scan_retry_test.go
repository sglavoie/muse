package app

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestBridgeScanChannel_SkipsGenrePopulationOnUpsertFailure(t *testing.T) {
	raw := make(chan library.Track, 1)
	raw <- library.Track{Hash: "a", Path: "/a.mp3"}
	close(raw)

	writer := &stubGenreWriter{upsertErr: errors.New("upsert failed")}
	ch := BridgeScanChannel(raw, writer, 1)

	for range ch {
	}

	if got := len(writer.populateBatches); got != 0 {
		t.Fatalf("PopulateTrackGenres calls = %d, want 0 when UpsertBatch fails", got)
	}
}

func TestBridgeScanChannel_RetriesFailedBatchOnLaterDrain(t *testing.T) {
	raw := make(chan library.Track, 3)
	raw <- library.Track{Hash: "a", Path: "/a.mp3"}
	raw <- library.Track{Hash: "b", Path: "/b.mp3"}
	raw <- library.Track{Hash: "c", Path: "/c.mp3"}
	close(raw)

	writer := &flakyUpsertBatcher{failures: 1, err: errors.New("transient upsert failed")}
	ch := BridgeScanChannel(raw, writer, 2)

	var got []library.Track
	for track := range ch {
		got = append(got, track)
	}

	assertTrackHashes(t, got, []string{"a", "b", "c"})

	if len(writer.attempts) != 2 {
		t.Fatalf("UpsertBatch attempts = %d, want 2", len(writer.attempts))
	}
	assertTrackHashes(t, writer.attempts[0], []string{"a", "b"})
	assertTrackHashes(t, writer.attempts[1], []string{"a", "b", "c"})

	if len(writer.persisted) != 1 {
		t.Fatalf("successful upsert batches = %d, want 1", len(writer.persisted))
	}
	assertTrackHashes(t, writer.persisted[0], []string{"a", "b", "c"})
}

func TestBridgeScanChannel_CapsRetriedBatchGrowth(t *testing.T) {
	raw := make(chan library.Track, 5)
	for _, hash := range []string{"a", "b", "c", "d", "e"} {
		raw <- library.Track{Hash: hash, Path: "/" + hash + ".mp3"}
	}
	close(raw)

	writer := &flakyUpsertBatcher{failures: 5, err: errors.New("persistent upsert failure")}
	ch := BridgeScanChannel(raw, writer, 2)

	for range ch {
	}

	if len(writer.attempts) != 5 {
		t.Fatalf("UpsertBatch attempts = %d, want 5", len(writer.attempts))
	}

	maxBatchSize := 0
	for i, attempt := range writer.attempts {
		if got := len(attempt); got > maxBatchSize {
			maxBatchSize = got
		}
		if got := attempt[len(attempt)-1].Hash; got == "e" {
			t.Fatalf("attempt %d unexpectedly retained dropped track e: %v", i, attempt)
		}
	}

	if maxBatchSize != 4 {
		t.Fatalf("max retained batch size = %d, want 4", maxBatchSize)
	}
}
