package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/tui"
)

// TestRestoreSessionEmpty verifies that RestoreSession on an empty database
// returns a non-nil RestoredSession with no entries and default flags.
func TestRestoreSessionEmpty(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if restored == nil {
		t.Fatal("RestoreSession returned nil")
	}
	if len(restored.QueueEntries) != 0 {
		t.Errorf("QueueEntries: want 0, got %d", len(restored.QueueEntries))
	}
	if len(restored.PlayerTracks) != 0 {
		t.Errorf("PlayerTracks: want 0, got %d", len(restored.PlayerTracks))
	}
	if restored.RepeatMode != tui.RepeatOff {
		t.Errorf("RepeatMode: want RepeatOff, got %v", restored.RepeatMode)
	}
	if restored.Shuffle {
		t.Error("Shuffle: want false")
	}
	if restored.CurrentHash != "" {
		t.Errorf("CurrentHash: want empty, got %q", restored.CurrentHash)
	}
}

// TestRestoreSessionReadableFiles verifies that queue entries with readable
// file paths are returned with Missing=false.
func TestRestoreSessionReadableFiles(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	audioPath := filepath.Join(dirs.Runtime, "track.mp3")
	if err := os.WriteFile(audioPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	insertTrack(t, database, "hash-aaa", audioPath, "Artist A", "Song A")
	insertQueueEntry(t, database, "hash-aaa", 1000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	e := restored.QueueEntries[0]
	if e.Missing {
		t.Error("QueueEntry.Missing: want false for readable file")
	}
	if e.Artist != "Artist A" {
		t.Errorf("Artist: want %q, got %q", "Artist A", e.Artist)
	}
	if e.Title != "Song A" {
		t.Errorf("Title: want %q, got %q", "Song A", e.Title)
	}
	if e.Display != "Artist A – Song A" {
		t.Errorf("Display: want %q, got %q", "Artist A – Song A", e.Display)
	}
	if len(restored.PlayerTracks) != 1 {
		t.Fatalf("PlayerTracks: want 1, got %d", len(restored.PlayerTracks))
	}
	if restored.PlayerTracks[0].Hash != "hash-aaa" {
		t.Errorf("PlayerTracks[0].Hash: want %q, got %q", "hash-aaa", restored.PlayerTracks[0].Hash)
	}
}

// TestRestoreSessionMissingFiles verifies that queue entries with unreadable
// file paths have Missing=true in the returned QueueEntry slice.
func TestRestoreSessionMissingFiles(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	missingPath := filepath.Join(dirs.Runtime, "nonexistent.mp3")

	insertTrack(t, database, "hash-bbb", missingPath, "Artist B", "Song B")
	insertQueueEntry(t, database, "hash-bbb", 1000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if !restored.QueueEntries[0].Missing {
		t.Error("QueueEntry.Missing: want true for unreadable path")
	}
	if len(restored.PlayerTracks) != 1 {
		t.Fatalf("PlayerTracks: want 1, got %d", len(restored.PlayerTracks))
	}
}

func TestRestoreSessionUnreadablePathMarkedMissing(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	blocker := filepath.Join(dirs.Runtime, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadablePath := filepath.Join(blocker, "track.mp3")

	insertTrack(t, database, "hash-enotdir", unreadablePath, "Artist C", "Song C")
	insertQueueEntry(t, database, "hash-enotdir", 1000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if !restored.QueueEntries[0].Missing {
		t.Fatal("QueueEntry.Missing: want true for unreadable path component")
	}
}

// TestRestoreSessionMixedFiles verifies that only missing files have Missing=true
// when the queue contains both readable and unreadable entries.
func TestRestoreSessionMixedFiles(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	readablePath := filepath.Join(dirs.Runtime, "exists.mp3")
	if err := os.WriteFile(readablePath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dirs.Runtime, "gone.mp3")

	insertTrack(t, database, "hash-r", readablePath, "Readable", "Track")
	insertTrack(t, database, "hash-m", missingPath, "Missing", "Track")
	insertQueueEntry(t, database, "hash-r", 1000)
	insertQueueEntry(t, database, "hash-m", 2000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 2 {
		t.Fatalf("QueueEntries: want 2, got %d", len(restored.QueueEntries))
	}
	if restored.QueueEntries[0].Missing {
		t.Error("QueueEntries[0] (readable): want Missing=false")
	}
	if !restored.QueueEntries[1].Missing {
		t.Error("QueueEntries[1] (missing path): want Missing=true")
	}
}

// TestRestoreSessionQueuePersistence verifies that queue contents survive a
// simulated session restart.
func TestRestoreSessionQueuePersistence(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	pathA := filepath.Join(dirs.Runtime, "track-a.mp3")
	pathB := filepath.Join(dirs.Runtime, "track-b.mp3")
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(p, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	insertTrack(t, database, "hash-aaa", pathA, "Artist A", "Song A")
	insertTrack(t, database, "hash-bbb", pathB, "Artist B", "Song B")

	idA := insertQueueEntry(t, database, "hash-aaa", 1000)
	idB := insertQueueEntry(t, database, "hash-bbb", 2000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 2 {
		t.Fatalf("QueueEntries: want 2, got %d", len(restored.QueueEntries))
	}
	if len(restored.PlayerTracks) != 2 {
		t.Fatalf("PlayerTracks: want 2, got %d", len(restored.PlayerTracks))
	}

	eA := restored.QueueEntries[0]
	if eA.ID != idA {
		t.Errorf("QueueEntries[0].ID: want %d, got %d", idA, eA.ID)
	}
	if eA.Hash != "hash-aaa" {
		t.Errorf("QueueEntries[0].Hash: want %q, got %q", "hash-aaa", eA.Hash)
	}
	if eA.Artist != "Artist A" {
		t.Errorf("QueueEntries[0].Artist: want %q, got %q", "Artist A", eA.Artist)
	}
	if eA.Title != "Song A" {
		t.Errorf("QueueEntries[0].Title: want %q, got %q", "Song A", eA.Title)
	}

	eB := restored.QueueEntries[1]
	if eB.ID != idB {
		t.Errorf("QueueEntries[1].ID: want %d, got %d", idB, eB.ID)
	}
	if eB.Hash != "hash-bbb" {
		t.Errorf("QueueEntries[1].Hash: want %q, got %q", "hash-bbb", eB.Hash)
	}
	if eB.Artist != "Artist B" {
		t.Errorf("QueueEntries[1].Artist: want %q, got %q", "Artist B", eB.Artist)
	}
	if eB.Title != "Song B" {
		t.Errorf("QueueEntries[1].Title: want %q, got %q", "Song B", eB.Title)
	}

	for i, e := range restored.QueueEntries {
		if restored.PlayerTracks[i].Hash != e.Hash {
			t.Errorf("PlayerTracks[%d].Hash: want %q, got %q", i, e.Hash, restored.PlayerTracks[i].Hash)
		}
	}
}

// TestRestoreSessionMissingFileMarkedInDB verifies that MarkQueueEntryMissing
// persists the missing state back to the database.
func TestRestoreSessionMissingFileMarkedInDB(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	missingPath := filepath.Join(dirs.Runtime, "ghost.mp3")
	insertTrack(t, database, "hash-ghost", missingPath, "Ghost", "Track")
	id := insertQueueEntry(t, database, "hash-ghost", 1000)

	if _, err := app.RestoreSession(database, w); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}

	var missing int
	err := database.QueryRow(`SELECT missing FROM queue WHERE id=?`, id).Scan(&missing)
	if err != nil {
		t.Fatalf("query queue.missing: %v", err)
	}
	if missing != 1 {
		t.Errorf("queue.missing: want 1, got %d", missing)
	}
}

// TestRestoreSessionSessionFlags verifies that repeat_mode, shuffle,
// current_hash, and position_ms are restored from the session table.
func TestRestoreSessionSessionFlags(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	audioPath := filepath.Join(dirs.Runtime, "song.mp3")
	if err := os.WriteFile(audioPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-xyz", audioPath, "X", "Y")
	insertQueueEntry(t, database, "hash-xyz", 1000)
	setSessionRow(t, database, "hash-xyz", 30500, false, "all")

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if restored.RepeatMode != tui.RepeatAll {
		t.Errorf("RepeatMode: want RepeatAll, got %v", restored.RepeatMode)
	}
	if restored.Shuffle {
		t.Error("Shuffle: want false")
	}
	if restored.CurrentHash != "hash-xyz" {
		t.Errorf("CurrentHash: want %q, got %q", "hash-xyz", restored.CurrentHash)
	}
	if restored.PositionMs != 30500 {
		t.Errorf("PositionMs: want 30500, got %d", restored.PositionMs)
	}
}

// TestRestoreSessionRepeatOne verifies that repeat_mode="one" is correctly parsed.
func TestRestoreSessionRepeatOne(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	setSessionRow(t, database, "", 0, false, "one")

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if restored.RepeatMode != tui.RepeatOne {
		t.Errorf("RepeatMode: want RepeatOne, got %v", restored.RepeatMode)
	}
}
