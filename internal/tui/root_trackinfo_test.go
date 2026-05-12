package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// setupTrackInfoModel returns a model with a focused track in the library.
func setupTrackInfoModel(t *testing.T) Model {
	t.Helper()
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/music/test.flac", Artist: "Test Artist", Title: "Test Song",
	})
	// Use the queue view with a focused entry instead — simpler to set up.
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/music/test.flac", Artist: "Test Artist", Title: "Test Song"},
	}
	m.queue.cursor = 0
	return m
}

// TestTrackInfoKeyNoTrack verifies that pressing i with no focused track
// shows a notification.
func TestTrackInfoKeyNoTrack(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command when no track focused")
	}
	if nm.overlay.trackInfo != nil {
		t.Fatal("expected no overlay when no track focused")
	}
	if nm.statusBar.topNotification().msg != "No track to inspect" {
		t.Fatalf("expected 'No track to inspect' notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestTrackInfoKeyReturnsCmd verifies that pressing i with a focused track
// returns a non-nil command (the async exiftool invocation).
func TestTrackInfoKeyReturnsCmd(t *testing.T) {
	m := setupTrackInfoModel(t)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil command for track info")
	}
	// Overlay should not be set yet — it's set when trackInfoMsg arrives.
	if nm.overlay.trackInfo != nil {
		t.Fatal("overlay should not be set before trackInfoMsg arrives")
	}
}

// TestTrackInfoMsgSuccess verifies that receiving a trackInfoMsg with tags
// sets the overlay.
func TestTrackInfoMsgSuccess(t *testing.T) {
	m := New(context.Background())

	tags := map[string]any{
		"Title":  "Song",
		"Artist": "Band",
	}
	next, _ := m.Update(trackInfoMsg{path: "/music/song.flac", tags: tags})
	nm := next.(Model)

	if nm.overlay.trackInfo == nil {
		t.Fatal("expected trackInfo overlay to be set")
	}
	if nm.overlay.trackInfo.path != "/music/song.flac" {
		t.Errorf("expected path '/music/song.flac', got %q", nm.overlay.trackInfo.path)
	}
}

// TestTrackInfoMsgError verifies that receiving a trackInfoMsg with an error
// shows a notification and does not set the overlay.
func TestTrackInfoMsgError(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(trackInfoMsg{
		path: "/music/song.flac",
		err:  errors.New("exiftool failed"),
	})
	nm := next.(Model)

	if nm.overlay.trackInfo != nil {
		t.Fatal("expected no overlay on error")
	}
	if nm.statusBar.topNotification().msg == "" {
		t.Fatal("expected error notification")
	}
}

// TestTrackInfoToggleOff verifies that pressing i while overlay is active
// dismisses it.
func TestTrackInfoToggleOff(t *testing.T) {
	m := New(context.Background())
	overlay := newTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command on dismiss")
	}
	if nm.overlay.trackInfo != nil {
		t.Fatal("expected overlay to be dismissed on toggle")
	}
}

// TestTrackInfoEscDismisses verifies that Esc dismisses the overlay.
func TestTrackInfoEscDismisses(t *testing.T) {
	m := New(context.Background())
	overlay := newTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.trackInfo != nil {
		t.Fatal("expected overlay to be nil after Esc")
	}
}

// TestTrackInfoSwallowsKeys verifies that while the overlay is active,
// other keys are swallowed.
func TestTrackInfoSwallowsKeys(t *testing.T) {
	m := New(context.Background())
	overlay := newTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay
	m.activeView = viewLibrary

	// Press '3' (switch to queue) — should be swallowed.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	if nm.activeView != viewLibrary {
		t.Fatalf("expected view to stay at library, got %d", nm.activeView)
	}
	if nm.overlay.trackInfo == nil {
		t.Fatal("overlay should still be active after swallowed key")
	}
}

// TestTrackInfoCtrlCDismissesAndQuits verifies that ctrl+c dismisses
// the overlay and returns a quit command.
func TestTrackInfoCtrlCDismissesAndQuits(t *testing.T) {
	m := New(context.Background())
	overlay := newTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	nm := next.(Model)

	if nm.overlay.trackInfo != nil {
		t.Fatal("expected overlay to be dismissed on ctrl+c")
	}
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.QuitMsg from ctrl+c command")
	}
}

// TestTrackInfoOverlayRenderedInView verifies that when the overlay is
// active, View() renders its content.
func TestTrackInfoOverlayRenderedInView(t *testing.T) {
	m := New(context.Background())
	overlay := newTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "View Song"}, 0, nil)
	m.overlay.trackInfo = &overlay
	m.termWidth = 80
	m.termHeight = 24

	v := m.View()
	if v.Content == "" {
		t.Fatal("expected non-empty View content with track info overlay active")
	}
}

// TestTrackInfoPathUnknown verifies that pressing i on a track with no path
// (and not in the index) shows a notification.
func TestTrackInfoPathUnknown(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	// Add a queue entry with no path and a hash that isn't in the index.
	m.queue.entries = []QueueEntry{{Hash: "orphan", Display: "Orphan"}}
	m.queue.cursor = 0

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command for unknown path")
	}
	if nm.overlay.trackInfo != nil {
		t.Fatal("expected no overlay for unknown path")
	}
	if nm.statusBar.topNotification().msg != "Track path unknown" {
		t.Fatalf("expected 'Track path unknown' notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestTrackInfoYankPath verifies that pressing y while the track info overlay
// is active returns a clipboard command and shows a notification.
func TestTrackInfoYankPath(t *testing.T) {
	m := New(context.Background())
	overlay := newTrackInfoOverlay("/music/artist/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil command for clipboard copy")
	}
	if nm.overlay.trackInfo == nil {
		t.Fatal("overlay should remain active after yank")
	}
	if nm.statusBar.topNotification().msg != "Path copied to clipboard" {
		t.Fatalf("expected 'Path copied to clipboard' notification, got %q", nm.statusBar.topNotification().msg)
	}
}
