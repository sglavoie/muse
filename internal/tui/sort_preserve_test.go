package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestDequeuePreservesSortOrder(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "Zulu"},
		{ID: 20, Title: "Alpha"},
		{ID: 30, Title: "Mike"},
	}
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	// Delete underlying entry at index 1 ("Alpha")
	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)

	if nm.queue.sortState.Field != sorting.SortTitle {
		t.Errorf("expected sort preserved as SortTitle, got %v", nm.queue.sortState.Field)
	}
	if nm.queue.displayOrder == nil {
		t.Fatal("expected displayOrder to be recomputed, got nil")
	}
	if len(nm.queue.displayOrder) != 2 {
		t.Fatalf("expected displayOrder length 2, got %d", len(nm.queue.displayOrder))
	}
	// Remaining entries: Zulu (idx 0), Mike (idx 1). Sorted by title: Mike, Zulu.
	if nm.queue.entries[nm.queue.displayOrder[0]].Title != "Mike" {
		t.Errorf("first sorted entry should be Mike, got %q", nm.queue.entries[nm.queue.displayOrder[0]].Title)
	}
	if nm.queue.entries[nm.queue.displayOrder[1]].Title != "Zulu" {
		t.Errorf("second sorted entry should be Zulu, got %q", nm.queue.entries[nm.queue.displayOrder[1]].Title)
	}
}

func TestEnqueuePreservesSortOrder(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "Zulu"},
		{ID: 20, Title: "Mike"},
	}
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	m = m.handleTrackEnqueuedMsg(libtui.TrackEnqueuedMsg{
		Track:   library.Track{Hash: "abc", Title: "Alpha", Path: "/a.mp3"},
		QueueID: 30,
	})

	if m.queue.sortState.Field != sorting.SortTitle {
		t.Errorf("expected sort preserved as SortTitle, got %v", m.queue.sortState.Field)
	}
	if m.queue.displayOrder == nil {
		t.Fatal("expected displayOrder to be recomputed, got nil")
	}
	if len(m.queue.displayOrder) != 3 {
		t.Fatalf("expected displayOrder length 3, got %d", len(m.queue.displayOrder))
	}
	// Sorted by title: Alpha, Mike, Zulu
	if m.queue.entries[m.queue.displayOrder[0]].Title != "Alpha" {
		t.Errorf("first sorted entry should be Alpha, got %q", m.queue.entries[m.queue.displayOrder[0]].Title)
	}
}

func TestBatchEnqueuePreservesSortOrder(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "Zulu"},
	}
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	m = m.handleBatchEnqueuedMsg(BatchEnqueuedMsg{
		Entries: []QueueEntry{
			{ID: 20, Title: "Alpha"},
			{ID: 30, Title: "Mike"},
		},
	})

	if m.queue.sortState.Field != sorting.SortTitle {
		t.Errorf("expected sort preserved, got %v", m.queue.sortState.Field)
	}
	if m.queue.displayOrder == nil {
		t.Fatal("expected displayOrder recomputed")
	}
	// Sorted: Alpha, Mike, Zulu
	if m.queue.entries[m.queue.displayOrder[0]].Title != "Alpha" {
		t.Errorf("first should be Alpha, got %q", m.queue.entries[m.queue.displayOrder[0]].Title)
	}
}

func TestDedupPreservesSortOrder(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Hash: "h1", Title: "Zulu"},
		{ID: 20, Hash: "h2", Title: "Alpha"},
		{ID: 30, Hash: "h1", Title: "Zulu"}, // duplicate
	}
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	m, _ = m.handleQueueDedupMsg()

	if m.queue.sortState.Field != sorting.SortTitle {
		t.Errorf("expected sort preserved, got %v", m.queue.sortState.Field)
	}
	if m.queue.displayOrder == nil {
		t.Fatal("expected displayOrder recomputed")
	}
	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(m.queue.entries))
	}
}

func TestShuffleStillResetsSortOrder(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "A"},
		{ID: 20, Title: "B"},
		{ID: 30, Title: "C"},
	}
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	m, _ = m.handleQueueShuffleMsg()

	if m.queue.sortState.Field != sorting.SortDefault {
		t.Errorf("expected sort reset after shuffle, got %v", m.queue.sortState.Field)
	}
	if m.queue.displayOrder != nil {
		t.Error("expected displayOrder nil after shuffle")
	}
}

func TestDequeueWithNoSortIsNoop(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "A"},
		{ID: 20, Title: "B"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)

	if nm.queue.sortState.Field != sorting.SortDefault {
		t.Errorf("expected sort to remain default, got %v", nm.queue.sortState.Field)
	}
	if nm.queue.displayOrder != nil {
		t.Error("expected displayOrder to remain nil when no sort active")
	}
}

func TestQueueSortDirPreservesFilteredFocusedEntry(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "Beta", Artist: "keep"},
		{ID: 20, Title: "Charlie", Artist: "keep"},
		{ID: 30, Title: "Alpha", Artist: "skip"},
	}
	m.queue.filter.Text = "keep"
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()
	m.queue.cursor = 1 // "Charlie" in the filtered ascending view.

	next, _, ok := m.handleSortDir(sorting.SortDesc)
	if !ok {
		t.Fatal("expected queue sort dir change to be handled")
	}
	nm := next.(Model)
	filtered := nm.queue.filteredEntries()

	if nm.queue.cursor < 0 || nm.queue.cursor >= len(filtered) {
		t.Fatalf("cursor %d out of filtered range %d", nm.queue.cursor, len(filtered))
	}
	if got := filtered[nm.queue.cursor].ID; got != 20 {
		t.Fatalf("focused entry after sort dir change: got ID %d, want 20", got)
	}
}

func TestPlaylistDeletePreservesSortOrder(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd), WithPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Zebra"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "Middle"},
	}))
	m.activeView = viewPlaylists
	m.playlists.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.playlists = m.playlists.RecomputePlaylistSort()
	m.playlists.Cursor = 0 // "Alpha" in sorted view

	// 'd' triggers playlist delete
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playlists.SortState.Field != sorting.SortName {
		t.Errorf("expected sort preserved as SortName, got %v", nm.playlists.SortState.Field)
	}
	if nm.playlists.SortedPlaylists == nil {
		t.Error("expected sortedPlaylists to be recomputed, got nil")
	}
}

func TestPlaylistTrackRemovePreservesSortOrder(t *testing.T) {
	pd := &stubPlaylistDeleter{}
	m := New(context.Background(), WithPlaylistDeleter(pd))
	m.activeView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Zulu", Artist: "A", DurationMs: 1000},
		{ID: 20, Position: 2, Title: "Alpha", Artist: "B", DurationMs: 2000},
		{ID: 30, Position: 3, Title: "Mike", Artist: "C", DurationMs: 3000},
	}
	m.playlists.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.playlists = m.playlists.RecomputePlaylistSort()
	m.playlists.Cursor = 0 // "Alpha" in sorted view

	// 'd' triggers track delete
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playlists.SortState.Field != sorting.SortTitle {
		t.Errorf("expected sort preserved as SortTitle, got %v", nm.playlists.SortState.Field)
	}
	if nm.playlists.SortedTrackOrder == nil {
		t.Error("expected sortedTrackOrder to be recomputed, got nil")
	}
	if len(nm.playlists.Tracks) != 2 {
		t.Fatalf("expected 2 tracks remaining, got %d", len(nm.playlists.Tracks))
	}
}

func TestPlaylistShuffleStillResetsSortOrder(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "A"},
		{ID: 20, Position: 2, Title: "B"},
		{ID: 30, Position: 3, Title: "C"},
	}
	m.playlists.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.playlists = m.playlists.RecomputePlaylistSort()

	m, _ = m.handlePlaylistShuffleMsg()

	if m.playlists.SortState.Field != sorting.SortDefault {
		t.Errorf("expected sort reset after shuffle, got %v", m.playlists.SortState.Field)
	}
	if m.playlists.SortedTrackOrder != nil {
		t.Error("expected sortedTrackOrder nil after shuffle")
	}
}
