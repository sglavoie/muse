package albums

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestOpenTracksAndBack(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"Alpha", "Beta"},
		Cursor: 1,
		Width:  80,
	}

	v = v.OpenTracks([]library.Track{{Title: "T1"}, {Title: "T2"}})
	if v.Level != TrackLevel {
		t.Fatalf("expected TrackLevel, got %d", v.Level)
	}
	if v.Cursor != 0 {
		t.Errorf("expected cursor 0, got %d", v.Cursor)
	}
	if v.AlbumCursor != 1 {
		t.Errorf("expected saved AlbumCursor=1, got %d", v.AlbumCursor)
	}

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %d", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("expected restored cursor=1, got %d", v.Cursor)
	}
}

func TestBackAtAlbumLevelIsNoop(t *testing.T) {
	v := View{Level: AlbumLevel, Cursor: 2}
	v = v.Back()
	if v.Level != AlbumLevel || v.Cursor != 2 {
		t.Errorf("Back at AlbumLevel should be no-op, got level=%d cursor=%d", v.Level, v.Cursor)
	}
}

func TestBackClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       4,
		ScrollOffset: 2,
		Albums:       []string{"a", "b", "c", "d", "e"},
		Height:       20,
	}

	v = v.OpenTracks([]library.Track{{Title: "x"}})
	v.Albums = []string{"a", "b"}
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: ScrollOffset(%d) must be <= Cursor(%d)", v.ScrollOffset, v.Cursor)
	}
}

func TestBackKeepsCursorVisibleAfterResize(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       8,
		ScrollOffset: 0,
		Albums:       []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
		Height:       5,
		Width:        80,
	}

	v = v.OpenTracks([]library.Track{{Title: "x"}})
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 8 {
		t.Fatalf("expected cursor=8, got %d", v.Cursor)
	}
	maxRows := v.visibleRows()
	if v.Cursor < v.ScrollOffset || v.Cursor >= v.ScrollOffset+maxRows {
		t.Errorf("cursor=%d not visible in window [%d, %d)", v.Cursor, v.ScrollOffset, v.ScrollOffset+maxRows)
	}
}

func TestOpenTracksTranslatesFilteredCursorToUnfilteredIndex(t *testing.T) {
	v := View{
		Level:      AlbumLevel,
		Albums:     []string{"Alpha", "Beta", "Gamma", "Delta"},
		FilterText: "ta",
		Width:      80,
		Height:     20,
	}

	// EffectiveAlbums under filter "ta" = ["Beta", "Delta"].
	// Focusing index 1 corresponds to "Delta" (unfiltered index 3).
	v.Cursor = 1

	v = v.OpenTracks([]library.Track{{Title: "x"}})
	if v.AlbumCursor != 3 {
		t.Errorf("expected saved AlbumCursor=3 (unfiltered index of Delta), got %d", v.AlbumCursor)
	}

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel after Back, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("expected restored cursor=3 (Delta in unfiltered list), got %d", v.Cursor)
	}
	items := v.EffectiveAlbums()
	if items[v.Cursor] != "Delta" {
		t.Errorf("expected cursor on Delta, got %q", items[v.Cursor])
	}
}

func TestBackClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       3,
		ScrollOffset: 1,
		Albums:       []string{"a", "b", "c", "d"},
		Height:       20,
	}

	v = v.OpenTracks([]library.Track{{Title: "x"}})
	v.Albums = nil
	v = v.Back()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("Back with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}
