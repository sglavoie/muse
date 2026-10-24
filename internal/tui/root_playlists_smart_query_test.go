package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestPlaylistLoadErrorClearsPendingSmartQuery verifies that a failed smart
// open abandons its pending load, so a later regular playlist is not treated
// as smart (delete and reorder would otherwise no-op).
func TestPlaylistLoadErrorClearsPendingSmartQuery(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewPlaylists
	m.playlists.SmartQuery = &db.SmartPlaylistQuery{}
	m.playlists.PendingLoadID = 10

	next, _ := m.Update(playlistTracksLoadErrorMsg{err: errors.New("boom"), playlistID: 10})
	m = next.(Model)

	if m.playlists.SmartQuery != nil {
		t.Error("failed smart open should clear SmartQuery")
	}
	if m.playlists.PendingLoadID != 0 {
		t.Errorf("failed open should clear PendingLoadID, got %d", m.playlists.PendingLoadID)
	}
}

// TestStalePlaylistLoadErrorKeepsPendingLoad verifies that a failure for a
// load the user already moved past does not abandon the current one.
func TestStalePlaylistLoadErrorKeepsPendingLoad(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewPlaylists
	m.playlists.SmartQuery = &db.SmartPlaylistQuery{}
	m.playlists.PendingLoadID = 20

	next, _ := m.Update(playlistTracksLoadErrorMsg{err: errors.New("boom"), playlistID: 10})
	m = next.(Model)

	if m.playlists.SmartQuery == nil || m.playlists.PendingLoadID != 20 {
		t.Errorf("stale failure should keep pending load 20, got pending=%d smart=%v",
			m.playlists.PendingLoadID, m.playlists.SmartQuery != nil)
	}
}

// TestSmartRefreshErrorKeepsOpenSmartPlaylist verifies that a failed 'r'
// refresh at track level leaves the open smart playlist intact.
func TestSmartRefreshErrorKeepsOpenSmartPlaylist(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.SmartQuery = &db.SmartPlaylistQuery{}
	m.playlists.PendingLoadID = 10

	next, _ := m.Update(playlistTracksLoadErrorMsg{err: errors.New("boom"), playlistID: 10})
	m = next.(Model)

	if m.playlists.SmartQuery == nil || m.playlists.PendingLoadID != 10 {
		t.Error("refresh failure should keep the open smart playlist")
	}
}

// TestRegularOpenAfterSmartOpenClearsSmartQuery verifies that a quick
// smart-to-regular switch leaves the regular playlist non-smart.
func TestRegularOpenAfterSmartOpenClearsSmartQuery(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 10, Name: "Smart", IsSmart: true},
		{ID: 20, Name: "Regular", TrackCount: 1},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.ActiveView = viewPlaylists
	m.playlistReader = &stubPlaylistTrackReader{rows: []db.PlaylistTrackRow{{ID: 1, Title: "Song"}}}
	m.playlists.SmartQuery = &db.SmartPlaylistQuery{} // left by the smart open
	m.playlists.PendingLoadID = 10

	m, cmd, _ := m.openRegularPlaylist(playlists[1])
	if m.playlists.SmartQuery != nil {
		t.Fatal("regular open should clear SmartQuery")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.playlists.Level != pltui.TrackLevel || m.playlists.SmartQuery != nil {
		t.Errorf("regular playlist should open non-smart, level=%d smart=%v",
			m.playlists.Level, m.playlists.SmartQuery != nil)
	}
}
