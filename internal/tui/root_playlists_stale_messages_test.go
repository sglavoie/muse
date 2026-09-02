package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestStalePlaylistTracksLoadedMsgDiscarded verifies that a
// playlistTracksLoadedMsg whose playlistID does not match the pending load
// is silently discarded — for example when the user opens playlist A, backs
// out, then opens playlist B before A's async result arrives.
func TestStalePlaylistTracksLoadedMsgDiscarded(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 10, Name: "Alpha", TrackCount: 1},
		{ID: 20, Name: "Beta", TrackCount: 1},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.ActiveView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Simulate user opening playlist 10: pendingLoadID set to 10.
	m.playlists.PendingLoadID = 10

	// User backs out before async load completes — pendingLoadID cleared.
	m.playlists = m.playlists.BackToList()
	if m.playlists.PendingLoadID != 0 {
		t.Fatal("backToList should clear pendingLoadID")
	}

	// User opens playlist 20 — pendingLoadID set to 20.
	m.playlists.PendingLoadID = 20

	// Stale result from playlist 10 arrives — should be discarded.
	staleMsg := playlistTracksLoadedMsg{
		tracks:     []db.PlaylistTrackRow{{ID: 1, Title: "Stale Track", DurationMs: 60000}},
		playlistID: 10,
	}
	next, _ = m.Update(staleMsg)
	m = next.(Model)

	if m.playlists.Level != pltui.ListLevel {
		t.Errorf("stale msg should not transition to TrackLevel, got level=%d", m.playlists.Level)
	}
	if len(m.playlists.Tracks) != 0 {
		t.Errorf("stale msg should not load tracks, got %d tracks", len(m.playlists.Tracks))
	}

	// Correct result from playlist 20 arrives — should be applied.
	correctMsg := playlistTracksLoadedMsg{
		tracks:     []db.PlaylistTrackRow{{ID: 2, Title: "Good Track", DurationMs: 90000}},
		playlistID: 20,
	}
	next, _ = m.Update(correctMsg)
	m = next.(Model)

	if m.playlists.Level != pltui.TrackLevel {
		t.Errorf("correct msg should transition to TrackLevel, got level=%d", m.playlists.Level)
	}
	if len(m.playlists.Tracks) != 1 || m.playlists.Tracks[0].Title != "Good Track" {
		t.Errorf("correct msg should load tracks, got %v", m.playlists.Tracks)
	}
}

// TestStaleRefreshMsgDiscarded verifies that a refresh playlistTracksLoadedMsg
// with a mismatched playlistID is discarded.
func TestStaleRefreshMsgDiscarded(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{{ID: 1, Title: "Original", DurationMs: 60000}}
	m.playlists.PendingLoadID = 20

	staleRefresh := playlistTracksLoadedMsg{
		tracks:     []db.PlaylistTrackRow{{ID: 2, Title: "Stale Refresh", DurationMs: 30000}},
		isRefresh:  true,
		playlistID: 99, // wrong ID
	}
	next, _ := m.Update(staleRefresh)
	m = next.(Model)

	if len(m.playlists.Tracks) != 1 || m.playlists.Tracks[0].Title != "Original" {
		t.Errorf("stale refresh should not replace tracks, got %v", m.playlists.Tracks)
	}
}
