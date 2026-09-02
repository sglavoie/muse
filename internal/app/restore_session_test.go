package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestRestoreSessionEmpty verifies that RestoreSession on an empty database
// returns a non-nil RestoredSession with no entries and default flags.
func TestRestoreSessionEmpty(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	restored := harness.restore()
	if restored == nil {
		t.Fatal("RestoreSession returned nil")
	}
	if len(restored.QueueEntries) != 0 {
		t.Errorf("QueueEntries: want 0, got %d", len(restored.QueueEntries))
	}
	if len(restored.PlayerTracks) != 0 {
		t.Errorf("PlayerTracks: want 0, got %d", len(restored.PlayerTracks))
	}
	if restored.RepeatMode != playback.RepeatOff {
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
	harness := newRestoreSessionHarness(t)
	audioPath := harness.writeAudioFile("track.mp3")
	harness.insertTrackAndQueue("hash-aaa", audioPath, "Artist A", "Song A", 1000)

	restored := harness.restore()
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
// file paths have Missing=true in the returned queue.Entry slice.
func TestRestoreSessionMissingFiles(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	missingPath := harness.path("nonexistent.mp3")
	harness.insertTrackAndQueue("hash-bbb", missingPath, "Artist B", "Song B", 1000)

	restored := harness.restore()
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
	harness := newRestoreSessionHarness(t)
	blocker := harness.path("not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadablePath := filepath.Join(blocker, "track.mp3")

	harness.insertTrackAndQueue("hash-enotdir", unreadablePath, "Artist C", "Song C", 1000)

	restored := harness.restore()
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if !restored.QueueEntries[0].Missing {
		t.Fatal("QueueEntry.Missing: want true for unreadable path component")
	}
}

func TestRestoreSessionDirectoryPathMarkedMissing(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	dirPath := harness.path("track.mp3")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	id := harness.insertTrackAndQueue("hash-dir", dirPath, "Artist D", "Song D", 1000)

	restored := harness.restore()
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if !restored.QueueEntries[0].Missing {
		t.Fatal("QueueEntry.Missing: want true when saved track path is a directory")
	}

	queueMissing := harness.queueMissing(id)
	if queueMissing != 1 {
		t.Errorf("queue.missing: want 1 for directory path, got %d", queueMissing)
	}
}

// TestRestoreSessionMixedFiles verifies that only missing files have Missing=true
// when the queue contains both readable and unreadable entries.
func TestRestoreSessionMixedFiles(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	readablePath := harness.writeAudioFile("exists.mp3")
	missingPath := harness.path("gone.mp3")
	harness.insertTrackAndQueue("hash-r", readablePath, "Readable", "Track", 1000)
	harness.insertTrackAndQueue("hash-m", missingPath, "Missing", "Track", 2000)

	restored := harness.restore()
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
	harness := newRestoreSessionHarness(t)
	pathA := harness.writeAudioFile("track-a.mp3")
	pathB := harness.writeAudioFile("track-b.mp3")
	idA := harness.insertTrackAndQueue("hash-aaa", pathA, "Artist A", "Song A", 1000)
	idB := harness.insertTrackAndQueue("hash-bbb", pathB, "Artist B", "Song B", 2000)

	restored := harness.restore()
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
	harness := newRestoreSessionHarness(t)
	missingPath := harness.path("ghost.mp3")
	id := harness.insertTrackAndQueue("hash-ghost", missingPath, "Ghost", "Track", 1000)

	harness.restore()

	missing := harness.queueMissing(id)
	if missing != 1 {
		t.Errorf("queue.missing: want 1, got %d", missing)
	}
}

func TestRestoreSessionRepairsTrackMissingForStaleQueueMissing(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	missingPath := harness.path("still-gone.mp3")
	id := harness.insertTrackAndQueue("hash-still-gone", missingPath, "Gone", "Track", 1000)

	harness.markQueueMissing(id, 1)
	harness.markTrackMissing("hash-still-gone", 0)

	restored := harness.restore()
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if !restored.QueueEntries[0].Missing {
		t.Fatal("QueueEntry.Missing: want true while file is still absent")
	}

	if trackMissing := harness.trackMissing("hash-still-gone"); trackMissing != 1 {
		t.Errorf("tracks.missing: want 1 after restore repairs stale queue-only missing state, got %d", trackMissing)
	}
}

// TestRestoreSessionSessionFlags verifies that repeat_mode, shuffle,
// current_hash, and position_ms are restored from the session table.
func TestRestoreSessionSessionFlags(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	audioPath := harness.writeAudioFile("song.mp3")
	harness.insertTrackAndQueue("hash-xyz", audioPath, "X", "Y", 1000)
	harness.setSession("hash-xyz", 30500, false, "all")

	restored := harness.restore()
	if restored.RepeatMode != playback.RepeatAll {
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
	harness := newRestoreSessionHarness(t)
	harness.setSession("", 0, false, "one")

	restored := harness.restore()
	if restored.RepeatMode != playback.RepeatOne {
		t.Errorf("RepeatMode: want RepeatOne, got %v", restored.RepeatMode)
	}
}

// TestRestoreSessionClearsStaleMissing verifies that a queue entry previously
// marked missing=1 in the DB has its missing flag cleared when the file is
// readable at restore time.
func TestRestoreSessionClearsStaleMissing(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	// Create the file after marking the entry missing so we simulate a file
	// that was gone in a previous session and has since reappeared.
	audioPath := harness.path("returned.mp3")
	id := harness.insertTrackAndQueue("hash-returned", audioPath, "Artist R", "Song R", 1000)

	// Manually mark missing=1 in both tables to simulate a prior-session scan.
	harness.markQueueMissing(id, 1)
	harness.markTrackMissing("hash-returned", 1)

	// Write the file so it is now readable.
	if err := os.WriteFile(audioPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	restored := harness.restore()
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if restored.QueueEntries[0].Missing {
		t.Error("QueueEntry.Missing: want false after stale missing cleared")
	}

	queueMissing := harness.queueMissing(id)
	if queueMissing != 0 {
		t.Errorf("queue.missing: want 0 after clear, got %d", queueMissing)
	}

	trackMissing := harness.trackMissing("hash-returned")
	if trackMissing != 0 {
		t.Errorf("tracks.missing: want 0 after clear, got %d", trackMissing)
	}
}

// TestRestoreSessionStaleMissingCurrentTrackCanRestore verifies that when the
// current track was previously missing but is now readable, RestoreSession
// sets Missing=false so the caller can proceed with RestorePlayer.
func TestRestoreSessionStaleMissingCurrentTrackCanRestore(t *testing.T) {
	harness := newRestoreSessionHarness(t)
	audioPath := harness.path("current.mp3")
	id := harness.insertTrackAndQueue("hash-current", audioPath, "C", "Current", 1000)
	harness.setSession("hash-current", 5000, false, "none")

	// Mark both tables missing.
	harness.markQueueMissing(id, 1)
	harness.markTrackMissing("hash-current", 1)

	// File is now back on disk.
	if err := os.WriteFile(audioPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	restored := harness.restore()
	if len(restored.QueueEntries) == 0 {
		t.Fatal("QueueEntries: want at least 1")
	}
	if restored.QueueEntries[0].Missing {
		t.Error("current track Missing: want false (file is back)")
	}
	if restored.CurrentHash != "hash-current" {
		t.Errorf("CurrentHash: want %q, got %q", "hash-current", restored.CurrentHash)
	}
}
