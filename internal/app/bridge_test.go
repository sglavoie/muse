package app

import (
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// stubUpsertBatcher records all batches passed to UpsertBatch.
type stubUpsertBatcher struct {
	mu      sync.Mutex
	batches [][]library.Track
}

func (s *stubUpsertBatcher) UpsertBatch(tracks []library.Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]library.Track, len(tracks))
	copy(cp, tracks)
	s.batches = append(s.batches, cp)
	return nil
}

func (s *stubUpsertBatcher) allTracks() []library.Track {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []library.Track
	for _, b := range s.batches {
		all = append(all, b...)
	}
	return all
}

// stubMissingMarker records hashes marked as missing.
type stubMissingMarker struct {
	mu     sync.Mutex
	hashes []string
}

func (s *stubMissingMarker) MarkTrackMissing(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes = append(s.hashes, hash)
	return nil
}

func TestBridgeScanChannel(t *testing.T) {
	raw := make(chan library.Track, 3)
	raw <- library.Track{Hash: "a", Path: "/a.mp3"}
	raw <- library.Track{Hash: "b", Path: "/b.mp3"}
	raw <- library.Track{Hash: "c", Path: "/c.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	ch := BridgeScanChannel(raw, writer, 2)

	var got []library.Track
	for t := range ch {
		got = append(got, t)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 forwarded tracks, got %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Hash != want {
			t.Errorf("track %d: got hash %q, want %q", i, got[i].Hash, want)
		}
	}

	upserted := writer.allTracks()
	if len(upserted) != 3 {
		t.Fatalf("expected 3 upserted tracks, got %d", len(upserted))
	}
}

func TestBridgeScanChannel_EmptyInput(t *testing.T) {
	raw := make(chan library.Track)
	close(raw)

	writer := &stubUpsertBatcher{}
	ch := BridgeScanChannel(raw, writer, 10)

	var got []library.Track
	for t := range ch {
		got = append(got, t)
	}

	if len(got) != 0 {
		t.Fatalf("expected 0 tracks, got %d", len(got))
	}
	if len(writer.batches) != 0 {
		t.Fatalf("expected 0 batches, got %d", len(writer.batches))
	}
}

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

	// Check upserted tracks (only non-deleted events).
	upserted := writer.allTracks()
	if len(upserted) != 2 {
		t.Fatalf("expected 2 upserted tracks, got %d", len(upserted))
	}

	// Check missing marker was called for the deleted path.
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

// TestBridgeWatchChannel_ClosedInput verifies that BridgeWatchChannel
// terminates cleanly when the input channel is already closed (the fallback
// used when the filesystem watcher fails to start). Without this, a nil
// channel would block the goroutine forever.
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

// TestBridgeWatchChannel_NewThenDelete verifies that a file discovered by the
// watcher and then deleted within the same session is correctly marked missing.
// Before the fix, hashByPath was never updated with new tracks, so the
// deletion lookup would miss the hash.
func TestBridgeWatchChannel_NewThenDelete(t *testing.T) {
	raw := make(chan library.WatchEvent, 2)
	// First: watcher discovers a new file.
	raw <- library.WatchEvent{Track: library.Track{Hash: "new-hash", Path: "/new.mp3"}}
	// Second: the same file is deleted.
	raw <- library.WatchEvent{Deleted: true, Path: "/new.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	marker := &stubMissingMarker{}
	hashByPath := map[string]string{} // initially empty — file didn't exist at startup

	ch := BridgeWatchChannel(raw, writer, marker, hashByPath, 10)

	for range ch {
	}

	// The bridge should have updated hashByPath when it saw the new track,
	// so the subsequent deletion can resolve the hash and mark it missing.
	if len(marker.hashes) != 1 || marker.hashes[0] != "new-hash" {
		t.Fatalf("expected missing marker called with [new-hash], got %v", marker.hashes)
	}
}

// TestBridgeWatchChannel_CreatedAtSetBeforeForward verifies that the TUI
// channel receives watch events with CreatedAt already populated (via
// ensureCreatedAt), not the raw zero value. Before the fix, the event was
// forwarded to the channel before ensureCreatedAt was applied.
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

// TestBridgeWatchChannel_DoesNotMutateCallerMap verifies that
// BridgeWatchChannel copies the hashByPath map instead of writing to the
// caller's map. Before the fix, the goroutine wrote new entries into the
// shared map, which could race with concurrent readers on the caller side.
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

// TestBridgeScanChannel_CreatedAtSetBeforeForward verifies that the TUI
// channel receives scan tracks with CreatedAt already populated.
func TestBridgeScanChannel_CreatedAtSetBeforeForward(t *testing.T) {
	raw := make(chan library.Track, 1)
	raw <- library.Track{Hash: "s1", Path: "/s1.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	ch := BridgeScanChannel(raw, writer, 10)

	track := <-ch
	if track.CreatedAt == 0 {
		t.Error("TUI channel received track with CreatedAt=0; ensureCreatedAt was not applied before forwarding")
	}
}
