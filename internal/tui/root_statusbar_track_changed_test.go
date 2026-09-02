package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestTrackChangedMsgUpdatesStatusBar(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	nm, _ := updateModel(m, playback.TrackChangedMsg{Artist: "New Artist", Title: "New Title", PlaylistSource: "My Playlist"})

	assertViewContains(t, nm, "New Artist")
	assertViewContains(t, nm, "New Title")
	assertViewContains(t, nm, "My Playlist")
	if nm.playback.state != playback.StatePlaying {
		t.Fatalf("TrackChangedMsg must set PlaybackPlaying, got %v", nm.playback.state)
	}
}

func TestTrackChangedMsgDefaultsPlaylistSourceToQueue(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}}

	nm, _ := updateModel(m, playback.TrackChangedMsg{Artist: "A", Title: "T"})
	assertViewContains(t, nm, "Queue")
}

func TestTrackChangedClearsPreviousAlbumWhenHashNotFound(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.flac"}, {ID: 2, Hash: "unknown", Path: "/b.flac"}}
	m.statusBar.Album = "Previous Album"

	nm, _ := updateModel(m, playback.TrackChangedMsg{Hash: "unknown", Artist: "New Artist", Title: "New Title", Index: 1})

	if nm.statusBar.Album != "" {
		t.Errorf("statusBar.album must be reset when new hash is not in library index; got %q", nm.statusBar.Album)
	}
}
