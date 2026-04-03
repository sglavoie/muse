package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestScanBatchPreservesUserEditedOverlay verifies that handleScanBatchMsg
// uses the userEditedOverlay to preserve manually edited metadata instead
// of overwriting with raw scanned values.
func TestScanBatchPreservesUserEditedOverlay(t *testing.T) {
	m := New(context.Background())
	m.userEditedOverlay = map[string]db.UserEditedRow{
		"hash1": {Hash: "hash1", Title: "Edited Title", Artist: "Edited Artist", Album: "Edited Album", Genre: "Edited Genre"},
	}

	scanned := []library.Track{
		{Hash: "hash1", Path: "/a.mp3", Title: "Scanned Title", Artist: "Scanned Artist", Album: "Scanned Album", Genre: "Scanned Genre"},
		{Hash: "hash2", Path: "/b.mp3", Title: "B Title", Artist: "B Artist", Album: "B Album", Genre: "B Genre"},
	}

	next, _ := m.Update(library.ScanBatchMsg{Tracks: scanned})
	nm := next.(Model)

	// Track with user-edited overlay should have edited metadata.
	t1, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if t1.Title != "Edited Title" {
		t.Errorf("hash1 Title = %q; want Edited Title", t1.Title)
	}
	if t1.Artist != "Edited Artist" {
		t.Errorf("hash1 Artist = %q; want Edited Artist", t1.Artist)
	}
	if t1.Album != "Edited Album" {
		t.Errorf("hash1 Album = %q; want Edited Album", t1.Album)
	}
	if t1.Genre != "Edited Genre" {
		t.Errorf("hash1 Genre = %q; want Edited Genre", t1.Genre)
	}

	// Track without overlay should have scanned metadata.
	t2, ok := nm.library.index.FindByHash("hash2")
	if !ok {
		t.Fatal("hash2 not found in index")
	}
	if t2.Title != "B Title" {
		t.Errorf("hash2 Title = %q; want B Title", t2.Title)
	}
}

func TestScanBatchStampsCreatedAtForNewTrack(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{{Hash: "hash1", Path: "/a.mp3", Title: "A"}}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if track.CreatedAt == 0 {
		t.Fatal("expected created_at to be stamped for new scan track")
	}
}

// TestRescanSnapshotPreservesCreatedAtAndDuration verifies that a full rescan
// (which clears the index) still preserves created_at and duration via the
// pre-rescan snapshot.
func TestRescanSnapshotPreservesCreatedAtAndDuration(t *testing.T) {
	m := New(context.Background())

	// Populate the index with a track that has created_at and duration set.
	m.library.index.Insert(library.Track{
		Hash: "hash1", Path: "/a.mp3", Title: "A",
		CreatedAt: 1000, Duration: 120.5,
	})

	// Build snapshot (simulating what actionRescan does before clearing).
	tracks := m.library.index.Tracks()
	snap := make(map[string]rescanEntry, len(tracks))
	for _, track := range tracks {
		snap[track.Hash] = rescanEntry{CreatedAt: track.CreatedAt, Duration: track.Duration}
	}
	m.rescanSnapshot = snap

	// Clear the index (simulating rescan).
	m.library.index = library.LibraryIndex{}

	// Deliver a scan batch with CreatedAt=0 and Duration=0 (scanner defaults).
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: "hash1", Path: "/a.mp3", Title: "A", CreatedAt: 0, Duration: 0},
	}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if track.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d; want 1000 (should be preserved from snapshot)", track.CreatedAt)
	}
	if track.Duration != 120.5 {
		t.Errorf("Duration = %f; want 120.5 (should be preserved from snapshot)", track.Duration)
	}
}

// TestRescanSnapshotAllowsNonZeroDurationUpdate verifies that when a rescan
// provides a non-zero duration, it takes precedence over the snapshot value.
func TestRescanSnapshotAllowsNonZeroDurationUpdate(t *testing.T) {
	m := New(context.Background())

	m.library.index.Insert(library.Track{
		Hash: "hash1", Path: "/a.mp3", Title: "A",
		CreatedAt: 1000, Duration: 60.0,
	})

	tracks := m.library.index.Tracks()
	snap := make(map[string]rescanEntry, len(tracks))
	for _, track := range tracks {
		snap[track.Hash] = rescanEntry{CreatedAt: track.CreatedAt, Duration: track.Duration}
	}
	m.rescanSnapshot = snap
	m.library.index = library.LibraryIndex{}

	// Scanner found a new duration.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: "hash1", Path: "/a.mp3", Title: "A", CreatedAt: 0, Duration: 90.0},
	}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("hash1")
	if !ok {
		t.Fatal("hash1 not found in index")
	}
	if track.Duration != 90.0 {
		t.Errorf("Duration = %f; want 90.0 (non-zero scanned duration should take precedence)", track.Duration)
	}
}
