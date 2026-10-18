package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestMixesDismissRoundTrip(t *testing.T) {
	stub := &stubDismissalWriter{}
	m := setupMixesAtTrackLevel(WithDismissalWriter(stub))

	m = enterVisualMode(m)
	m, cmd := m.handleMixDismiss()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from handleMixDismiss")
	}

	rawMsg := cmd()
	msg, ok := rawMsg.(mixDismissedMsg)
	if !ok {
		t.Fatalf("expected mixDismissedMsg, got %T", rawMsg)
	}
	if msg.hash != "mx1" {
		t.Errorf("expected hash mx1, got %q", msg.hash)
	}

	m = m.handleMixDismissedMsg(msg)
	if m.visual.active || m.activeViewSelectionCount() != 0 {
		t.Fatal("dismissal left a stale visual selection")
	}

	if _, found := m.dismissedHashes["mx1"]; !found {
		t.Error("expected mx1 in dismissedHashes after dismiss")
	}
	for _, tr := range m.mixes.CachedMixes[db.MixRediscover] {
		if tr.Hash == "mx1" {
			t.Error("expected mx1 removed from cachedMixes after dismiss")
		}
	}
	for _, tr := range m.mixes.TrackView.SourceTracks {
		if tr.Hash == "mx1" {
			t.Error("expected mx1 removed from trackView.sourceTracks after dismiss")
		}
	}
	if len(stub.dismissed) != 1 || stub.dismissed[0] != "mx1" {
		t.Errorf("expected stub to record dismiss for mx1, got %v", stub.dismissed)
	}
}

func TestMixesUndismissRoundTrip(t *testing.T) {
	stub := &stubDismissalWriter{}
	m := setupMixesAtTrackLevel(WithDismissalWriter(stub))
	m = m.handleMixDismissedMsg(mixDismissedMsg{hash: "mx1"})
	if m.mixes.FocusedTrack().Hash == "mx1" {
		t.Fatal("dismissed track remained focused")
	}

	m, cmd := m.handleMixUndismiss()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from handleMixUndismiss")
	}

	rawMsg := cmd()
	msg, ok := rawMsg.(mixUndismissedMsg)
	if !ok {
		t.Fatalf("expected mixUndismissedMsg, got %T", rawMsg)
	}
	if msg.hash != "mx1" {
		t.Errorf("expected hash mx1, got %q", msg.hash)
	}

	m = m.handleMixUndismissedMsg(msg)

	if _, found := m.dismissedHashes["mx1"]; found {
		t.Error("expected mx1 removed from dismissedHashes after undismiss")
	}
	if len(stub.undismissed) != 1 || stub.undismissed[0] != "mx1" {
		t.Errorf("expected stub to record undismiss for mx1, got %v", stub.undismissed)
	}
}

// TestMixesDismissPropagatesAcrossCachedMixes verifies that dismissing a
// track removes the hash from every cached mix, not just the active one.
// Without this sweep, navigating to another previously-loaded mix would
// still show the dismissed track until the cache is rebuilt.
func TestMixesDismissPropagatesAcrossCachedMixes(t *testing.T) {
	stub := &stubDismissalWriter{}
	m := setupMixesAtTrackLevel(WithDismissalWriter(stub))
	// Seed a second cached mix that contains the same hash (mx1) plus a
	// distinct hash. The dismiss should leave the unrelated hash alone but
	// scrub mx1 from this mix's cache too.
	m.mixes.CachedMixes[db.MixHiddenGems] = []library.Track{
		{Hash: "mx1", Title: "Dup", Artist: "ArtistX", Path: "/mx1.flac"},
		{Hash: "mxOther", Title: "Other", Artist: "ArtistZ", Path: "/mxo.flac"},
	}

	_, cmd := m.handleMixDismiss()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from handleMixDismiss")
	}
	msg := cmd().(mixDismissedMsg)
	m = m.handleMixDismissedMsg(msg)

	for _, tr := range m.mixes.CachedMixes[db.MixHiddenGems] {
		if tr.Hash == "mx1" {
			t.Error("expected mx1 removed from other cached mix after dismiss")
		}
	}
	// Unrelated hash must still be present.
	foundOther := false
	for _, tr := range m.mixes.CachedMixes[db.MixHiddenGems] {
		if tr.Hash == "mxOther" {
			foundOther = true
		}
	}
	if !foundOther {
		t.Error("dismiss must not remove unrelated hashes from other cached mixes")
	}
}

func TestMixesDismissNotAtTrackLevel(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)

	_, cmd := m.handleMixDismiss()

	if cmd != nil {
		t.Error("expected nil cmd from handleMixDismiss at list level")
	}
}

func TestMixesDismissNilWriter(t *testing.T) {
	m := setupMixesAtTrackLevel()

	_, cmd := m.handleMixDismiss()

	if cmd != nil {
		t.Error("expected nil cmd from handleMixDismiss when writer not wired")
	}
}

func TestMixesDismissPreservesCursorAndSort(t *testing.T) {
	tracks := []library.Track{
		{Hash: "mx1", Title: "Alpha", Artist: "ArtistX", Path: "/mx1.flac", Duration: 180},
		{Hash: "mx2", Title: "Beta", Artist: "ArtistX", Path: "/mx2.flac", Duration: 120},
		{Hash: "mx3", Title: "Gamma", Artist: "ArtistY", Path: "/mx3.flac", Duration: 240},
	}
	stub := &stubDismissalWriter{}
	m := New(context.Background(),
		WithRecommendationReader(&stubRecReader{sufficient: true, tracks: tracks}),
		WithDismissalWriter(stub),
	)
	m = m.switchToView(viewMixes)
	m.mixes.CachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.SelectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	m.mixes.TrackView.Cursor = 2
	m.mixes.TrackView.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.mixes.TrackView = m.mixes.TrackView.RecomputeSortedTracks()

	m, cmd := m.handleMixDismiss()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from handleMixDismiss")
	}
	rawMsg := cmd()
	msg := rawMsg.(mixDismissedMsg)
	m = m.handleMixDismissedMsg(msg)

	if m.mixes.TrackView.SortState.Field != sorting.SortTitle {
		t.Errorf("sort field reset: got %v, want SortTitle", m.mixes.TrackView.SortState.Field)
	}
	if m.mixes.TrackView.SortState.Dir != sorting.SortAsc {
		t.Errorf("sort dir reset: got %v, want SortAsc", m.mixes.TrackView.SortState.Dir)
	}
	if m.mixes.TrackView.Cursor > 1 {
		t.Errorf("cursor not clamped: got %d, want <= 1 (only 2 tracks remain)", m.mixes.TrackView.Cursor)
	}
	if len(m.mixes.TrackView.SourceTracks) != 2 {
		t.Errorf("sourceTracks length: got %d, want 2", len(m.mixes.TrackView.SourceTracks))
	}
	if len(m.mixes.TrackView.SortedTracks) != 2 {
		t.Errorf("sortedTracks length: got %d, want 2", len(m.mixes.TrackView.SortedTracks))
	}
}

type failingMixRestore struct {
	stubDismissalWriter
	err error
}

func (s *failingMixRestore) UndismissTrack(hash string) error {
	if s.err != nil {
		return s.err
	}
	return s.stubDismissalWriter.UndismissTrack(hash)
}

func TestMixRestoreAfterEmptyListAndFailedWrite(t *testing.T) {
	writer := &failingMixRestore{err: errors.New("database unavailable")}
	m := setupMixesAtTrackLevel(WithDismissalWriter(writer))
	for _, track := range sampleMixTracks() {
		m = m.handleMixDismissedMsg(mixDismissedMsg{hash: track.Hash})
	}
	if len(m.mixes.TrackView.SourceTracks) != 0 {
		t.Fatal("mix should be empty")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	if cmd == nil {
		t.Fatal("D cannot restore from empty mix")
	}
	next, _ = next.Update(cmd())
	m = next.(Model)
	if m.lastMixDismissed != "mx3" {
		t.Fatal("failed write lost retry target")
	}
	writer.err = nil
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	next, reload := next.Update(cmd())
	m = next.(Model)
	if m.lastMixDismissed != "" || reload == nil {
		t.Fatal("restore did not clear target and reload mix")
	}
	next, _ = m.Update(reload())
	m = next.(Model)
	if len(m.mixes.TrackView.SourceTracks) != 1 || m.mixes.TrackView.SourceTracks[0].Hash != "mx3" {
		t.Fatal("restored recommendation missing")
	}
}
