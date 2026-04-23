package tui

import (
	"context"
	"testing"
)

// TestResetPlaybackStateClearsAllFields verifies that resetPlaybackState zeros
// out all playback-related fields including stopAfterCurrent, peak counters,
// durationCorrected, queueExhausted, and missingTrackCount.
func TestResetPlaybackStateClearsAllFields(t *testing.T) {
	p := &stubStopAfterPlayer{}
	p.stopAfter = true

	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.playback.peakElapsedSecs = 100
	m.playback.peakTotalSecs = 200
	m.playback.durationCorrected = true
	m.playback.stopAfterCurrent = true
	m.playback.queueExhausted = true
	m.playback.currentHash = "somehash"
	m.playback.currentIndex = 3
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.statusBar.playlistSource = "Source"
	m.missingTrackCount = 5

	m = m.resetPlaybackState()

	if m.playback.state != PlaybackIdle {
		t.Errorf("state = %v, want PlaybackIdle", m.playback.state)
	}
	if m.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", m.playback.elapsedSecs)
	}
	if m.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", m.playback.totalSecs)
	}
	if m.playback.peakElapsedSecs != 0 {
		t.Errorf("peakElapsedSecs = %v, want 0", m.playback.peakElapsedSecs)
	}
	if m.playback.peakTotalSecs != 0 {
		t.Errorf("peakTotalSecs = %v, want 0", m.playback.peakTotalSecs)
	}
	if m.playback.durationCorrected {
		t.Error("durationCorrected should be false")
	}
	if m.playback.stopAfterCurrent {
		t.Error("stopAfterCurrent should be false")
	}
	if m.playback.queueExhausted {
		t.Error("queueExhausted should be false")
	}
	if m.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", m.playback.currentHash)
	}
	if m.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1", m.playback.currentIndex)
	}
	if m.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty", m.statusBar.artist)
	}
	if m.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty", m.statusBar.title)
	}
	if m.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty", m.statusBar.playlistSource)
	}
	if m.missingTrackCount != 0 {
		t.Errorf("missingTrackCount = %d, want 0", m.missingTrackCount)
	}
	if p.stopAfter {
		t.Error("player stopAfter should be cleared via SetStopAfterCurrent(false)")
	}
}

// TestResetPlaybackStateStopsPlayer verifies that resetPlaybackState calls
// Stop() on the player when it implements the commandPlayer interface.
func TestResetPlaybackStateStopsPlayer(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying

	m = m.resetPlaybackState()

	if p.stopCalls != 1 {
		t.Errorf("expected 1 Stop() call, got %d", p.stopCalls)
	}
}

// TestResetPlaybackStateNoPlayerPanic verifies that resetPlaybackState does not
// panic when the player is nil or does not implement commandPlayer.
func TestResetPlaybackStateNoPlayerPanic(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackPlaying
	m.playback.stopAfterCurrent = true

	// Should not panic.
	m = m.resetPlaybackState()

	if m.playback.stopAfterCurrent {
		t.Error("stopAfterCurrent should be false even without a player")
	}
}

// TestResetQueueStateClearsSaveAsPlaylist verifies that resetQueueState clears
// the savingAsPlaylist and savingFromSelection flags so that a stale inline
// save input does not persist after the queue is emptied.
func TestResetQueueStateClearsSaveAsPlaylist(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 1, Path: "/a.flac", Display: "A"}}
	m.queue.savingAsPlaylist = true
	m.queue.savingFromSelection = true

	m = m.resetQueueState()

	if m.queue.savingAsPlaylist {
		t.Error("savingAsPlaylist should be false after resetQueueState")
	}
	if m.queue.savingFromSelection {
		t.Error("savingFromSelection should be false after resetQueueState")
	}
	if len(m.queue.entries) != 0 {
		t.Errorf("entries should be nil after resetQueueState, got %d", len(m.queue.entries))
	}
}

// TestSwitchViewCancelsSaveAsPlaylist verifies that switching away from the
// queue view cancels the active save-as-playlist input so the stale inline
// input does not persist when the user returns to the queue view.
func TestSwitchViewCancelsSaveAsPlaylist(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{{ID: 1, Path: "/a.flac", Display: "A"}}
	m.queue.savingAsPlaylist = true
	m.queue.savingFromSelection = true

	m = m.switchToView(viewLibrary)

	if m.queue.savingAsPlaylist {
		t.Error("savingAsPlaylist should be false after switchToView")
	}
	if m.queue.savingFromSelection {
		t.Error("savingFromSelection should be false after switchToView")
	}
	if m.activeView != viewLibrary {
		t.Errorf("activeView = %v, want viewLibrary", m.activeView)
	}
}

// TestSwitchViewCancelsPlaylistInlineInputs verifies that switching away from
// the playlists view cancels active inline create and rename inputs.
func TestSwitchViewCancelsPlaylistInlineInputs(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.creating = true
	m.playlists.renaming = true

	m = m.switchToView(viewLibrary)

	if m.playlists.creating {
		t.Error("playlists.creating should be false after switchToView")
	}
	if m.playlists.renaming {
		t.Error("playlists.renaming should be false after switchToView")
	}
}

// TestSwitchViewCancelsTagInlineInputs verifies that switching away from the
// tags view cancels active inline create and rename inputs.
func TestSwitchViewCancelsTagInlineInputs(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.creating = true
	m.tagsView.renaming = true

	m = m.switchToView(viewLibrary)

	if m.tagsView.creating {
		t.Error("tagsView.creating should be false after switchToView")
	}
	if m.tagsView.renaming {
		t.Error("tagsView.renaming should be false after switchToView")
	}
}

// TestClearQueueClearsStopAfterCurrent verifies the end-to-end flow: enabling
// stop-after-current, clearing the queue, and then checking the flag is reset.
// This is the specific user-facing bug that was fixed.
func TestClearQueueClearsStopAfterCurrent(t *testing.T) {
	p := &stubStopAfterPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.stopAfterCurrent = true
	p.stopAfter = true
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Display: "A"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.playback.stopAfterCurrent {
		t.Error("stopAfterCurrent should be cleared after ClearQueueMsg")
	}
	if p.stopAfter {
		t.Error("player stopAfter should be cleared after ClearQueueMsg")
	}
}
