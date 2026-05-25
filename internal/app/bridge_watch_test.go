package app

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestBridgeWatchChannel(t *testing.T) {
	raw := make(chan library.WatchEvent, 3)
	raw <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	raw <- library.WatchEvent{Deleted: true, Path: "/b.mp3"}
	raw <- library.WatchEvent{Track: library.Track{Hash: "c", Path: "/c.mp3"}}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{"/b.mp3": "b-hash"}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)

	var got []library.WatchEvent
	for ev := range ch {
		got = append(got, ev)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 forwarded events, got %d", len(got))
	}

	upserted := writer.allTracks()
	if len(upserted) != 2 {
		t.Fatalf("expected 2 upserted tracks, got %d", len(upserted))
	}

	if len(marker.hashes) != 1 || marker.hashes[0] != "b-hash" {
		t.Fatalf("expected missing marker called with [b-hash], got %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_DeleteUnknownPath(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	raw <- library.WatchEvent{Deleted: true, Path: "/unknown.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)

	for range ch {
	}

	if len(marker.hashes) != 0 {
		t.Fatalf("expected no missing markers for unknown path, got %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_ClosedInput(t *testing.T) {
	raw := make(chan library.WatchEvent)
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	ch := BridgeWatchChannel(raw, writer, marker, nil, 10)

	var got []library.WatchEvent
	for ev := range ch {
		got = append(got, ev)
	}

	if len(got) != 0 {
		t.Fatalf("expected 0 events, got %d", len(got))
	}
	if len(writer.batches) != 0 {
		t.Fatalf("expected 0 batches, got %d", len(writer.batches))
	}
	if len(marker.hashes) != 0 {
		t.Fatalf("expected 0 missing markers, got %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_CreatedAtSetBeforeForward(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	raw <- library.WatchEvent{Track: library.Track{Hash: "t1", Path: "/t1.mp3"}}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)

	ev := <-ch
	if ev.Track.CreatedAt == 0 {
		t.Error("TUI channel received event with CreatedAt=0; ensureCreatedAt was not applied before forwarding")
	}
}
