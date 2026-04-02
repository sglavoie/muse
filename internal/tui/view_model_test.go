package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestModelSetRepeatMode(t *testing.T) {
	m := New(context.Background(), WithRepeatMode(RepeatAll))
	if m.playback.repeatMode != RepeatAll {
		t.Errorf("SetRepeatMode: expected RepeatAll, got %v", m.playback.repeatMode)
	}
}

func TestModelSetShuffle(t *testing.T) {
	m := New(context.Background(), WithShuffle(true))
	if !m.playback.shuffle {
		t.Error("SetShuffle(true): expected shuffle=true")
	}
}

func TestModelSetPlaybackState(t *testing.T) {
	m := New(context.Background(), WithPlaybackState(PlaybackPaused))
	if m.playback.state != PlaybackPaused {
		t.Errorf("SetPlaybackState: expected PlaybackPaused, got %v", m.playback.state)
	}
}

func TestModelSetCurrentTrackHash(t *testing.T) {
	m := New(context.Background(), WithCurrentTrackHash("abc123"))
	if m.playback.currentHash != "abc123" {
		t.Errorf("SetCurrentTrackHash: expected 'abc123', got %q", m.playback.currentHash)
	}
}

func TestModelSetLibraryTracks(t *testing.T) {
	tracks := []library.Track{{Artist: "A", Title: "T1", Hash: "h1"}, {Artist: "B", Title: "T2", Hash: "h2"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	if m.library.index.Len() != 2 {
		t.Errorf("SetLibraryTracks: expected 2 tracks, got %d", m.library.index.Len())
	}
}

func TestModelRemoveLibraryTrackByPath(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "A", Title: "T1", Hash: "h1", Path: "/a.mp3"},
		{Artist: "B", Title: "T2", Hash: "h2", Path: "/b.mp3"},
	}))
	m.RemoveLibraryTrackByPath("/a.mp3")
	if m.library.index.Len() != 1 {
		t.Errorf("RemoveLibraryTrackByPath: expected 1 track, got %d", m.library.index.Len())
	}
}
