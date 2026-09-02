package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestLibraryViewToggleBulkSelectionToggles verifies that calling
// ToggleBulkSelection twice on the same hash leaves the set empty.
func TestLibraryViewToggleBulkSelectionToggles(t *testing.T) {
	v := LibraryView{}

	v = v.ToggleBulkSelection("abc")
	if _, ok := v.selectedHashes["abc"]; !ok {
		t.Fatal("expected hash to be selected after first toggle")
	}

	v = v.ToggleBulkSelection("abc")
	if _, ok := v.selectedHashes["abc"]; ok {
		t.Fatal("expected hash to be deselected after second toggle")
	}
	if len(v.selectedHashes) != 0 {
		t.Fatalf("expected empty selection set, got %v", v.selectedHashes)
	}
}

// TestLibraryViewClearSelectionEmptiesSet verifies that ClearSelection removes
// all bulk-selected hashes.
func TestLibraryViewClearSelectionEmptiesSet(t *testing.T) {
	v := LibraryView{}
	v = v.ToggleBulkSelection("a")
	v = v.ToggleBulkSelection("b")
	v = v.ClearSelection()
	if len(v.selectedHashes) != 0 {
		t.Fatalf("expected empty selection after ClearSelection, got %v", v.selectedHashes)
	}
}

func TestLibraryViewHandleActionNavigation(t *testing.T) {
	var idx library.LibraryIndex
	for i := range 10 {
		idx.Insert(library.Track{
			Artist: "A",
			Title:  fmt.Sprintf("T%d", i),
			Hash:   fmt.Sprintf("h%d", i),
		})
	}
	v := LibraryView{index: idx, width: 80, height: 24}

	v = v.HandleAction(keys.ActionCursorDown)
	if v.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.cursor)
	}
	v = v.HandleAction(keys.ActionCursorUp)
	if v.cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.cursor)
	}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.cursor != 9 {
		t.Errorf("GoToBottom: expected 9, got %d", v.cursor)
	}
	v = v.HandleAction(keys.ActionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

func TestLibraryViewHandleActionBulkToggle(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})
	idx.Insert(library.Track{Artist: "B", Title: "T2", Hash: "h2"})

	v := LibraryView{index: idx, cursor: 0}
	v = v.HandleAction(keys.ActionBulkSelectionToggle)
	if len(v.selectedHashes) != 1 {
		t.Errorf("BulkToggle: expected 1 selected, got %d", len(v.selectedHashes))
	}
}

func TestLibraryViewEnqueueFocused(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})

	v := LibraryView{index: idx, cursor: 0}
	qa := &fakeQueueAppender{}
	cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused: expected non-nil cmd")
	}
	msg := cmd()
	enqueued, ok := msg.(libtui.TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", msg)
	}
	if enqueued.Track.Artist != "A" {
		t.Errorf("enqueued track artist: expected A, got %q", enqueued.Track.Artist)
	}
}

func TestLibraryViewEnqueueFocusedEmpty(t *testing.T) {
	v := LibraryView{}
	qa := &fakeQueueAppender{}
	cmd := v.EnqueueFocused(qa, 0)
	if cmd != nil {
		t.Error("EnqueueFocused empty: expected nil cmd")
	}
}

func TestLibraryViewBackspaceFilter(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})

	f := filter.State{Active: true, Text: "AB"}
	f, _ = f.Activate()
	v := LibraryView{index: idx, fuzzy: library.FakeFuzzySearcher{}, filter: f}
	prevText := v.filter.Text
	v.filter, _ = v.filter.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if v.filter.Text != prevText {
		v.cursor = 0
		v.scrollOffset = 0
		v = v.rebuildFilterCache()
	}
	if v.filter.Text != "A" {
		t.Errorf("BackspaceFilter: expected 'A', got %q", v.filter.Text)
	}
	if v.cursor != 0 {
		t.Errorf("BackspaceFilter: expected cursor=0, got %d", v.cursor)
	}
}

func TestLibraryViewBackspaceFilterInactiveNoOp(t *testing.T) {
	v := LibraryView{filter: filter.State{Text: "AB"}}
	v.filter, _ = v.filter.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if v.filter.Text != "AB" {
		t.Errorf("BackspaceFilter inactive: expected 'AB', got %q", v.filter.Text)
	}
}

func TestLibraryViewDeactivateFilter(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})

	v := LibraryView{
		index:  idx,
		fuzzy:  library.FakeFuzzySearcher{},
		filter: filter.State{Active: true, Text: "AB"},
		cursor: 5,
	}
	v.filter = v.filter.Deactivate()
	v.cursor = 0
	v.scrollOffset = 0
	v = v.rebuildFilterCache()
	if v.filter.Active {
		t.Error("DeactivateFilter: expected filter.Active=false")
	}
	if v.filter.Text != "" {
		t.Errorf("DeactivateFilter: expected filter.Text='', got %q", v.filter.Text)
	}
	if v.cursor != 0 {
		t.Errorf("DeactivateFilter: expected cursor=0, got %d", v.cursor)
	}
}

func TestLibraryViewCommitFilter(t *testing.T) {
	v := LibraryView{filter: filter.State{Active: true, Text: "test"}}
	v.filter = v.filter.Commit()
	if v.filter.Active {
		t.Error("CommitFilter: expected filter.Active=false")
	}
	if v.filter.Text != "test" {
		t.Errorf("CommitFilter: expected filter.Text='test', got %q", v.filter.Text)
	}
}
