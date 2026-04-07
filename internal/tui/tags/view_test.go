package tags

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestEffectiveTags_ExcludesZeroCount(t *testing.T) {
	v := View{
		Tags: []db.TagRow{
			{ID: 1, Name: "has-tracks"},
			{ID: 2, Name: "empty"},
			{ID: 3, Name: "also-has-tracks"},
		},
		TagCounts: map[int64]int{1: 5, 2: 0, 3: 2},
	}

	got := v.EffectiveTags()
	if len(got) != 2 {
		t.Fatalf("got %d tags, want 2", len(got))
	}
	if got[0].Name != "has-tracks" || got[1].Name != "also-has-tracks" {
		t.Errorf("unexpected tags: %+v", got)
	}
}

func TestEffectiveTags_ExcludesZeroCountWithFilter(t *testing.T) {
	v := View{
		Tags: []db.TagRow{
			{ID: 1, Name: "workout"},
			{ID: 2, Name: "work"},
			{ID: 3, Name: "chill"},
		},
		TagCounts:  map[int64]int{1: 3, 2: 0, 3: 1},
		FilterText: "work",
	}

	got := v.EffectiveTags()
	if len(got) != 1 {
		t.Fatalf("got %d tags, want 1", len(got))
	}
	if got[0].Name != "workout" {
		t.Errorf("got %q, want %q", got[0].Name, "workout")
	}
}

func TestEffectiveTags_NilCountsShowsAll(t *testing.T) {
	v := View{
		Tags: []db.TagRow{
			{ID: 1, Name: "a"},
			{ID: 2, Name: "b"},
		},
	}

	got := v.EffectiveTags()
	if len(got) != 2 {
		t.Fatalf("got %d tags, want 2 (nil counts should show all)", len(got))
	}
}
