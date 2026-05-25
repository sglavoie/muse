package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- 'd' soft-delete ---

func TestSoftDeletePlaylistAtListLevelRemovesEntry(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 1,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
			{ID: 3, Name: "Gamma"},
		},
	}

	idx := v.Cursor
	v.Playlists = append(v.Playlists[:idx], v.Playlists[idx+1:]...)
	if v.Cursor >= len(v.Playlists) && v.Cursor > 0 {
		v.Cursor = len(v.Playlists) - 1
	}

	if len(v.Playlists) != 2 {
		t.Errorf("expected 2 playlists after delete, got %d", len(v.Playlists))
	}
	for _, p := range v.Playlists {
		if p.Name == "Beta" {
			t.Error("'Beta' must be removed from the playlist list")
		}
	}
}

func TestSoftDeletePlaylistClampsLastEntry(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 2,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "A"},
			{ID: 2, Name: "B"},
			{ID: 3, Name: "C"},
		},
	}

	idx := v.Cursor
	v.Playlists = append(v.Playlists[:idx], v.Playlists[idx+1:]...)
	if v.Cursor >= len(v.Playlists) && v.Cursor > 0 {
		v.Cursor = len(v.Playlists) - 1
	}

	if v.Cursor != 1 {
		t.Errorf("expected cursor clamped to 1 after removing last entry, got %d", v.Cursor)
	}
}

func TestSoftDeleteTrackAtTrackLevelRemovesEntry(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 1,
		Tracks: []db.PlaylistTrackRow{
			{ID: 10, Position: 1, Title: "First"},
			{ID: 20, Position: 2, Title: "Second"},
			{ID: 30, Position: 3, Title: "Third"},
		},
	}

	idx := v.Cursor
	v.Tracks = append(v.Tracks[:idx], v.Tracks[idx+1:]...)
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}

	if len(v.Tracks) != 2 {
		t.Errorf("expected 2 tracks after remove, got %d", len(v.Tracks))
	}
	for _, tr := range v.Tracks {
		if tr.Title == "Second" {
			t.Error("'Second' must be removed from the track list")
		}
	}
}

func TestSoftDeleteTrackClampsLastEntry(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 1,
		Tracks: []db.PlaylistTrackRow{
			{ID: 10, Position: 1, Title: "Only"},
			{ID: 20, Position: 2, Title: "Gone"},
		},
	}

	idx := v.Cursor
	v.Tracks = append(v.Tracks[:idx], v.Tracks[idx+1:]...)
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}

	if v.Cursor != 0 {
		t.Errorf("expected cursor clamped to 0 after removing last track, got %d", v.Cursor)
	}
}
