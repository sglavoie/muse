package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestHandleTagViewTracksLoaded_DropsStaleResultWhenViewSwitched verifies the
// async ReadTracksByTag command does not flip the tags view to TrackLevel
// when the user has navigated away from viewTags before the message arrived.
// Before the fix, the late message would silently transition the level so a
// later return to viewTags would render tracks instead of the tag list.
func TestHandleTagViewTracksLoaded_DropsStaleResultWhenViewSwitched(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{{ID: 1, Name: "Alpha"}}

	// User switches away from the tags view while the load is in flight.
	m.ActiveView = viewLibrary

	msg := tagViewTracksLoadedMsg{
		tag:    db.TagRow{ID: 1, Name: "Alpha"},
		tracks: []db.PlaylistTrackRow{{ID: 10, Hash: "h1"}},
		counts: map[int64]int{1: 1},
	}
	m = m.handleTagViewTracksLoaded(msg)

	if m.tagsView.Level != tagtui.TagLevel {
		t.Errorf("level should remain TagLevel when view was switched; got %v", m.tagsView.Level)
	}
	if m.tagsView.Tracks != nil {
		t.Errorf("tracks should not be populated for stale msg; got %d", len(m.tagsView.Tracks))
	}
	// Counts (a less invasive update) should still apply.
	if got := m.tagsView.TagCounts[1]; got != 1 {
		t.Errorf("counts should still apply from stale msg; got %d", got)
	}
}

// TestHandleTagViewTracksLoaded_DropsStaleResultWhenLevelChanged verifies that
// the message is dropped when the user has already drilled into TrackLevel via
// a different load (or pressed back/forward in between), preventing the late
// msg from overwriting the current tracks with stale ones for a different tag.
func TestHandleTagViewTracksLoaded_DropsStaleResultWhenLevelChanged(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	existing := []db.PlaylistTrackRow{{ID: 99, Hash: "current"}}
	m.tagsView.Tracks = existing
	m.tagsView.SelectedTag = db.TagRow{ID: 2, Name: "Beta"}

	msg := tagViewTracksLoadedMsg{
		tag:    db.TagRow{ID: 1, Name: "Alpha"},
		tracks: []db.PlaylistTrackRow{{ID: 10, Hash: "stale"}},
	}
	m = m.handleTagViewTracksLoaded(msg)

	if len(m.tagsView.Tracks) != 1 || m.tagsView.Tracks[0].Hash != "current" {
		t.Errorf("tracks should remain unchanged; got %+v", m.tagsView.Tracks)
	}
	if m.tagsView.SelectedTag.ID != 2 {
		t.Errorf("selectedTag should remain Beta(2); got %d", m.tagsView.SelectedTag.ID)
	}
}
