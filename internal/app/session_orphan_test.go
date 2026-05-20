package app_test

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

// TestApplyRestoredSession_OrphanCurrentHashIgnored verifies that when the
// restored session references a current track hash that does not appear in
// the restored player queue, the TUI is NOT marked as having a current track.
// Previously the hash was stored on the model without setting an index,
// causing status-bar / now-playing desync.
func TestApplyRestoredSession_OrphanCurrentHashIgnored(t *testing.T) {
	installLogger(t)

	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}

	// Queue contains a single track ("aaa"), but the persisted current_hash
	// points to a different ("ghost") track that is not in the queue.
	qe := []tui.QueueEntry{
		{ID: 1, Hash: "aaa", Path: "/music/a.mp3", Artist: "A", Title: "Song A"},
	}
	pt := []player.QueueTrack{
		{Hash: "aaa", Path: "/music/a.mp3"},
	}
	restored := &app.RestoredSession{
		QueueEntries: qe,
		PlayerTracks: pt,
		CurrentHash:  "ghost-hash-not-in-queue",
	}

	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, nil, nil, nil, 50)

	if mgr.indexSet {
		t.Error("SetCurrentIndex should NOT be called when current_hash is orphaned")
	}
	if got := model.PlaybackCurrentIndex(); got >= 0 {
		t.Errorf("model PlaybackCurrentIndex = %d, want < 0 (no current track)", got)
	}
	if got := model.StatusBarArtist(); got != "" {
		t.Errorf("status bar artist = %q, want empty (no current track)", got)
	}
	if got := model.StatusBarTitle(); got != "" {
		t.Errorf("status bar title = %q, want empty (no current track)", got)
	}
}

// TestApplyRestoredSession_CurrentHashMatchedStillWorks verifies the happy
// path was not broken by the orphan-hash fix: when the hash matches a queue
// entry, the index, status bar, and player are all updated.
func TestApplyRestoredSession_CurrentHashMatchedStillWorks(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}

	qe := []tui.QueueEntry{
		{ID: 1, Hash: "aaa", Path: "/music/a.mp3", Artist: "A", Title: "Song A"},
		{ID: 2, Hash: "bbb", Path: "/music/b.mp3", Artist: "B", Title: "Song B", SourcePlaylist: "Jazz"},
	}
	pt := []player.QueueTrack{
		{Hash: "aaa", Path: "/music/a.mp3"},
		{Hash: "bbb", Path: "/music/b.mp3"},
	}
	restored := &app.RestoredSession{
		QueueEntries: qe,
		PlayerTracks: pt,
		CurrentHash:  "bbb",
	}

	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, nil, nil, nil, 50)

	if !mgr.indexSet || mgr.currentIndex != 1 {
		t.Errorf("currentIndex = %d (set=%v), want 1 (set=true)", mgr.currentIndex, mgr.indexSet)
	}
	if got := model.PlaybackCurrentIndex(); got != 1 {
		t.Errorf("model PlaybackCurrentIndex = %d, want 1", got)
	}
	if got := model.StatusBarTitle(); got != "Song B" {
		t.Errorf("status bar title = %q, want %q", got, "Song B")
	}
}
