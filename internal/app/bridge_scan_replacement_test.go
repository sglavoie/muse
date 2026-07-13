package app

import (
	"context"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// recordingLibraryWriter satisfies both UpsertBatcher and MissingMarker so a
// test can assert what the scan bridge upserts *and* which displaced hashes it
// durably marks missing. It is mutex-guarded because the bridge persists from a
// background goroutine that -race observes concurrently with the assertions.
type recordingLibraryWriter struct {
	mu      sync.Mutex
	upserts [][]library.Track
	missing []string
}

func (w *recordingLibraryWriter) UpsertBatch(tracks []library.Track) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]library.Track, len(tracks))
	copy(cp, tracks)
	w.upserts = append(w.upserts, cp)
	return nil
}

func (w *recordingLibraryWriter) MarkTrackMissing(hash string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.missing = append(w.missing, hash)
	return nil
}

func (w *recordingLibraryWriter) upsertedHashes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, b := range w.upserts {
		for _, t := range b {
			out = append(out, t.Hash)
		}
	}
	return out
}

func (w *recordingLibraryWriter) missingHashes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, len(w.missing))
	copy(out, w.missing)
	return out
}

// TestBridgeScanChannel_ContentReplacementMarksDisplacedMissing pins the DB
// half of FR-011: a startup scan that finds a *different* hash at a cached path
// (an in-place content replacement) must mark the displaced old identity
// missing durably — not silently leave it active — while upserting the new
// hash. Before the fix the scan bridge discarded register's displaced return,
// so the old and new hashes both stayed active for one path.
func TestBridgeScanChannel_ContentReplacementMarksDisplacedMissing(t *testing.T) {
	// The DB cached old content at this path (registry seed = path→old hash).
	registry := newPathHashRegistry(map[string]string{"/song.mp3": "old-hash"})

	raw := make(chan library.Track, 1)
	raw <- library.Track{Hash: "new-hash", Path: "/song.mp3"}
	close(raw)

	writer := &recordingLibraryWriter{}
	ch, done := bridgeScanChannel(context.Background(), raw, writer, registry, 10)

	var forwarded []library.Track
	for tr := range ch {
		forwarded = append(forwarded, tr)
	}
	<-done

	// The displaced old identity is marked missing durably, exactly once.
	if got := writer.missingHashes(); len(got) != 1 || got[0] != "old-hash" {
		t.Fatalf("expected displaced [old-hash] marked missing, got %v", got)
	}
	// The new identity is upserted; the old identity is not re-upserted.
	if got := writer.upsertedHashes(); len(got) != 1 || got[0] != "new-hash" {
		t.Fatalf("expected only [new-hash] upserted, got %v", got)
	}
	// The new content is still forwarded to the TUI so it appears in the index.
	if len(forwarded) != 1 || forwarded[0].Hash != "new-hash" {
		t.Fatalf("expected new-hash forwarded to TUI, got %v", forwarded)
	}
	// The path now resolves to the new identity so a later deletion is correct.
	if hash, ok := registry.remove("/song.mp3"); !ok || hash != "new-hash" {
		t.Fatalf("expected /song.mp3 to resolve to new-hash, got (%q, %v)", hash, ok)
	}
}

// TestBridgeScanChannel_NewDiscoveryMarksNothing is the negative control: a scan
// discovery at a brand-new path (no cached hash to displace) must not mark any
// hash missing. Only a genuine same-path different-hash replacement triggers the
// durable mark; a plain new file or a same-hash move does not.
func TestBridgeScanChannel_NewDiscoveryMarksNothing(t *testing.T) {
	registry := newPathHashRegistry(nil)

	raw := make(chan library.Track, 2)
	raw <- library.Track{Hash: "a", Path: "/a.mp3"}
	raw <- library.Track{Hash: "b", Path: "/b.mp3"}
	close(raw)

	writer := &recordingLibraryWriter{}
	ch, done := bridgeScanChannel(context.Background(), raw, writer, registry, 10)
	for range ch {
	}
	<-done

	if got := writer.missingHashes(); len(got) != 0 {
		t.Fatalf("brand-new discoveries must mark nothing missing, got %v", got)
	}
}
