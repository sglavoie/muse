package radio

import (
	"slices"
	"testing"
)

// A tag merge is not a deletion: the references that named the source have to
// follow it. These are the value-level moves the TUI performs on the settings
// and the captured seed of a session that is running when the merge lands.

func TestFilterSet_RemapTag_MovesAndDeduplicates(t *testing.T) {
	set := FilterSet{TagIDs: []int64{5, 2, 9}, Genres: []string{"Rock"}}

	got := set.RemapTag(9, 2)

	if !slices.Equal(got.TagIDs, []int64{2, 5}) {
		t.Errorf("tag ids = %v, want [2 5]: the destination is kept once and the result is normalized", got.TagIDs)
	}
	if !slices.Equal(got.Genres, []string{"Rock"}) {
		t.Errorf("genres = %v, want the merge to leave them alone", got.Genres)
	}
	// The receiver is shared with whatever is planning off the update loop.
	if !slices.Equal(set.TagIDs, []int64{5, 2, 9}) {
		t.Errorf("the remap mutated its receiver: %v", set.TagIDs)
	}
}

func TestFilterSet_RemapTag_IgnoresAnythingItDoesNotReference(t *testing.T) {
	set := FilterSet{TagIDs: []int64{5, 2}}

	for _, tc := range []struct {
		name      string
		src, dest int64
	}{
		{"a tag the set does not hold", 9, 2},
		{"a tag merged into itself", 5, 5},
		{"a non-positive id", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := set.RemapTag(tc.src, tc.dest)
			if !slices.Equal(got.TagIDs, []int64{5, 2}) {
				t.Errorf("tag ids = %v, want the unsorted original left as it was", got.TagIDs)
			}
		})
	}
}

func TestSettings_RemapTag_KeepsEachSidesIntent(t *testing.T) {
	s := DefaultSettings()
	s.Include = FilterSet{TagIDs: []int64{4}}
	s.Exclude = FilterSet{TagIDs: []int64{7, 4}}

	got := s.RemapTag(4, 7)

	if !slices.Equal(got.Include.TagIDs, []int64{7}) {
		t.Errorf("include = %v, want [7]", got.Include.TagIDs)
	}
	if !slices.Equal(got.Exclude.TagIDs, []int64{7}) {
		t.Errorf("exclude = %v, want [7]: an exclusion stays an exclusion", got.Exclude.TagIDs)
	}
	if got.DiscoveryPercent != s.DiscoveryPercent || got.Anchor != s.Anchor {
		t.Errorf("the remap changed something other than the tag references: %+v", got)
	}
}

func TestSeedSnapshot_RemapTag_MovesTheCapturedReference(t *testing.T) {
	seed := SeedSnapshot{TrackHash: "h1", Title: "T", TagIDs: []int64{3, 8}, Genres: []string{"Jazz"}}

	got := seed.RemapTag(8, 3)

	if !slices.Equal(got.TagIDs, []int64{3}) {
		t.Errorf("seed tag ids = %v, want [3]", got.TagIDs)
	}
	if got.TrackHash != seed.TrackHash || got.Title != seed.Title ||
		!slices.Equal(got.Genres, seed.Genres) {
		t.Errorf("the remap rewrote the frozen metadata: %+v", got)
	}
}
