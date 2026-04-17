package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// setupPlayingModel returns a model that looks like a track is playing.
func setupPlayingModel(t *testing.T) Model {
	t.Helper()
	m := New(context.Background())
	m.playback.state = PlaybackPlaying
	m.statusBar.artist = "Test Artist"
	m.statusBar.title = "Test Song"
	m.playback.elapsedSecs = 30
	m.playback.totalSecs = 180
	m.playback.currentHash = "h1"
	m.library.index.Insert(library.Track{
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
	if nm.overlay.nowPlaying.artist != "Test Artist" {
		t.Errorf("expected artist 'Test Artist', got %q", nm.overlay.nowPlaying.artist)
	}
	if nm.overlay.nowPlaying.title != "Test Song" {
		t.Errorf("expected title 'Test Song', got %q", nm.overlay.nowPlaying.title)
	}
	if nm.overlay.nowPlaying.album != "Test Album" {
		t.Errorf("expected album 'Test Album', got %q", nm.overlay.nowPlaying.album)
	}
	if nm.overlay.nowPlaying.elapsed != 30 {
		t.Errorf("expected elapsed 30, got %v", nm.overlay.nowPlaying.elapsed)
	}
	if nm.overlay.nowPlaying.state != PlaybackPlaying {
		t.Errorf("expected state PlaybackPlaying, got %v", nm.overlay.nowPlaying.state)
	}
}

// TestNowPlayingKeyIdleShowsNotification verifies that pressing I when
// no track is playing shows a notification.
func TestNowPlayingKeyIdleShowsNotification(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackIdle

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command")
	}
	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected no overlay when idle")
	}
	if nm.statusBar.notification.msg != "No track playing" {
		t.Fatalf("expected 'No track playing' notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestNowPlayingKeyStoppedShowsNotification verifies that pressing I when
// playback is stopped shows a notification.
func TestNowPlayingKeyStoppedShowsNotification(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackStopped

	next, _ := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if nm.overlay.nowPlaying != nil {
		t.Fatal("expected no overlay when stopped")
	}
	if nm.statusBar.notification.msg != "No track playing" {
		t.Fatalf("expected 'No track playing' notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestNowPlayingToggleOff verifies that pressing I while overlay is active
// dismisses it.
func TestNowPlayingToggleOff(t *testing.T) {
	m := setupPlayingModel(t)
	m.overlay.nowPlaying = &NowPlayingOverlay{artist: "A", title: "T"}

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
	m.overlay.nowPlaying = &NowPlayingOverlay{artist: "A", title: "T"}

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
	m.overlay.nowPlaying = &NowPlayingOverlay{artist: "A", title: "T"}
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
	m.overlay.nowPlaying = &NowPlayingOverlay{artist: "A", title: "T", state: PlaybackPlaying}
	m.playback.elapsedSecs = 45.5
	m.playback.totalSecs = 200.0
	m.playback.state = PlaybackPaused

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to still be active")
	}
	if nm.overlay.nowPlaying.elapsed != 45.5 {
		t.Errorf("expected elapsed=45.5, got %v", nm.overlay.nowPlaying.elapsed)
	}
	if nm.overlay.nowPlaying.total != 200.0 {
		t.Errorf("expected total=200.0, got %v", nm.overlay.nowPlaying.total)
	}
	if nm.overlay.nowPlaying.state != PlaybackPaused {
		t.Errorf("expected state PlaybackPaused, got %v", nm.overlay.nowPlaying.state)
	}
}

// TestNowPlayingCtrlCDismissesAndQuits verifies that ctrl+c dismisses
// the overlay and returns a quit command.
func TestNowPlayingCtrlCDismissesAndQuits(t *testing.T) {
	m := New(context.Background())
	m.overlay.nowPlaying = &NowPlayingOverlay{artist: "A", title: "T"}

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
	m.overlay.nowPlaying = &NowPlayingOverlay{
		artist:  "View Artist",
		title:   "View Song",
		elapsed: 10,
		total:   60,
		state:   PlaybackPlaying,
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
	m.overlay.nowPlaying = &NowPlayingOverlay{
		artist:  "Test Artist",
		title:   "Test Song",
		album:   "Test Album",
		elapsed: 30,
		total:   180,
		state:   PlaybackPlaying,
	}
	// Insert a second track.
	m.library.index.Insert(library.Track{
		Hash: "h2", Path: "/b.flac", Artist: "New Artist", Title: "New Song", Album: "New Album",
	})

	next, _ := m.Update(TrackChangedMsg{
		Hash:   "h2",
		Index:  1,
		Artist: "New Artist",
		Title:  "New Song",
	})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to remain active after track change")
	}
	if nm.overlay.nowPlaying.artist != "New Artist" {
		t.Errorf("expected artist 'New Artist', got %q", nm.overlay.nowPlaying.artist)
	}
	if nm.overlay.nowPlaying.title != "New Song" {
		t.Errorf("expected title 'New Song', got %q", nm.overlay.nowPlaying.title)
	}
	if nm.overlay.nowPlaying.album != "New Album" {
		t.Errorf("expected album 'New Album', got %q", nm.overlay.nowPlaying.album)
	}
	if nm.overlay.nowPlaying.elapsed != 0 {
		t.Errorf("expected elapsed=0 after track change, got %v", nm.overlay.nowPlaying.elapsed)
	}
	if nm.overlay.nowPlaying.state != PlaybackPlaying {
		t.Errorf("expected state PlaybackPlaying, got %v", nm.overlay.nowPlaying.state)
	}
}

// TestNowPlayingTrackChangeClearsStaleAlbum verifies that when the overlay is
// open and the track changes to one not in the library index, the album and
// playCount fields are reset to zero values instead of retaining stale data
// from the previous track.
func TestNowPlayingTrackChangeClearsStaleAlbum(t *testing.T) {
	m := setupPlayingModel(t)
	m.overlay.nowPlaying = &NowPlayingOverlay{
		artist:    "Test Artist",
		title:     "Test Song",
		album:     "Test Album",
		playCount: 42,
		elapsed:   30,
		total:     180,
		state:     PlaybackPlaying,
	}

	// Send a TrackChangedMsg with a hash that is NOT in the library index.
	next, _ := m.Update(TrackChangedMsg{
		Hash:   "unknown-hash",
		Index:  99,
		Artist: "Mystery Artist",
		Title:  "Mystery Song",
	})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to remain active after track change")
	}
	if nm.overlay.nowPlaying.artist != "Mystery Artist" {
		t.Errorf("expected artist 'Mystery Artist', got %q", nm.overlay.nowPlaying.artist)
	}
	if nm.overlay.nowPlaying.title != "Mystery Song" {
		t.Errorf("expected title 'Mystery Song', got %q", nm.overlay.nowPlaying.title)
	}
	if nm.overlay.nowPlaying.album != "" {
		t.Errorf("expected album to be cleared, got %q", nm.overlay.nowPlaying.album)
	}
	if nm.overlay.nowPlaying.playCount != 0 {
		t.Errorf("expected playCount to be cleared, got %d", nm.overlay.nowPlaying.playCount)
	}
}

// TestNowPlayingSessionRestore verifies that the overlay picks up artist/title
// from the library index when statusBar fields are empty (session restore case).
func TestNowPlayingSessionRestore(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackPaused
	m.playback.currentHash = "h1"
	m.playback.elapsedSecs = 10
	m.playback.totalSecs = 300
	// statusBar.artist and statusBar.title are intentionally empty (session restore).
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "Restored Artist", Title: "Restored Song", Album: "Restored Album",
	})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	nm := next.(Model)

	if nm.overlay.nowPlaying == nil {
		t.Fatal("expected overlay to be set")
	}
	if nm.overlay.nowPlaying.artist != "Restored Artist" {
		t.Errorf("expected artist from index, got %q", nm.overlay.nowPlaying.artist)
	}
	if nm.overlay.nowPlaying.title != "Restored Song" {
		t.Errorf("expected title from index, got %q", nm.overlay.nowPlaying.title)
	}
	if nm.overlay.nowPlaying.album != "Restored Album" {
		t.Errorf("expected album from index, got %q", nm.overlay.nowPlaying.album)
	}
}
