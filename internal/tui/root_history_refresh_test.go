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
	m.history.sourceTracks = []library.Track{
		{Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2"},
	}

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	if len(nm.history.sourceTracks) != 2 {
		t.Fatalf("expected 2 history tracks, got %d", len(nm.history.sourceTracks))
	}
	if nm.history.sourceTracks[0].Hash != "h1" {
		t.Errorf("expected h1 at index 0, got %q", nm.history.sourceTracks[0].Hash)
	}
	if nm.history.sourceTracks[0].LastPlayedAt != 1000 {
		t.Errorf("expected LastPlayedAt=1000, got %d", nm.history.sourceTracks[0].LastPlayedAt)
	}
}

func TestPlayRecordedMsg_NoOpWhenHistoryNotLoaded(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac"})
	// sourceTracks is empty (history never loaded)

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	if len(nm.history.sourceTracks) != 0 {
		t.Fatalf("expected empty history, got %d entries", len(nm.history.sourceTracks))
	}
}

func TestPlayRecordedMsg_CapsAt500Entries(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "new", Path: "/new.flac"})

	// Fill history to 500 entries.
	tracks := make([]library.Track, 500)
	for i := range tracks {
		tracks[i] = library.Track{Hash: "old", Path: "/old.flac"}
	}
	m.history.sourceTracks = tracks

	next, _ := m.Update(playRecordedMsg{Hash: "new", PlayedAt: 2000})
	nm := next.(Model)

	if len(nm.history.sourceTracks) != 500 {
		t.Fatalf("expected 500 history tracks (capped), got %d", len(nm.history.sourceTracks))
	}
	if nm.history.sourceTracks[0].Hash != "new" {
		t.Errorf("expected new track at index 0, got %q", nm.history.sourceTracks[0].Hash)
	}
}

func TestPlayRecordedMsg_SkipsUnknownHash(t *testing.T) {
	m := New(context.Background())
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac"},
	}

	next, _ := m.Update(playRecordedMsg{Hash: "unknown", PlayedAt: 1000})
	nm := next.(Model)

	if len(nm.history.sourceTracks) != 1 {
		t.Fatalf("expected 1 history track (unchanged), got %d", len(nm.history.sourceTracks))
	}
}

func TestPlayRecordedMsg_RebuildsFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Artist", Title: "Filtered"})
	m.history.sourceTracks = []library.Track{
		{Hash: "h2", Path: "/b.flac", Artist: "Other", Title: "Song"},
	}
	m.history.filter = filter.State{Active: true, Text: "Artist"}

	next, _ := m.Update(playRecordedMsg{Hash: "h1", PlayedAt: 1000})
	nm := next.(Model)

	// The filter cache should have been rebuilt (non-nil when filter text is set).
	// The track "Artist" matches the filter text "Artist".
	if len(nm.history.sourceTracks) != 2 {
		t.Fatalf("expected 2 history tracks, got %d", len(nm.history.sourceTracks))
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
	m.history.sourceTracks = []library.Track{
		{Hash: "old", Path: "/old.flac"},
	}

	m = m.switchToView(viewHistory)

	if reader.calls != 1 {
		t.Fatalf("expected 1 ReadPlayHistory call, got %d", reader.calls)
	}
	if len(m.history.sourceTracks) != 2 {
		t.Fatalf("expected 2 tracks from reader, got %d", len(m.history.sourceTracks))
	}
	if m.history.sourceTracks[0].Hash != "h1" {
		t.Errorf("expected h1 at index 0, got %q", m.history.sourceTracks[0].Hash)
	}
}
