package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestFocusedTrackMeta_Found verifies that the found flag is true when a track
// with empty artist/album metadata is focused.
func TestFocusedTrackMeta_Found(t *testing.T) {
	tracks := []library.Track{{Artist: "", Album: "", Title: "No Meta", Hash: "h1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Error("expected found=true for track with empty artist/album")
	}
	if artist != "" {
		t.Errorf("artist: want empty, got %q", artist)
	}
	if album != "" {
		t.Errorf("album: want empty, got %q", album)
	}
}
