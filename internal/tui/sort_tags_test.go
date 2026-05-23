package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestTagsSortFieldsForLevel(t *testing.T) {
	t.Run("TagLevel returns nil", func(t *testing.T) {
		got := tagsSortFieldsForLevel(tagtui.TagLevel)
		if got != nil {
			t.Errorf("expected nil for TagLevel, got %v", got)
		}
	})
	t.Run("TrackLevel returns tagsTrackSortFields", func(t *testing.T) {
		got := tagsSortFieldsForLevel(tagtui.TrackLevel)
		if len(got) != len(tagsTrackSortFields) {
			t.Fatalf("len=%d, want %d", len(got), len(tagsTrackSortFields))
		}
		for i, f := range got {
			if f != tagsTrackSortFields[i] {
				t.Errorf("idx %d: got %d, want %d", i, f, tagsTrackSortFields[i])
			}
		}
	})
}

func TestTagsRecomputeSort(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "Charlie", Album: "Red", DurationMs: 300000},
		{Hash: "h2", Title: "Alpha", Artist: "Alice", Album: "Blue", DurationMs: 100000},
		{Hash: "h3", Title: "Bravo", Artist: "Bob", Album: "Green", DurationMs: 200000},
	}

	t.Run("sort by title asc", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortTitle, Dir: SortAsc},
		}
		v = v.recomputeTagsSort()
		if v.sortedTrackOrder == nil {
			t.Fatal("expected non-nil sortedTrackOrder")
		}
		display := v.displayTracks()
		if display[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", display[0].Title)
		}
		if display[2].Title != "Zulu" {
			t.Errorf("last should be Zulu, got %q", display[2].Title)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortArtist, Dir: SortDesc},
		}
		v = v.recomputeTagsSort()
		display := v.displayTracks()
		if display[0].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", display[0].Artist)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortDuration, Dir: SortAsc},
		}
		v = v.recomputeTagsSort()
		display := v.displayTracks()
		if display[0].DurationMs != 100000 {
			t.Errorf("first should have 100000ms, got %d", display[0].DurationMs)
		}
	})

	t.Run("default clears sortedTrackOrder", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TrackLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortDefault},
		}
		v = v.recomputeTagsSort()
		if v.sortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil for SortDefault")
		}
	})

	t.Run("TagLevel clears sortedTrackOrder", func(t *testing.T) {
		v := TagsView{
			level:     tagtui.TagLevel,
			tracks:    tracks,
			sortState: SortState{Field: SortTitle, Dir: SortAsc},
		}
		v = v.recomputeTagsSort()
		if v.sortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil at TagLevel")
		}
	})
}

func TestTagsSortResetOnBack(t *testing.T) {
	v := TagsView{
		level:     tagtui.TrackLevel,
		tracks:    []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}},
		sortState: SortState{Field: SortTitle, Dir: SortDesc},
	}
	v = v.recomputeTagsSort()
	if v.sortedTrackOrder == nil {
		t.Fatal("precondition: sortedTrackOrder should be non-nil")
	}

	v2 := v.back()
	if v2.sortState.Field != SortDefault {
		t.Error("sort should be reset on back")
	}
	if v2.sortedTrackOrder != nil {
		t.Error("sortedTrackOrder should be nil after back")
	}
}

func TestTagsBackClampsRestoredCursorAgainstCurrentTags(t *testing.T) {
	v := TagsView{
		level:           tagtui.TrackLevel,
		tags:            []db.TagRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		tagCounts:       map[int64]int{1: 1, 2: 1},
		tracks:          []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}},
		cursor:          0,
		scrollOffset:    0,
		tagCursor:       4,
		tagScrollOffset: 3,
		height:          10,
	}

	v = v.back()
	if v.level != tagtui.TagLevel {
		t.Fatalf("expected TagLevel after back, got %v", v.level)
	}
	if v.cursor != 1 {
		t.Fatalf("expected cursor clamped to last tag, got %d", v.cursor)
	}
	if v.scrollOffset > v.cursor {
		t.Fatalf("expected scrollOffset <= cursor, got scroll=%d cursor=%d", v.scrollOffset, v.cursor)
	}
}

func TestTagsSortResetOnOpenTracks(t *testing.T) {
	v := TagsView{
		level:     tagtui.TagLevel,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	tag := db.TagRow{ID: 1, Name: "MyTag"}
	tracks := []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}}
	v2 := v.openTracks(tag, tracks)
	if v2.sortState.Field != SortDefault {
		t.Error("sort should be reset on openTracks")
	}
}

func TestTagsFocusedTrackWithSort(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "C"},
		{Hash: "h2", Title: "Alpha", Artist: "A"},
	}
	v := TagsView{
		level:     tagtui.TrackLevel,
		tracks:    tracks,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
		cursor:    0,
	}
	v = v.recomputeTagsSort()

	track, ok := v.focusedTrack()
	if !ok {
		t.Fatal("expected focused track")
	}
	if track.Title != "Alpha" {
		t.Errorf("cursor 0 with title-asc sort should be Alpha, got %q", track.Title)
	}
}

func TestTagsVisibleReserve(t *testing.T) {
	tests := []struct {
		name string
		v    TagsView
		want int
	}{
		{"tag level no filter no sort", TagsView{level: tagtui.TagLevel}, 0},
		{"tag level with filter", TagsView{level: tagtui.TagLevel, filter: filter.State{Active: true}}, 1},
		{"tag level with sort", TagsView{level: tagtui.TagLevel, sortState: SortState{Field: SortName}}, 1},
		{"track level no sort", TagsView{level: tagtui.TrackLevel}, 3},
		{"track level with sort", TagsView{level: tagtui.TrackLevel, sortState: SortState{Field: SortTitle}}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsVisibleReserve(tt.v)
			if got != tt.want {
				t.Errorf("tagsVisibleReserve = %d, want %d", got, tt.want)
			}
		})
	}
}
