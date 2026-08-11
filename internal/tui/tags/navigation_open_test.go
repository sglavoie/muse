package tags

import (
	"testing"
)

// TestOpenTracksTranslatesFilteredCursorToUnfilteredIndex verifies that
// OpenTracks saves the unfiltered index of the focused tag so Back() (which
// clears the filter) restores the cursor to the same tag rather than to the
// position the filtered index happened to occupy.
func TestOpenTracksTranslatesFilteredCursorToUnfilteredIndex(t *testing.T) {
	tags := tagRows("workout", "work", "chill")
	v := View{
		Level:      TagLevel,
		Tags:       tags,
		TagCounts:  tagCountsFor(tags, 1),
		FilterText: "ch",
		Width:      80,
		Height:     20,
	}

	// EffectiveTags under filter "ch" = ["chill"].
	// Focusing index 0 corresponds to "chill" (unfiltered index 2).
	v.Cursor = 0

	v = v.OpenTracks(tags[2], trackRows("x"))
	if v.TagCursor != 2 {
		t.Errorf("expected saved TagCursor=2 (unfiltered index of chill), got %d", v.TagCursor)
	}

	v = v.Back()
	if v.Level != TagLevel {
		t.Fatalf("expected TagLevel after Back, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("expected restored cursor=2 (chill in unfiltered list), got %d", v.Cursor)
	}
	items := v.EffectiveTags()
	if items[v.Cursor].Name != "chill" {
		t.Errorf("expected cursor on chill, got %q", items[v.Cursor].Name)
	}
}

// TestOpenTracksTranslatesFilteredCursorSkippingZeroCountTags guards a
// regression where unfilteredTagCursorPos translated to the raw v.Tags index,
// which left the post-Back cursor landing on the wrong tag whenever zero-count
// tags appeared before the focused tag in v.Tags (Back clears the filter and
// EffectiveTags excludes zero-count tags, so the translation target must be
// the zero-count-filtered slice).
func TestOpenTracksTranslatesFilteredCursorSkippingZeroCountTags(t *testing.T) {
	tags := tagRows("alpha", "beta", "chill", "delta")
	v := View{
		Level: TagLevel,
		Tags:  tags,
		TagCounts: map[int64]int{
			tags[0].ID: 0,
			tags[1].ID: 0,
			tags[2].ID: 1,
			tags[3].ID: 1,
		},
		FilterText: "ch",
		Width:      80,
		Height:     20,
	}

	// EffectiveTags under filter "ch" (with zero counts excluded) = ["chill"].
	v.Cursor = 0

	v = v.OpenTracks(tags[2], trackRows("x"))
	v = v.Back()
	if v.Level != TagLevel {
		t.Fatalf("expected TagLevel after Back, got %v", v.Level)
	}
	items := v.EffectiveTags()
	if v.Cursor < 0 || v.Cursor >= len(items) {
		t.Fatalf("cursor %d out of range [0,%d)", v.Cursor, len(items))
	}
	if items[v.Cursor].Name != "chill" {
		t.Errorf("expected restored cursor on %q, got %q", "chill", items[v.Cursor].Name)
	}
}

func TestOpenTracksClearsSelectedHashes(t *testing.T) {
	tag := tagRows("tag")[0]
	v := View{
		Level:          TagLevel,
		SelectedHashes: map[string]struct{}{"stale": {}},
	}

	v = v.OpenTracks(tag, trackRows("next"))

	if v.SelectedHashes != nil {
		t.Fatalf("OpenTracks should clear stale selected hashes, got %v", v.SelectedHashes)
	}
}
