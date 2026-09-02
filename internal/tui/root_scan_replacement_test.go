package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestScanReplacesInPlaceContentAndMarksDisplacedQueueMissing pins the
// in-memory half of FR-011: a startup scan that discovers a different hash at a
// cached path (in-place content replacement) must replace the stale index entry
// rather than display both identities, and must mark every queue instance of
// the displaced identity unavailable so it cannot play the replacement content
// under stale metadata. The durable tracks.missing mark is covered by the app
// scan-bridge test; here we drive the ScanBatchMsg the bridge forwards.
func TestScanReplacesInPlaceContentAndMarksDisplacedQueueMissing(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "song.mp3")
	otherPath := filepath.Join(root, "other.mp3")
	const oldHash = "hold"
	const newHash = "hnew"
	const otherHash = "hother"

	m := New(context.Background(),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
	)

	// Cached-first startup state: the index holds the old content at the path,
	// plus an unrelated track that must stay untouched by the replacement.
	m.library.Index.Insert(library.Track{Hash: oldHash, Path: path, Title: "Old", Artist: "A"})
	m.library.Index.Insert(library.Track{Hash: otherHash, Path: otherPath, Title: "Other", Artist: "B"})

	// The queue holds the doomed content twice and the unrelated track once, so
	// we prove EVERY instance of the displaced identity becomes unavailable.
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: oldHash, Path: path, Title: "Old", Artist: "A"},
		{ID: 2, Hash: oldHash, Path: path, Title: "Old", Artist: "A"},
		{ID: 3, Hash: otherHash, Path: otherPath, Title: "Other", Artist: "B"},
	}

	// The startup diff scan reports the path now holds new content (new hash).
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: newHash, Path: path, Title: "New", Artist: "A"},
	}})
	m = next.(Model)

	// Index shows only the new identity at the path — not both.
	if got, ok := m.library.Index.FindByPath(path); !ok || got.Hash != newHash {
		t.Fatalf("path %s should resolve to new hash, got (%q, ok=%v)", path, got.Hash, ok)
	}
	if _, ok := m.library.Index.FindByHash(oldHash); ok {
		t.Error("displaced old hash must be removed from the index")
	}
	if _, ok := m.library.Index.FindByHash(newHash); !ok {
		t.Error("new hash must be present in the index")
	}
	if _, ok := m.library.Index.FindByHash(otherHash); !ok {
		t.Error("unrelated track must remain in the index")
	}

	// Both queue instances of the displaced identity are unavailable; the
	// unrelated instance stays available.
	if !m.queue.entries[0].Missing || !m.queue.entries[1].Missing {
		t.Errorf("displaced queue instances must be missing, got %v / %v",
			m.queue.entries[0].Missing, m.queue.entries[1].Missing)
	}
	if m.queue.entries[2].Missing {
		t.Error("unrelated queue instance must stay available")
	}
}

// TestScanSameHashMoveDoesNotMarkQueueMissing is the negative control: a
// same-hash move (same content at a new path) is healed by the reconcile
// fan-out and must not be mistaken for a content replacement, so no queue
// instance is marked missing.
func TestScanSameHashMoveDoesNotMarkQueueMissing(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.mp3")
	newPath := filepath.Join(root, "new.mp3")
	const hash = "hmove"

	m := New(context.Background(),
		WithLibraryRoots([]string{root}),
		WithExtractor(stubExtractor{}),
	)
	m.library.Index.Insert(library.Track{Hash: hash, Path: oldPath, Title: "Song", Artist: "A"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: hash, Path: oldPath, Title: "Song", Artist: "A"},
	}

	// The scan discovers the same content at a new path (a move/rename).
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: hash, Path: newPath, Title: "Song", Artist: "A"},
	}})
	m = next.(Model)

	// The single index entry is re-pointed at the new path (no duplicate).
	if got, ok := m.library.Index.FindByHash(hash); !ok || got.Path != newPath {
		t.Fatalf("moved track should resolve to new path, got (%q, ok=%v)", got.Path, ok)
	}
	if m.library.Index.Len() != 1 {
		t.Fatalf("move must not duplicate the index entry, got %d", m.library.Index.Len())
	}
	// The queue instance is healed to the new path and stays available.
	if m.queue.entries[0].Missing {
		t.Error("a same-hash move must not mark the queue instance missing")
	}
	if m.queue.entries[0].Path != newPath {
		t.Errorf("queue instance should be healed to new path, got %q", m.queue.entries[0].Path)
	}
}
