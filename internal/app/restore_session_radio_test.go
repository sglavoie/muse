package app_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui"
)

// TestRestoreSessionSeedsRecentHashes verifies that RestoreSession carries the
// most recently played tracks into RecentHashes, oldest-first, so radio's
// in-memory avoidance ring is no longer empty after a restart.
func TestRestoreSessionSeedsRecentHashes(t *testing.T) {
	h := newRestoreSessionHarness(t)
	path := h.writeAudioFile("a.flac")
	h.insertTrackAndQueue("h1", path, "Alice", "One", 0)
	h.insertTrackAndQueue("h2", path, "Alice", "Two", 1)

	now := int64(1_700_000_000)
	if _, err := h.database.Exec(
		`INSERT INTO play_events (track_hash, played_at, listened_secs, completion_rate, counted)
		 VALUES ('h1', ?, 60, 0.9, 1), ('h2', ?, 60, 0.9, 1)`, now, now+10); err != nil {
		t.Fatalf("insert play events: %v", err)
	}

	restored := h.restore()

	want := []string{"h1", "h2"}
	if len(restored.RecentHashes) != len(want) {
		t.Fatalf("RecentHashes = %v, want %v", restored.RecentHashes, want)
	}
	for i := range want {
		if restored.RecentHashes[i] != want[i] {
			t.Fatalf("RecentHashes = %v, want %v", restored.RecentHashes, want)
		}
	}
	if len(restored.RecentHashes) > tui.RadioRecentCap {
		t.Errorf("RecentHashes longer than RadioRecentCap (%d)", tui.RadioRecentCap)
	}
}

// TestRestoreSessionRecentHashesEmptyHistory verifies that an empty play
// history leaves RecentHashes empty instead of failing the restore.
func TestRestoreSessionRecentHashesEmptyHistory(t *testing.T) {
	h := newRestoreSessionHarness(t)

	restored := h.restore()

	if len(restored.RecentHashes) != 0 {
		t.Errorf("RecentHashes = %v, want empty", restored.RecentHashes)
	}
}
