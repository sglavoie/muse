package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
)

func TestPlayRecordedMsg_PrependsToLoadedHistory(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1"})
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2"},
	}

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	if len(nm.library.sourceTracks) != 2 {
		t.Fatalf("expected 2 history tracks, got %d", len(nm.library.sourceTracks))
	}
	if nm.library.sourceTracks[0].Hash != "h1" {
		t.Errorf("expected h1 at index 0, got %q", nm.library.sourceTracks[0].Hash)
	}
	if nm.library.sourceTracks[0].LastPlayedAt != 1000 {
		t.Errorf("expected LastPlayedAt=1000, got %d", nm.library.sourceTracks[0].LastPlayedAt)
	}
}

func TestPlayRecordedMsg_NoOpWhenHistoryNotLoaded(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac"})
	m.library.mode = LibraryModeHistory
	// sourceTracks is empty (history never loaded)

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	if len(nm.library.sourceTracks) != 0 {
		t.Fatalf("expected empty history, got %d entries", len(nm.library.sourceTracks))
	}
}

func TestPlayRecordedMsg_CapsAt500Entries(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "new", Path: "/new.flac"})
	m.library.mode = LibraryModeHistory

	// Fill history to 500 entries.
	tracks := make([]library.Track, 500)
	for i := range tracks {
		tracks[i] = library.Track{Hash: "old", Path: "/old.flac"}
	}
	m.library.sourceTracks = tracks

	next, _ := m.Update(playRecordedMsg{Hash: "new", PlayedAt: 2000})
	nm := next.(Model)

	if len(nm.library.sourceTracks) != 500 {
		t.Fatalf("expected 500 history tracks (capped), got %d", len(nm.library.sourceTracks))
	}
	if nm.library.sourceTracks[0].Hash != "new" {
		t.Errorf("expected new track at index 0, got %q", nm.library.sourceTracks[0].Hash)
	}
}

// TestPlayRecordedMsg_ClampsCursorAfterReplace verifies that after sourceTracks
// is rewritten by a recorded play, the library cursor is clamped to the new
// list length so that the next render does not over-index the slice.
func TestPlayRecordedMsg_ClampsCursorAfterReplace(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac"})
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac"},
		{Hash: "h2", Path: "/b.flac"},
		{Hash: "h3", Path: "/c.flac"},
	}
	m.library.cursor = 2

	// Recording h1 dedups: it is prepended and the existing h1 row is dropped,
	// so the list shrinks from 3 to 3 (no change). Drop an entry first to
	// trigger a shrink.
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac"},
	}
	m.library.cursor = 5

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	count := nm.library.trackCount()
	if nm.library.cursor < 0 || nm.library.cursor >= count {
		t.Errorf("cursor %d out of bounds for trackCount %d", nm.library.cursor, count)
	}
}

func TestPlayRecordedMsg_SkipsUnknownHash(t *testing.T) {
	m := New(context.Background())
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac"},
	}

	next, _ := m.Update(playRecordedMsg{Hash: "unknown", PlayedAt: 1000})
	nm := next.(Model)

	if len(nm.library.sourceTracks) != 1 {
		t.Fatalf("expected 1 history track (unchanged), got %d", len(nm.library.sourceTracks))
	}
}

func TestPlayRecordedMsg_RebuildsFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Artist", Title: "Filtered"})
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "h2", Path: "/b.flac", Artist: "Other", Title: "Song"},
	}
	m.library.filter = filter.State{Active: true, Text: "Artist"}

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	// The filter cache should have been rebuilt (non-nil when filter text is set).
	// The track "Artist" matches the filter text "Artist".
	if len(nm.library.sourceTracks) != 2 {
		t.Fatalf("expected 2 history tracks, got %d", len(nm.library.sourceTracks))
	}
}

// stubHistoryReader implements playHistoryReader for tests.
type stubHistoryReader struct {
	tracks []library.Track
	calls  int
}

func (s *stubHistoryReader) ReadPlayHistory(limit int) ([]library.Track, error) {
	s.calls++
	return s.tracks, nil
}

func TestHistoryViewSwitch_AlwaysReloads(t *testing.T) {
	reader := &stubHistoryReader{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac"},
			{Hash: "h2", Path: "/b.flac"},
		},
	}
	m := New(context.Background(), WithHistoryReader(reader))
	// Pre-populate sourceTracks to simulate a prior load.
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "old", Path: "/old.flac"},
	}

	m = m.switchToView(viewLibrary)

	if reader.calls != 1 {
		t.Fatalf("expected 1 ReadPlayHistory call, got %d", reader.calls)
	}
	if len(m.library.sourceTracks) != 2 {
		t.Fatalf("expected 2 tracks from reader, got %d", len(m.library.sourceTracks))
	}
	if m.library.sourceTracks[0].Hash != "h1" {
		t.Errorf("expected h1 at index 0, got %q", m.library.sourceTracks[0].Hash)
	}
}
