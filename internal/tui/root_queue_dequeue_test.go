package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// TestDequeueTrackMsgCallsPlayerQueueRemoverWithCorrectIndex verifies that
// DequeueTrackMsg calls RemoveFromQueue on the player with the dequeued track's
// index so the player's in-memory queue stays in sync with the TUI queue.
func TestDequeueTrackMsgCallsPlayerQueueRemoverWithCorrectIndex(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("expected 2 queue entries after dequeue, got %d", len(nm.queue.entries))
	}
	if len(p.removeFromQueueCalls) != 1 {
		t.Fatalf("expected 1 RemoveFromQueue call, got %d", len(p.removeFromQueueCalls))
	}
	if p.removeFromQueueCalls[0] != 1 {
		t.Fatalf("expected RemoveFromQueue called with index 1, got %d", p.removeFromQueueCalls[0])
	}
}

// TestDequeueTrackMsgDoesNotCallStop verifies that dequeuing a track never
// calls Stop on the player, regardless of playback state. Playback of the
// currently active track continues uninterrupted; only the queue index list
// is trimmed.
func TestDequeueTrackMsgDoesNotCallStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state PlaybackState
	}{
		{"idle", PlaybackIdle},
		{"playing", PlaybackPlaying},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubPlayerQueueRemover{}
			m := New(context.Background(), WithPlayer(p))
			m.playback.state = tc.state
			m.queue.entries = []QueueEntry{
				{ID: 10, Path: "/music/a.flac", Display: "A"},
				{ID: 20, Path: "/music/b.flac", Display: "B"},
			}

			m.Update(DequeueTrackMsg{Index: 0})

			if p.stopCalls != 0 {
				t.Fatalf("DequeueTrackMsg must not call Stop, got %d calls", p.stopCalls)
			}
		})
	}
}

// TestDequeueTrackMsgClampsScrollOffset verifies that dequeuing a track when
// the queue is scrolled clamps the scrollOffset so it does not point past the
// end of the shortened list.
func TestDequeueTrackMsgClampsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
		{ID: 30, Path: "/c.flac", Display: "C"},
	}
	m.queue.scrollOffset = 2
	m.queue.cursor = 2

	next, _ := m.Update(DequeueTrackMsg{Index: 2})
	nm := next.(Model)

	if nm.queue.scrollOffset > len(nm.queue.entries)-1 {
		t.Fatalf("scrollOffset %d exceeds last entry index %d after dequeue",
			nm.queue.scrollOffset, len(nm.queue.entries)-1)
	}
}

// TestDequeueLastEntryResetsScrollOffset verifies that dequeuing the only
// entry resets scrollOffset to 0.
func TestDequeueLastEntryResetsScrollOffset(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 10, Path: "/a.flac", Display: "A"}}
	m.queue.scrollOffset = 0
	m.queue.cursor = 0

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.queue.scrollOffset != 0 {
		t.Fatalf("scrollOffset must be 0 after dequeuing the last entry, got %d",
			nm.queue.scrollOffset)
	}
}

// TestDequeueTrackMsgDoesNotTouchPlaylistTracks verifies that processing a
// DequeueTrackMsg leaves any loaded playlist tracks completely unchanged.
func TestDequeueTrackMsgDoesNotTouchPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}
	playlistTracks := []db.PlaylistTrackRow{{ID: 1, Position: 1, Title: "Playlist Track", Artist: "Artist", DurationMs: 60000}}
	m = drillIntoPlaylistTrackLevel(m, playlistTracks)

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if len(nm.playlists.tracks) != len(playlistTracks) {
		t.Fatalf("playlist tracks must be unchanged after dequeue: expected %d, got %d",
			len(playlistTracks), len(nm.playlists.tracks))
	}
	if nm.playlists.tracks[0].ID != playlistTracks[0].ID {
		t.Fatalf("playlist track ID must be unchanged: expected %d, got %d",
			playlistTracks[0].ID, nm.playlists.tracks[0].ID)
	}
}

// TestClearQueueClearsStatusBarTrackInfo verifies that ClearQueueMsg resets the
// status bar artist, title, playlistSource, and currentTrackHash to empty strings.
func TestClearQueueClearsStatusBarTrackInfo(t *testing.T) {
	sp := &stubCommandPlayer{}
	m := New(context.Background(), WithPlayer(sp))
	m.statusBar.artist = "Old Artist"
	m.statusBar.title = "Old Title"
	m.statusBar.playlistSource = "Old Playlist"
	m.playback.currentHash = "oldhash"
	m.playback.state = PlaybackPlaying
	m.queue.entries = []QueueEntry{{ID: 10, Path: "/a.flac", Display: "A"}}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty after ClearQueueMsg", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty after ClearQueueMsg", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty after ClearQueueMsg", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty after ClearQueueMsg", nm.playback.currentHash)
	}
}

// TestPlaybackEndedClearsStatusBarTrackInfo verifies that PlaybackEndedMsg
// resets the status bar artist, title, playlistSource, and currentTrackHash.
func TestPlaybackEndedClearsStatusBarTrackInfo(t *testing.T) {
	m := New(context.Background(), WithPlayer(&stubPlayer{}))
	m.statusBar.artist = "Old Artist"
	m.statusBar.title = "Old Title"
	m.statusBar.playlistSource = "Queue"
	m.playback.currentHash = "oldhash"
	m.playback.state = PlaybackPlaying
	m.playback.repeatMode = RepeatOff

	next, _ := m.Update(PlaybackEndedMsg{})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty after PlaybackEndedMsg", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty after PlaybackEndedMsg", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty after PlaybackEndedMsg", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty after PlaybackEndedMsg", nm.playback.currentHash)
	}
}

// TestDequeuePlayingTrackClearsStatusBar verifies that dequeuing the currently
// playing track stops playback and clears the status bar fields.
func TestDequeuePlayingTrackClearsStatusBar(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.statusBar.playlistSource = "Queue"
	m.playback.elapsedSecs = 42.0
	m.playback.totalSecs = 180.0
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("statusBar.artist = %q, want empty", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("statusBar.title = %q, want empty", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("statusBar.playlistSource = %q, want empty", nm.statusBar.playlistSource)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentTrackHash = %q, want empty", nm.playback.currentHash)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0", nm.playback.totalSecs)
	}
	if p.stopCalls != 1 {
		t.Errorf("Stop must be called once, got %d calls", p.stopCalls)
	}
}

// TestTickDoesNotUpdateElapsedWhenIdle verifies that the tick handler does not
// overwrite elapsedSecs/totalSecs when playback is idle.
func TestTickDoesNotUpdateElapsedWhenIdle(t *testing.T) {
	p := &stubElapsedReader{elapsed: 99.0, total: 300.0}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.playback.elapsedSecs != 0 {
		t.Errorf("elapsedSecs = %v, want 0 when idle", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("totalSecs = %v, want 0 when idle", nm.playback.totalSecs)
	}
}

// TestBulkDequeueCreatesUndoSlot verifies that bulk-dequeueing selected entries
// sets up an undoKindBulkDequeue slot with the correct entries, indices, and IDs.
func TestBulkDequeueCreatesUndoSlot(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C"},
		{ID: 40, Path: "/d.flac", Display: "D", Selected: true},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after bulk dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[0].ID != 10 || nm.queue.entries[1].ID != 30 {
		t.Fatalf("remaining entries should be ID=10,30; got %d,%d",
			nm.queue.entries[0].ID, nm.queue.entries[1].ID)
	}
	bulkAction, ok := nm.undo.slot.action.(undoBulkDequeueAction)
	if !ok {
		t.Fatalf("expected undoBulkDequeueAction, got %T", nm.undo.slot.action)
	}
	if len(bulkAction.ids) != 2 || bulkAction.ids[0] != 20 || bulkAction.ids[1] != 40 {
		t.Fatalf("bulkDequeueIDs = %v, want [20 40]", bulkAction.ids)
	}
	if len(bulkAction.indices) != 2 || bulkAction.indices[0] != 1 || bulkAction.indices[1] != 3 {
		t.Fatalf("bulkDequeueIndices = %v, want [1 3]", bulkAction.indices)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "undo") {
		t.Fatalf("notification should contain undo hint, got %q", nm.statusBar.notification.msg)
	}
	if len(d.dequeueTrackSoftCalls) != 2 {
		t.Fatalf("DequeueTrackSoft calls = %d, want 2", len(d.dequeueTrackSoftCalls))
	}
}

// TestFocusNowPlayingMovesCursorToCurrentTrack verifies that pressing "f" in
// the queue view moves the cursor to the entry matching currentTrackHash.
// TestFocusNowPlayingNoOpWhenNothingPlaying lives in root_queue_actions_test.go.

// TestDequeueDuplicateHashKeepsPlayback verifies that removing a queue entry
// whose hash matches the currently-playing track does NOT stop playback when
// another entry with the same hash remains in the queue.
func TestDequeueDuplicateHashKeepsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/a.flac", Display: "A (dup)", Hash: "abc123"},
		{ID: 30, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playbackState = %v, want PlaybackPlaying (another entry with same hash remains)", nm.playback.state)
	}
	if nm.playback.currentHash != "abc123" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "abc123")
	}
	if nm.statusBar.artist != "Artist" {
		t.Errorf("statusBar.artist = %q, want %q", nm.statusBar.artist, "Artist")
	}
	if p.stopCalls != 0 {
		t.Errorf("Stop should not be called when another instance of the hash remains, got %d calls", p.stopCalls)
	}
}

// TestDequeueLastInstanceOfHashStopsPlayback verifies that removing the only
// queue entry with the currently-playing hash stops playback.
func TestDequeueLastInstanceOfHashStopsPlayback(t *testing.T) {
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123"},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", nm.playback.currentHash)
	}
}

// TestBulkDequeueDuplicateHashKeepsPlayback verifies that bulk-removing
// selected entries does not stop playback when another entry with the
// currently-playing hash remains.
func TestBulkDequeueDuplicateHashKeepsPlayback(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123", Selected: true},
		{ID: 20, Path: "/music/a.flac", Display: "A (dup)", Hash: "abc123"},
		{ID: 30, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playbackState = %v, want PlaybackPlaying (another entry with same hash remains)", nm.playback.state)
	}
	if nm.playback.currentHash != "abc123" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "abc123")
	}
}

// TestBulkDequeueLastInstanceOfHashStopsPlayback verifies that bulk-removing
// all entries with the currently-playing hash stops playback.
func TestBulkDequeueLastInstanceOfHashStopsPlayback(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123", Selected: true},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", nm.playback.currentHash)
	}
}

// TestClearQueueForReplaceClearsFilter verifies that clearQueueForReplace
// deactivates the queue filter so the replacement entry is always visible.
func TestClearQueueForReplaceClearsFilter(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Hash: "aaa", Display: "A", Artist: "Art", Title: "A"},
		{ID: 2, Path: "/b.flac", Hash: "bbb", Display: "B", Artist: "Brt", Title: "B"},
	}
	m.queue.filter.Active = true
	m.queue.filter.Text = "art"

	m = m.clearQueueForReplace()

	if m.queue.filter.Active {
		t.Error("filter.Active must be false after clearQueueForReplace")
	}
	if m.queue.filter.Text != "" {
		t.Errorf("filter.Text must be empty after clearQueueForReplace, got %q", m.queue.filter.Text)
	}
	if len(m.queue.entries) != 0 {
		t.Errorf("queue entries must be empty after clearQueueForReplace, got %d", len(m.queue.entries))
	}
}

// TestPlayQueueIndexMsgClearsStateOnFailure verifies that when PlayFromIndex
// fails, handlePlayQueueIndexMsg clears stale status bar and playback fields.
func TestPlayQueueIndexMsgClearsStateOnFailure(t *testing.T) {
	p := &stubPlayer{playFromIndexErr: errStubPlay}
	m := New(context.Background(), WithPlayer(p))
	m.queue.entries = []QueueEntry{{Path: "/music/a.flac", Hash: "aaa", Display: "A", Artist: "Art", Title: "Ttl"}}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "old-hash"
	m.playback.elapsedSecs = 42
	m.playback.totalSecs = 180
	m.statusBar.artist = "Old Artist"
	m.statusBar.title = "Old Title"
	m.statusBar.playlistSource = "Old Playlist"

	next, _ := m.Update(PlayQueueIndexMsg{Index: 0})
	nm := next.(Model)

	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("expected empty currentHash, got %q", nm.playback.currentHash)
	}
	if nm.playback.elapsedSecs != 0 {
		t.Errorf("expected elapsedSecs=0, got %f", nm.playback.elapsedSecs)
	}
	if nm.playback.totalSecs != 0 {
		t.Errorf("expected totalSecs=0, got %f", nm.playback.totalSecs)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("expected empty artist, got %q", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("expected empty title, got %q", nm.statusBar.title)
	}
	if nm.statusBar.playlistSource != "" {
		t.Errorf("expected empty playlistSource, got %q", nm.statusBar.playlistSource)
	}
	if nm.statusBar.notification.msg == "" {
		t.Error("expected error notification")
	}
}

// TestBulkDequeueAdjustsCurrentIndex verifies that bulk-dequeueing selected
// entries adjusts playback.currentIndex so the now-playing highlight stays on
// the correct entry.
func TestBulkDequeueAdjustsCurrentIndex(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.currentIndex = 3
	m.playback.currentHash = "ddd"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A", Hash: "aaa", Selected: true},
		{ID: 20, Path: "/b.flac", Display: "B", Hash: "bbb"},
		{ID: 30, Path: "/c.flac", Display: "C", Hash: "ccc", Selected: true},
		{ID: 40, Path: "/d.flac", Display: "D", Hash: "ddd"},
		{ID: 50, Path: "/e.flac", Display: "E", Hash: "eee"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after bulk dequeue: expected 3 entries, got %d", len(nm.queue.entries))
	}
	if nm.playback.currentIndex != 1 {
		t.Errorf("currentIndex = %d, want 1 (adjusted after removing 2 entries before it)", nm.playback.currentIndex)
	}
}

// TestBulkDequeueCurrentTrackSelectedResetsIndex verifies that bulk-dequeueing
// the currently-playing entry sets currentIndex to -1.
func TestBulkDequeueCurrentTrackSelectedResetsIndex(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.currentIndex = 1
	m.playback.currentHash = "bbb"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A", Hash: "aaa"},
		{ID: 20, Path: "/b.flac", Display: "B", Hash: "bbb", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C", Hash: "ccc"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after bulk dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if nm.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1 (currently-playing track was removed)", nm.playback.currentIndex)
	}
}
