package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
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
		if len(got) != len(sorting.TagsTrackSortFields) {
			t.Fatalf("len=%d, want %d", len(got), len(sorting.TagsTrackSortFields))
		}
		for i, f := range got {
			if f != sorting.TagsTrackSortFields[i] {
				t.Errorf("idx %d: got %d, want %d", i, f, sorting.TagsTrackSortFields[i])
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
		v := tagtui.TagsView{
			Level:     tagtui.TrackLevel,
			Tracks:    tracks,
			SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
		}
		v = v.RecomputeTagsSort()
		if v.SortedTrackOrder == nil {
			t.Fatal("expected non-nil sortedTrackOrder")
		}
		display := v.DisplayTracks()
		if display[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", display[0].Title)
		}
		if display[2].Title != "Zulu" {
			t.Errorf("last should be Zulu, got %q", display[2].Title)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		v := tagtui.TagsView{
			Level:     tagtui.TrackLevel,
			Tracks:    tracks,
			SortState: sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortDesc},
		}
		v = v.RecomputeTagsSort()
		display := v.DisplayTracks()
		if display[0].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", display[0].Artist)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		v := tagtui.TagsView{
			Level:     tagtui.TrackLevel,
			Tracks:    tracks,
			SortState: sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc},
		}
		v = v.RecomputeTagsSort()
		display := v.DisplayTracks()
		if display[0].DurationMs != 100000 {
			t.Errorf("first should have 100000ms, got %d", display[0].DurationMs)
		}
	})

	t.Run("default clears sortedTrackOrder", func(t *testing.T) {
		v := tagtui.TagsView{
			Level:     tagtui.TrackLevel,
			Tracks:    tracks,
			SortState: sorting.SortState{Field: sorting.SortDefault},
		}
		v = v.RecomputeTagsSort()
		if v.SortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil for SortDefault")
		}
	})

	t.Run("TagLevel clears sortedTrackOrder", func(t *testing.T) {
		v := tagtui.TagsView{
			Level:     tagtui.TagLevel,
			Tracks:    tracks,
			SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
		}
		v = v.RecomputeTagsSort()
		if v.SortedTrackOrder != nil {
			t.Error("sortedTrackOrder should be nil at TagLevel")
		}
	})
}

func TestTagsSortResetOnBack(t *testing.T) {
	v := tagtui.TagsView{
		Level:     tagtui.TrackLevel,
		Tracks:    []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}},
		SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc},
	}
	v = v.RecomputeTagsSort()
	if v.SortedTrackOrder == nil {
		t.Fatal("precondition: sortedTrackOrder should be non-nil")
	}

	v2 := v.Back()
	if v2.SortState.Field != sorting.SortDefault {
		t.Error("sort should be reset on back")
	}
	if v2.SortedTrackOrder != nil {
		t.Error("sortedTrackOrder should be nil after back")
	}
}

func TestTagsBackClampsRestoredCursorAgainstCurrentTags(t *testing.T) {
	v := tagtui.TagsView{
		Level:           tagtui.TrackLevel,
		Tags:            []db.TagRow{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		TagCounts:       map[int64]int{1: 1, 2: 1},
		Tracks:          []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}},
		Cursor:          0,
		ScrollOffset:    0,
		TagCursor:       4,
		TagScrollOffset: 3,
		Height:          10,
	}

	v = v.Back()
	if v.Level != tagtui.TagLevel {
		t.Fatalf("expected TagLevel after back, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Fatalf("expected cursor clamped to last tag, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("expected scrollOffset <= cursor, got scroll=%d cursor=%d", v.ScrollOffset, v.Cursor)
	}
}

func TestTagsSortResetOnOpenTracks(t *testing.T) {
	v := tagtui.TagsView{
		Level:     tagtui.TagLevel,
		SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	tag := db.TagRow{ID: 1, Name: "MyTag"}
	tracks := []db.PlaylistTrackRow{{Hash: "h1", Title: "T1"}}
	v2 := v.OpenTracks(tag, tracks)
	if v2.SortState.Field != sorting.SortDefault {
		t.Error("sort should be reset on openTracks")
	}
}

func TestTagsFocusedTrackWithSort(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "C"},
		{Hash: "h2", Title: "Alpha", Artist: "A"},
	}
	v := tagtui.TagsView{
		Level:     tagtui.TrackLevel,
		Tracks:    tracks,
		SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
		Cursor:    0,
	}
	v = v.RecomputeTagsSort()

	track, ok := v.FocusedTrack()
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
		v    tagtui.TagsView
		want int
	}{
		{"tag level no filter no sort", tagtui.TagsView{Level: tagtui.TagLevel}, 0},
		{"tag level with filter", tagtui.TagsView{Level: tagtui.TagLevel, Filter: filter.State{Active: true}}, 1},
		{"tag level with sort", tagtui.TagsView{Level: tagtui.TagLevel, SortState: sorting.SortState{Field: sorting.SortName}}, 1},
		{"track level no sort", tagtui.TagsView{Level: tagtui.TrackLevel}, 3},
		{"track level with sort", tagtui.TagsView{Level: tagtui.TrackLevel, SortState: sorting.SortState{Field: sorting.SortTitle}}, 4},
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
