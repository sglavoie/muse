package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestAdjustScroll(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		scrollOffset int
		maxRows      int
		want         int
	}{
		{"cursor above viewport", 2, 5, 10, 2},
		{"cursor below viewport", 15, 0, 10, 6},
		{"cursor within viewport", 5, 0, 10, 0},
		{"cursor at viewport bottom edge", 9, 0, 10, 0},
		{"zero maxRows", 5, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustScroll(tt.cursor, tt.scrollOffset, tt.maxRows)
			if got != tt.want {
				t.Errorf("adjustScroll(%d, %d, %d) = %d, want %d",
					tt.cursor, tt.scrollOffset, tt.maxRows, got, tt.want)
			}
		})
	}
}

func TestFindStringPosition(t *testing.T) {
	items := []string{"Alpha", "Bravo", "Charlie"}
	if findStringPosition(items, "Bravo") != 1 {
		t.Error("expected 1 for Bravo")
	}
	if findStringPosition(items, "Missing") != 0 {
		t.Error("expected 0 for missing item")
	}
	if findStringPosition(nil, "A") != 0 {
		t.Error("expected 0 for nil items")
	}
}

func TestFindTrackHashPosition(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a"},
		{Hash: "b"},
		{Hash: "c"},
	}
	if findTrackHashPosition(tracks, "b") != 1 {
		t.Error("expected 1 for hash b")
	}
	if findTrackHashPosition(tracks, "missing") != 0 {
		t.Error("expected 0 for missing hash")
	}
	if findTrackHashPosition(tracks, "") != 0 {
		t.Error("expected 0 for empty hash")
	}
}

func TestFindPlaylistPosition(t *testing.T) {
	pls := []db.PlaylistRow{
		{ID: 10, Name: "A"},
		{ID: 20, Name: "B"},
		{ID: 30, Name: "C"},
	}
	if findPlaylistPosition(pls, 20) != 1 {
		t.Error("expected 1 for ID 20")
	}
	if findPlaylistPosition(pls, 99) != 0 {
		t.Error("expected 0 for missing ID")
	}
}

func TestSortQueueIndicesZeroDurationSortsLast(t *testing.T) {
	entries := []QueueEntry{
		{Title: "A", DurationMs: 0},
		{Title: "B", DurationMs: 5000},
		{Title: "C", DurationMs: 2000},
	}
	t.Run("asc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortDuration, SortAsc)
		if entries[order[0]].Title != "C" {
			t.Errorf("first should be C (2000ms), got %s", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "B" {
			t.Errorf("second should be B (5000ms), got %s", entries[order[1]].Title)
		}
		if entries[order[2]].DurationMs != 0 {
			t.Error("zero-duration entry should be last")
		}
	})
	t.Run("desc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortDuration, SortDesc)
		if entries[order[0]].Title != "B" {
			t.Errorf("first should be B (5000ms), got %s", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "C" {
			t.Errorf("second should be C (2000ms), got %s", entries[order[1]].Title)
		}
		if entries[order[2]].DurationMs != 0 {
			t.Error("zero-duration entry should be last")
		}
	})
}

func TestSortPlaylistTrackIndicesZeroDurationSortsLast(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "A", DurationMs: 0},
		{Hash: "h2", Title: "B", DurationMs: 3000},
		{Hash: "h3", Title: "C", DurationMs: 1000},
	}
	order := SortPlaylistTrackIndices(tracks, SortDuration, SortAsc)
	if tracks[order[0]].Hash != "h3" {
		t.Errorf("first should be h3 (1000ms), got %s", tracks[order[0]].Hash)
	}
	if tracks[order[1]].Hash != "h2" {
		t.Errorf("second should be h2 (3000ms), got %s", tracks[order[1]].Hash)
	}
	if tracks[order[2]].DurationMs != 0 {
		t.Error("zero-duration track should be last")
	}
}
