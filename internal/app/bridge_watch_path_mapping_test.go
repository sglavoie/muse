package app

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestBridgeWatchChannel_NewThenDelete(t *testing.T) {
	raw := make(chan library.WatchEvent, 2)
	raw <- library.WatchEvent{Track: library.Track{Hash: "new-hash", Path: "/new.mp3"}}
	raw <- library.WatchEvent{Deleted: true, Path: "/new.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)

	for range ch {
	}

	if len(marker.hashes) != 1 || marker.hashes[0] != "new-hash" {
		t.Fatalf("expected missing marker called with [new-hash], got %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_MoveDropsOldPathMapping(t *testing.T) {
	raw := make(chan library.WatchEvent, 2)
	raw <- library.WatchEvent{Track: library.Track{Hash: "same-hash", Path: "/new.mp3"}}
	raw <- library.WatchEvent{Deleted: true, Path: "/old.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{"/old.mp3": "same-hash"}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)
	for range ch {
	}

	if len(marker.hashes) != 0 {
		t.Fatalf("expected stale old-path delete to be ignored after move, got missing hashes %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_MoveThenDeleteNewPathMarksMissing(t *testing.T) {
	raw := make(chan library.WatchEvent, 2)
	raw <- library.WatchEvent{Track: library.Track{Hash: "same-hash", Path: "/new.mp3"}}
	raw <- library.WatchEvent{Deleted: true, Path: "/new.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{"/old.mp3": "same-hash"}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)
	for range ch {
	}

	if len(marker.hashes) != 1 || marker.hashes[0] != "same-hash" {
		t.Fatalf("expected new-path delete to mark [same-hash] missing, got %v", marker.hashes)
	}
}

func TestBridgeWatchChannel_ReplacedPathMarksOldHashMissing(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	raw <- library.WatchEvent{Track: library.Track{Hash: "new-hash", Path: "/same.mp3"}}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{"/same.mp3": "old-hash"}

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)
	for range ch {
	}

	if len(marker.hashes) != 1 || marker.hashes[0] != "old-hash" {
		t.Fatalf("expected replaced path to mark old hash missing, got %v", marker.hashes)
	}

	upserted := writer.allTracks()
	assertTrackHashes(t, upserted, []string{"new-hash"})
}

func TestBridgeWatchChannel_ReplacedPendingTrackIsNotFlushed(t *testing.T) {
	raw := make(chan library.WatchEvent, 2)
	raw <- library.WatchEvent{Track: library.Track{Hash: "old-hash", Path: "/same.mp3"}}
	raw <- library.WatchEvent{Track: library.Track{Hash: "new-hash", Path: "/same.mp3"}}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}

	ch := BridgeWatchChannel(raw, writer, marker, nil, 10)
	for range ch {
	}

	if len(marker.hashes) != 1 || marker.hashes[0] != "old-hash" {
		t.Fatalf("expected replaced pending track to mark old hash missing, got %v", marker.hashes)
	}

	upserted := writer.allTracks()
	assertTrackHashes(t, upserted, []string{"new-hash"})
}

func TestBridgeWatchChannel_DoesNotMutateCallerMap(t *testing.T) {
	raw := make(chan library.WatchEvent, 1)
	raw <- library.WatchEvent{Track: library.Track{Hash: "new-hash", Path: "/new.mp3"}}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	callerMap := map[string]string{}

	ch := BridgeWatchChannel(raw, writer, marker, callerMap, 10)
	for range ch {
	}

	if _, ok := callerMap["/new.mp3"]; ok {
		t.Error("BridgeWatchChannel mutated the caller's hashByPath map; expected it to use a copy")
	}
}
