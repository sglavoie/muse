package tui

import (
	"context"
	"testing"

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
