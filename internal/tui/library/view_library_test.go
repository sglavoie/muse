package library

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestLibraryViewToggleBulkSelectionToggles verifies that calling
// ToggleBulkSelection twice on the same hash leaves the set empty.
func TestLibraryViewToggleBulkSelectionToggles(t *testing.T) {
	v := View{}

	v = v.ToggleBulkSelection("abc")
	if _, ok := v.SelectedHashes["abc"]; !ok {
		t.Fatal("expected hash to be selected after first toggle")
	}

	v = v.ToggleBulkSelection("abc")
	if _, ok := v.SelectedHashes["abc"]; ok {
		t.Fatal("expected hash to be deselected after second toggle")
	}
	if len(v.SelectedHashes) != 0 {
		t.Fatalf("expected empty selection set, got %v", v.SelectedHashes)
	}
}

// TestLibraryViewClearSelectionEmptiesSet verifies that ClearSelection removes
// all bulk-selected hashes.
func TestLibraryViewClearSelectionEmptiesSet(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection("a")
	v = v.ToggleBulkSelection("b")
	v = v.ClearSelection()
	if len(v.SelectedHashes) != 0 {
		t.Fatalf("expected empty selection after ClearSelection, got %v", v.SelectedHashes)
	}
}

func TestLibraryViewHandleActionNavigation(t *testing.T) {
	var idx musiclib.LibraryIndex
	for i := range 10 {
		idx.Insert(musiclib.Track{
			Artist: "A",
			Title:  fmt.Sprintf("T%d", i),
			Hash:   fmt.Sprintf("h%d", i),
		})
	}
	v := View{Index: idx, Width: 80, Height: 24}

	v = v.HandleAction(keys.ActionCursorDown)
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.Cursor)
	}
	v = v.HandleAction(keys.ActionCursorUp)
	if v.Cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.Cursor)
	}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected 9, got %d", v.Cursor)
	}
	v = v.HandleAction(keys.ActionGoToTop)
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.Cursor)
	}
}

func TestLibraryViewHandleActionBulkToggle(t *testing.T) {
	var idx musiclib.LibraryIndex
	idx.Insert(musiclib.Track{Artist: "A", Title: "T1", Hash: "h1"})
	idx.Insert(musiclib.Track{Artist: "B", Title: "T2", Hash: "h2"})

	v := View{Index: idx, Cursor: 0}
	v = v.HandleAction(keys.ActionBulkSelectionToggle)
	if len(v.SelectedHashes) != 1 {
		t.Errorf("BulkToggle: expected 1 selected, got %d", len(v.SelectedHashes))
	}
}

func TestLibraryViewEnqueueFocused(t *testing.T) {
	var idx musiclib.LibraryIndex
	idx.Insert(musiclib.Track{Artist: "A", Title: "T1", Hash: "h1"})

	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}
	cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused: expected non-nil cmd")
	}
	msg := cmd()
	enqueued, ok := msg.(TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", msg)
	}
	if enqueued.Track.Artist != "A" {
		t.Errorf("enqueued track artist: expected A, got %q", enqueued.Track.Artist)
	}
}

func TestLibraryViewEnqueueFocusedEmpty(t *testing.T) {
	v := View{}
	qa := &fakeQueueAppender{}
	cmd := v.EnqueueFocused(qa, 0)
	if cmd != nil {
		t.Error("EnqueueFocused empty: expected nil cmd")
	}
}

func TestLibraryViewBackspaceFilter(t *testing.T) {
	var idx musiclib.LibraryIndex
	idx.Insert(musiclib.Track{Artist: "A", Title: "T1", Hash: "h1"})

	f := filter.State{Active: true, Text: "AB"}
	f, _ = f.Activate()
	v := View{Index: idx, Fuzzy: musiclib.FakeFuzzySearcher{}, Filter: f}
	prevText := v.Filter.Text
	v.Filter, _ = v.Filter.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if v.Filter.Text != prevText {
		v.Cursor = 0
		v.ScrollOffset = 0
		v = v.RebuildFilterCache()
	}
	if v.Filter.Text != "A" {
		t.Errorf("BackspaceFilter: expected 'A', got %q", v.Filter.Text)
	}
	if v.Cursor != 0 {
		t.Errorf("BackspaceFilter: expected cursor=0, got %d", v.Cursor)
	}
}

func TestLibraryViewBackspaceFilterInactiveNoOp(t *testing.T) {
	v := View{Filter: filter.State{Text: "AB"}}
	v.Filter, _ = v.Filter.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if v.Filter.Text != "AB" {
		t.Errorf("BackspaceFilter inactive: expected 'AB', got %q", v.Filter.Text)
	}
}

func TestLibraryViewDeactivateFilter(t *testing.T) {
	var idx musiclib.LibraryIndex
	idx.Insert(musiclib.Track{Artist: "A", Title: "T1", Hash: "h1"})

	v := View{
		Index:  idx,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
		Filter: filter.State{Active: true, Text: "AB"},
		Cursor: 5,
	}
	v.Filter = v.Filter.Deactivate()
	v.Cursor = 0
	v.ScrollOffset = 0
	v = v.RebuildFilterCache()
	if v.Filter.Active {
		t.Error("DeactivateFilter: expected filter.Active=false")
	}
	if v.Filter.Text != "" {
		t.Errorf("DeactivateFilter: expected filter.Text='', got %q", v.Filter.Text)
	}
	if v.Cursor != 0 {
		t.Errorf("DeactivateFilter: expected cursor=0, got %d", v.Cursor)
	}
}

func TestLibraryViewCommitFilter(t *testing.T) {
	v := View{Filter: filter.State{Active: true, Text: "test"}}
	v.Filter = v.Filter.Commit()
	if v.Filter.Active {
		t.Error("CommitFilter: expected filter.Active=false")
	}
	if v.Filter.Text != "test" {
		t.Errorf("CommitFilter: expected filter.Text='test', got %q", v.Filter.Text)
	}
}
