package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// setupPlayingModel returns a model that looks like a track is playing.
func setupPlayingModel(t *testing.T) Model {
	t.Helper()
	m := New(context.Background())
	m.playback.state = playback.StatePlaying
	m.statusBar.Artist = "Test Artist"
	m.statusBar.Title = "Test Song"
	m.playback.elapsedSecs = 30
	m.playback.totalSecs = 180
	m.playback.currentHash = "h1"
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Test Artist", Title: "Test Song"},
		{ID: 2, Hash: "h2", Path: "/b.flac"},
	}
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "Test Artist", Title: "Test Song", Album: "Test Album",
	})
	return m
}

// TestNowPlayingKeyShowsOverlay verifies that pressing I when a track is
// playing opens the now-playing info overlay.
func TestNowPlayingKeyShowsOverlay(t *testing.T) {
	m := setupPlayingModel(t)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command from I key")
	}
	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected nowPlayingInfo overlay to be set")
	}
	if nm.overlay.nowPlaying.Artist != "Test Artist" {
		t.Errorf("expected artist 'Test Artist', got %q", nm.overlay.nowPlaying.Artist)
	}
	if nm.overlay.nowPlaying.Title != "Test Song" {
		t.Errorf("expected title 'Test Song', got %q", nm.overlay.nowPlaying.Title)
	}
	if nm.overlay.nowPlaying.Album != "Test Album" {
		t.Errorf("expected album 'Test Album', got %q", nm.overlay.nowPlaying.Album)
	}
	if nm.overlay.nowPlaying.Elapsed != 30 {
		t.Errorf("expected elapsed 30, got %v", nm.overlay.nowPlaying.Elapsed)
	}
	if nm.overlay.nowPlaying.State != playback.StatePlaying {
		t.Errorf("expected state PlaybackPlaying, got %v", nm.overlay.nowPlaying.State)
	}
}

// TestNowPlayingKeyIdleShowsNotification verifies that pressing I when
// no track is playing shows a notification.
func TestNowPlayingKeyIdleShowsNotification(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StateIdle

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command")
	}
	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected no overlay when idle")
	}
	if nm.statusBar.TopNotification().Msg != "No track playing" {
		t.Fatalf("expected 'No track playing' notification, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestNowPlayingKeyStoppedShowsNotification verifies that pressing I when
// playback is stopped shows a notification.
func TestNowPlayingKeyStoppedShowsNotification(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StateStopped

	next, _ := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected no overlay when stopped")
	}
	if nm.statusBar.TopNotification().Msg != "No track playing" {
		t.Fatalf("expected 'No track playing' notification, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestNowPlayingToggleOff verifies that pressing I while overlay is active
// dismisses it.
func TestNowPlayingToggleOff(t *testing.T) {
	m := setupPlayingModel(t)
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{Artist: "A", Title: "T"}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command")
	}
	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected overlay to be dismissed on toggle")
	}
}

// TestNowPlayingEscDismisses verifies that Esc dismisses the overlay.
func TestNowPlayingEscDismisses(t *testing.T) {
	m := New(context.Background())
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{Artist: "A", Title: "T"}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected overlay to be nil after Esc")
	}
}

// TestNowPlayingSwallowsKeys verifies that while the overlay is active,
// other keys are swallowed.
func TestNowPlayingSwallowsKeys(t *testing.T) {
	m := New(context.Background())
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{Artist: "A", Title: "T"}
	m.activeView = viewLibrary

	// Press '3' (switch to queue) — should be swallowed.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	if nm.activeView != viewLibrary {
		t.Fatalf("expected view to stay at library, got %d", nm.activeView)
	}
	if nm.overlay.nowPlaying == nil {
		t.Fatal("overlay should still be active after swallowed key")
	}
}

// TestNowPlayingTickUpdates verifies that tick updates elapsed/total/state
// on the overlay.
func TestNowPlayingTickUpdates(t *testing.T) {
	m := New(context.Background())
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePlaying}
	m.playback.elapsedSecs = 45.5
	m.playback.totalSecs = 200.0
	m.playback.state = playback.StatePaused

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to still be active")
	}
	if nm.overlay.nowPlaying.Elapsed != 45.5 {
		t.Errorf("expected elapsed=45.5, got %v", nm.overlay.nowPlaying.Elapsed)
	}
	if nm.overlay.nowPlaying.Total != 200.0 {
		t.Errorf("expected total=200.0, got %v", nm.overlay.nowPlaying.Total)
	}
	if nm.overlay.nowPlaying.State != playback.StatePaused {
		t.Errorf("expected state PlaybackPaused, got %v", nm.overlay.nowPlaying.State)
	}
}

// TestNowPlayingCtrlCDismissesAndQuits verifies that ctrl+c dismisses
// the overlay and returns a quit command.
func TestNowPlayingCtrlCDismissesAndQuits(t *testing.T) {
	m := New(context.Background())
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{Artist: "A", Title: "T"}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	nm := next.(Model)

	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected overlay to be dismissed on ctrl+c")
	}
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.QuitMsg from ctrl+c command")
	}
}

// TestNowPlayingOverlayRenderedInView verifies that when the overlay is
// active, View() renders its content.
func TestNowPlayingOverlayRenderedInView(t *testing.T) {
	m := New(context.Background())
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{
		Artist:  "View Artist",
		Title:   "View Song",
		Elapsed: 10,
		Total:   60,
		State:   playback.StatePlaying,
	}
	m.termWidth = 80
	m.termHeight = 24

	v := m.View()
	if v.Content == "" {
		t.Fatal("expected non-empty View content with now-playing overlay active")
	}
}

// TestNowPlayingTrackChangeRefreshesOverlay verifies that when the track
// changes while the now-playing overlay is open, the overlay's artist, title,
// album, and state are updated to reflect the new track.
func TestNowPlayingTrackChangeRefreshesOverlay(t *testing.T) {
	m := setupPlayingModel(t)
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{
		Artist:  "Test Artist",
		Title:   "Test Song",
		Album:   "Test Album",
		Elapsed: 30,
		Total:   180,
		State:   playback.StatePlaying,
	}
	// Insert a second track.
	m.library.Index.Insert(library.Track{
		Hash: "h2", Path: "/b.flac", Artist: "New Artist", Title: "New Song", Album: "New Album",
	})

	next, _ := m.Update(playback.TrackChangedMsg{
		Hash:   "h2",
		Index:  1,
		Artist: "New Artist",
		Title:  "New Song",
	})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to remain active after track change")
	}
	if nm.overlay.nowPlaying.Artist != "New Artist" {
		t.Errorf("expected artist 'New Artist', got %q", nm.overlay.nowPlaying.Artist)
	}
	if nm.overlay.nowPlaying.Title != "New Song" {
		t.Errorf("expected title 'New Song', got %q", nm.overlay.nowPlaying.Title)
	}
	if nm.overlay.nowPlaying.Album != "New Album" {
		t.Errorf("expected album 'New Album', got %q", nm.overlay.nowPlaying.Album)
	}
	if nm.overlay.nowPlaying.Elapsed != 0 {
		t.Errorf("expected elapsed=0 after track change, got %v", nm.overlay.nowPlaying.Elapsed)
	}
	if nm.overlay.nowPlaying.State != playback.StatePlaying {
		t.Errorf("expected state PlaybackPlaying, got %v", nm.overlay.nowPlaying.State)
	}
}

// TestNowPlayingTrackChangeClearsStaleAlbum verifies that when the overlay is
// open and the track changes to one not in the library index, the album and
// playCount fields are reset to zero values instead of retaining stale data
// from the previous track.
func TestNowPlayingTrackChangeClearsStaleAlbum(t *testing.T) {
	m := setupPlayingModel(t)
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{
		Artist:    "Test Artist",
		Title:     "Test Song",
		Album:     "Test Album",
		PlayCount: 42,
		Elapsed:   30,
		Total:     180,
		State:     playback.StatePlaying,
	}

	// Send a TrackChangedMsg with a hash that is NOT in the library index.
	next, _ := m.Update(playback.TrackChangedMsg{
		Hash:   "unknown-hash",
		Index:  99,
		Artist: "Mystery Artist",
		Title:  "Mystery Song",
	})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to remain active after track change")
	}
	if nm.overlay.nowPlaying.Artist != "Mystery Artist" {
		t.Errorf("expected artist 'Mystery Artist', got %q", nm.overlay.nowPlaying.Artist)
	}
	if nm.overlay.nowPlaying.Title != "Mystery Song" {
		t.Errorf("expected title 'Mystery Song', got %q", nm.overlay.nowPlaying.Title)
	}
	if nm.overlay.nowPlaying.Album != "" {
		t.Errorf("expected album to be cleared, got %q", nm.overlay.nowPlaying.Album)
	}
	if nm.overlay.nowPlaying.PlayCount != 0 {
		t.Errorf("expected playCount to be cleared, got %d", nm.overlay.nowPlaying.PlayCount)
	}
}

// TestNowPlayingSessionRestore verifies that the overlay picks up artist/title
// from the library index when statusBar fields are empty (session restore case).
func TestNowPlayingSessionRestore(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StatePaused
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 10
	m.playback.totalSecs = 300
	// statusBar.artist and statusBar.title are intentionally empty (session restore).
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "Restored Artist", Title: "Restored Song", Album: "Restored Album",
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to be set")
	}
	if nm.overlay.nowPlaying.Artist != "Restored Artist" {
		t.Errorf("expected artist from index, got %q", nm.overlay.nowPlaying.Artist)
	}
	if nm.overlay.nowPlaying.Title != "Restored Song" {
		t.Errorf("expected title from index, got %q", nm.overlay.nowPlaying.Title)
	}
	if nm.overlay.nowPlaying.Album != "Restored Album" {
		t.Errorf("expected album from index, got %q", nm.overlay.nowPlaying.Album)
	}
}
