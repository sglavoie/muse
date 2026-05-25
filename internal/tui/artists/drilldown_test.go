package artists

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestOpenAlbumsTransitionsToAlbumLevel(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: []string{"Alice", "Bob", "Charlie"},
	}

	v2 := v.OpenAlbums([]string{"Alpha", "Beta"})
	if v2.Level != AlbumLevel {
		t.Errorf("OpenAlbums: expected Level=AlbumLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenAlbums: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.ArtistCursor != 2 {
		t.Errorf("OpenAlbums: expected ArtistCursor=2 (saved), got %d", v2.ArtistCursor)
	}
	if len(v2.Albums) != 2 {
		t.Errorf("OpenAlbums: expected 2 albums, got %d", len(v2.Albums))
	}
}

func TestOpenTracksTransitionsToTrackLevel(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Cursor: 1,
		Albums: []string{"Alpha", "Beta"},
	}

	v2 := v.OpenTracks([]library.Track{{Title: "Song A"}, {Title: "Song B"}})
	if v2.Level != TrackLevel {
		t.Errorf("OpenTracks: expected Level=TrackLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenTracks: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.AlbumCursor != 1 {
		t.Errorf("OpenTracks: expected AlbumCursor=1 (saved), got %d", v2.AlbumCursor)
	}
	if len(v2.Tracks) != 2 {
		t.Errorf("OpenTracks: expected 2 tracks, got %d", len(v2.Tracks))
	}
}

func TestBackFromAlbumLevelRestoresArtistCursor(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       3,
		ScrollOffset: 2,
		Artists:      make([]string, 10),
	}

	v = v.OpenAlbums([]string{"Alpha", "Beta"})
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected album cursor=0, got %d", v.Cursor)
	}

	v.Cursor = 1
	v.ScrollOffset = 1
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("Back: expected Level=ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back: expected Cursor=3 (restored), got %d", v.Cursor)
	}
	if v.ScrollOffset != 2 {
		t.Errorf("Back: expected ScrollOffset=2 (restored), got %d", v.ScrollOffset)
	}
	if v.Albums != nil {
		t.Errorf("Back: expected Albums=nil, got %v", v.Albums)
	}
}

func TestBackFromTrackLevelRestoresAlbumCursor(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       2,
		ScrollOffset: 1,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}

	v = v.OpenTracks([]library.Track{{Title: "Song A"}, {Title: "Song B"}})
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected track cursor=0, got %d", v.Cursor)
	}

	v.Cursor = 1
	v.ScrollOffset = 0
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Errorf("Back: expected Level=AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("Back: expected Cursor=2 (restored), got %d", v.Cursor)
	}
	if v.ScrollOffset != 1 {
		t.Errorf("Back: expected ScrollOffset=1 (restored), got %d", v.ScrollOffset)
	}
	if v.Tracks != nil {
		t.Errorf("Back: expected Tracks=nil, got %v", v.Tracks)
	}
}

func TestBackAtArtistLevelIsNoOp(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  1,
		Artists: []string{"Alice", "Bob"},
	}

	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("Back at ArtistLevel: expected Level=ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back at ArtistLevel: expected Cursor=1 (unchanged), got %d", v.Cursor)
	}
}

func TestBackClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Artists:      []string{"a", "b", "c", "d", "e", "f"},
	}

	v = v.OpenAlbums([]string{"X", "Y"})
	v.Artists = []string{"a", "b"}
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: expected ScrollOffset <= Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestBackClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       3,
		ScrollOffset: 1,
		Albums:       []string{"X", "Y", "Z", "W"},
	}

	v = v.OpenTracks([]library.Track{{Title: "t"}})
	v.Albums = nil
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("Back with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}

func TestBackKeepsRestoredCursorVisibleAfterResize(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       8,
		ScrollOffset: 0,
		Height:       20,
		Artists:      []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
	}

	v = v.OpenAlbums([]string{"X"})
	v.Height = 3
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 8 {
		t.Fatalf("expected restored cursor=8, got %d", v.Cursor)
	}
	if v.ScrollOffset != 7 {
		t.Errorf("expected scroll offset=7 to keep cursor visible, got %d", v.ScrollOffset)
	}
}

func TestBackDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       1,
		ScrollOffset: 0,
		Height:       10,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}

	v = v.OpenTracks([]library.Track{{Title: "Song"}})
	v.Height = 1
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestFullDrillDownCycle(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: []string{"Alice", "Bob", "Charlie"},
		Width:   80,
	}

	v = v.OpenAlbums([]string{"Alpha", "Beta"})
	if v.Level != AlbumLevel || v.Cursor != 0 {
		t.Fatalf("after OpenAlbums: Level=%v Cursor=%d", v.Level, v.Cursor)
	}

	v.Cursor = 1
	v = v.OpenTracks([]library.Track{{Title: "Song A"}, {Title: "Song B"}})
	if v.Level != TrackLevel || v.Cursor != 0 {
		t.Fatalf("after OpenTracks: Level=%v Cursor=%d", v.Level, v.Cursor)
	}

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Errorf("first Back: expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("first Back: expected album Cursor=1, got %d", v.Cursor)
	}

	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("second Back: expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("second Back: expected artist Cursor=2, got %d", v.Cursor)
	}
}
