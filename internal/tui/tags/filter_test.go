package tags

import "testing"

func TestEffectiveTags_ExcludesZeroCount(t *testing.T) {
	tags := tagRows("has-tracks", "empty", "also-has-tracks")
	v := View{
		Tags: tags,
		TagCounts: map[int64]int{
			tags[0].ID: 5,
			tags[1].ID: 0,
			tags[2].ID: 2,
		},
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
	tags := tagRows("workout", "work", "chill")
	v := View{
		Tags: tags,
		TagCounts: map[int64]int{
			tags[0].ID: 3,
			tags[1].ID: 0,
			tags[2].ID: 1,
		},
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
	tags := tagRows("a", "b")
	v := View{Tags: tags}

	got := v.EffectiveTags()
	if len(got) != 2 {
		t.Fatalf("got %d tags, want 2 (nil counts should show all)", len(got))
	}
}
