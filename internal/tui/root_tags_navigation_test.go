package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func sampleTagTracks() []db.PlaylistTrackRow {
	return []db.PlaylistTrackRow{
		{ID: 1, Hash: "th1", Title: "Tag Track 1", Artist: "A", Album: "X", DurationMs: 180000},
		{ID: 2, Hash: "th2", Title: "Tag Track 2", Artist: "B", Album: "Y", DurationMs: 200000},
		{ID: 3, Hash: "th3", Title: "Tag Track 3", Artist: "C", Album: "Z", DurationMs: 220000},
	}
}

func newTagsTrackLevelModel() Model {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = sampleTagTracks()
	m.tagsView.SelectedTag = db.TagRow{ID: 1, Name: "TestTag"}
	m.tagsView.Height = 40
	return m
}

func TestBulkSelectionToggleTagsTrackLevel(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.Cursor = 0

	m, _, _ = m.handleNavigationAction(keys.ActionBulkSelectionToggle)

	if len(m.tagsView.SelectedHashes) != 1 {
		t.Fatalf("after first x: expected 1 selected hash, got %d", len(m.tagsView.SelectedHashes))
	}
	if _, ok := m.tagsView.SelectedHashes["th1"]; !ok {
		t.Errorf("expected hash th1 selected, got: %v", m.tagsView.SelectedHashes)
	}

	m, _, _ = m.handleNavigationAction(keys.ActionBulkSelectionToggle)
	if len(m.tagsView.SelectedHashes) != 0 {
		t.Errorf("after second x: expected 0 selected hashes, got %d", len(m.tagsView.SelectedHashes))
	}
}

func TestBulkSelectionToggleTagsNoOpAtTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{{ID: 1, Name: "T1"}}
	m.tagsView.TagCounts = map[int64]int{1: 3}

	m, _, _ = m.handleNavigationAction(keys.ActionBulkSelectionToggle)
	if len(m.tagsView.SelectedHashes) != 0 {
		t.Errorf("expected no selection at TagLevel, got %d", len(m.tagsView.SelectedHashes))
	}
}

func TestClearSelectionTags(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.SelectedHashes = map[string]struct{}{"th1": {}, "th2": {}}

	m, _, _ = m.handleNavigationAction(keys.ActionClearSelection)
	if len(m.tagsView.SelectedHashes) != 0 {
		t.Errorf("after X: expected 0 selected hashes, got %d", len(m.tagsView.SelectedHashes))
	}
}

func TestVisualModeTagsTrackLevel(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.Cursor = 0

	m = doVisual(m)

	if !m.visual.active {
		t.Fatal("expected visual mode to be active")
	}
	if m.visual.anchor != 0 {
		t.Errorf("anchor: expected 0, got %d", m.visual.anchor)
	}
	if _, ok := m.tagsView.SelectedHashes["th1"]; !ok {
		t.Error("expected th1 to be selected at anchor")
	}

	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)

	if len(m.tagsView.SelectedHashes) != 3 {
		t.Errorf("expected 3 selected hashes after extending, got %d", len(m.tagsView.SelectedHashes))
	}
	for _, h := range []string{"th1", "th2", "th3"} {
		if _, ok := m.tagsView.SelectedHashes[h]; !ok {
			t.Errorf("expected %s to be selected", h)
		}
	}
}

func TestVisualModeTagsBlockedAtTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.Level = tagtui.TagLevel

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at TagLevel")
	}
}

func TestVisualModeTagsRangeShrinks(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown) // cursor 1
	m = doNav(m, keys.ActionCursorDown) // cursor 2
	m = doNav(m, keys.ActionCursorUp)   // cursor 1

	if len(m.tagsView.SelectedHashes) != 2 {
		t.Errorf("expected 2 selected after shrink, got %d", len(m.tagsView.SelectedHashes))
	}
	if _, ok := m.tagsView.SelectedHashes["th3"]; ok {
		t.Error("th3 should not be selected after shrinking range")
	}
}

func TestVisualModeTagsExitPreservesSelections(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown) // range: th1..th2
	m = doVisual(m)                     // exit

	if m.visual.active {
		t.Fatal("expected visual mode to be inactive after second v")
	}
	if len(m.tagsView.SelectedHashes) != 2 {
		t.Errorf("expected 2 selections after exit, got %d", len(m.tagsView.SelectedHashes))
	}
}
