package app

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestBridgeWatchChannel_SkipsGenrePopulationOnUpsertFailure(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	raw <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	close(raw)

	writer := &stubGenreWriter{upsertErr: errors.New("upsert failed")}
	marker := &stubMissingMarker{}
	ch := BridgeWatchChannel(raw, writer, marker, nil, 1)

	for range ch {
	}

	if got := len(writer.populateBatches); got != 0 {
		t.Fatalf("PopulateTrackGenres calls = %d, want 0 when UpsertBatch fails", got)
	}
}

func TestBridgeWatchChannel_RetriesFailedBatchOnLaterDrain(t *testing.T) {
	raw := make(chan library.WatchEvent, 3)
	raw <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	raw <- library.WatchEvent{Track: library.Track{Hash: "b", Path: "/b.mp3"}}
	raw <- library.WatchEvent{Deleted: true, Path: "/gone.mp3"}
	close(raw)

	writer := &flakyUpsertBatcher{failures: 1, err: errors.New("transient upsert failed")}
	marker := &stubMissingMarker{}
	ch := BridgeWatchChannel(raw, writer, marker, map[string]string{"/gone.mp3": "gone-hash"}, 2)

	var got []library.WatchEvent
	for ev := range ch {
		got = append(got, ev)
	}

	if len(got) != 3 {
		t.Fatalf("forwarded events = %d, want 3", len(got))
	}
	if !got[2].Deleted || got[2].Path != "/gone.mp3" {
		t.Fatalf("final forwarded event = %+v, want delete for /gone.mp3", got[2])
	}

	if len(writer.attempts) != 2 {
		t.Fatalf("UpsertBatch attempts = %d, want 2", len(writer.attempts))
	}
	assertTrackHashes(t, writer.attempts[0], []string{"a", "b"})
	assertTrackHashes(t, writer.attempts[1], []string{"a", "b"})

	if len(writer.persisted) != 1 {
		t.Fatalf("successful upsert batches = %d, want 1", len(writer.persisted))
	}
	assertTrackHashes(t, writer.persisted[0], []string{"a", "b"})

	if len(marker.hashes) != 1 || marker.hashes[0] != "gone-hash" {
		t.Fatalf("expected missing marker called with [gone-hash], got %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_DeleteDropsFailedPendingTrack(t *testing.T) {
	raw := make(chan library.WatchEvent, 2)
	raw <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	raw <- library.WatchEvent{Deleted: true, Path: "/a.mp3"}
	close(raw)

	writer := &flakyUpsertBatcher{failures: 1, err: errors.New("transient upsert failed")}
	marker := &stubMissingMarker{}
	ch := BridgeWatchChannel(raw, writer, marker, map[string]string{"/a.mp3": "a"}, 1)

	for range ch {
	}

	if len(writer.attempts) != 1 {
		t.Fatalf("UpsertBatch attempts = %d, want 1", len(writer.attempts))
	}
	assertTrackHashes(t, writer.attempts[0], []string{"a"})

	if len(writer.persisted) != 0 {
		t.Fatalf("successful upsert batches = %d, want 0", len(writer.persisted))
	}

	if len(marker.hashes) != 1 || marker.hashes[0] != "a" {
		t.Fatalf("expected missing marker called with [a], got %v", marker.hashes)
	}
}
