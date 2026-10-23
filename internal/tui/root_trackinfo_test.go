package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// setupTrackInfoModel returns a model with a focused track in the library.
func setupTrackInfoModel(t *testing.T) Model {
	t.Helper()
	m := New(context.Background())
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/music/test.flac", Artist: "Test Artist", Title: "Test Song",
	})
	// Use the queue view with a focused entry instead — simpler to set up.
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Hash: "h1", Path: "/music/test.flac", Artist: "Test Artist", Title: "Test Song"},
	}
	m.queue.Cursor = 0
	return m
}

// TestTrackInfoKeyNoTrack verifies that pressing i with no focused track
// shows a notification.
func TestTrackInfoKeyNoTrack(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewLibrary

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command when no track focused")
	}
	if nm.overlay.trackInfo != nil {
		t.Fatal("expected no overlay when no track focused")
	}
	if nm.statusBar.TopNotification().Msg != "No track to inspect" {
		t.Fatalf("expected 'No track to inspect' notification, got %q", nm.statusBar.TopNotification().Msg)
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
	// Stored metadata is available before the probe completes.
	if nm.overlay.trackInfo == nil {
		t.Fatal("overlay should open before trackInfoMsg arrives")
	}
}

// TestTrackInfoMsgSuccess verifies that receiving a trackInfoMsg with tags
// sets the overlay.
func TestTrackInfoMsgSuccess(t *testing.T) {
	m := startTrackInfoForTest(t)

	tags := map[string]any{
		"Title":  "Song",
		"Artist": "Band",
	}
	next, _ := m.Update(overlay.TrackInfoMsg{Request: 1, Path: "/music/test.flac", Tags: tags})
	nm := next.(Model)

	if nm.overlay.trackInfo == nil {
		t.Fatal("expected trackInfo overlay to be set")
	}
	if nm.overlay.trackInfo.Path != "/music/test.flac" {
		t.Errorf("expected path '/music/test.flac', got %q", nm.overlay.trackInfo.Path)
	}
}

// TestTrackInfoMsgError verifies that receiving a trackInfoMsg with an error
// retains library metadata and explains the unavailable file details.
func TestTrackInfoMsgError(t *testing.T) {
	m := startTrackInfoForTest(t)

	next, _ := m.Update(overlay.TrackInfoMsg{
		Request: 1, Path: "/music/test.flac",
		Err: errors.New("exiftool failed"),
	})
	nm := next.(Model)

	if nm.overlay.trackInfo == nil {
		t.Fatal("failed probe discarded stored metadata")
	}
	out := nm.overlay.trackInfo.Render(100, 30, *nm.styles)
	for _, want := range []string{"Test Song", "File details unavailable: exiftool failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s: %s", want, out)
		}
	}
}

// TestTrackInfoToggleOff verifies that pressing i while overlay is active
// dismisses it.
func TestTrackInfoToggleOff(t *testing.T) {
	m := New(context.Background())
	overlay := overlay.NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
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
	overlay := overlay.NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
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
	overlay := overlay.NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay
	m.ActiveView = viewLibrary

	// Press '3' (switch to queue) — should be swallowed.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	if nm.ActiveView != viewLibrary {
		t.Fatalf("expected view to stay at library, got %d", nm.ActiveView)
	}
	if nm.overlay.trackInfo == nil {
		t.Fatal("overlay should still be active after swallowed key")
	}
}

// TestTrackInfoCtrlCDismissesAndQuits verifies that ctrl+c dismisses
// the overlay and returns a quit command.
func TestTrackInfoCtrlCDismissesAndQuits(t *testing.T) {
	m := New(context.Background())
	overlay := overlay.NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "T"}, 0, nil)
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
	overlay := overlay.NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "View Song"}, 0, nil)
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
	m.ActiveView = viewQueue
	// Add a queue entry with no path and a hash that isn't in the index.
	m.queue.Entries = []queue.Entry{{Hash: "orphan", Display: "Orphan"}}
	m.queue.Cursor = 0

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command for unknown path")
	}
	if nm.overlay.trackInfo == nil {
		t.Fatal("expected stored metadata even without a path")
	}
	if !strings.Contains(nm.overlay.trackInfo.Render(100, 30, *nm.styles), "track path unknown") {
		t.Fatal("missing path explanation")
	}
}

// TestTrackInfoYankPath verifies that pressing y while the track info overlay
// is active returns a clipboard command without claiming success before it finishes.
func TestTrackInfoYankPath(t *testing.T) {
	m := New(context.Background())
	overlay := overlay.NewTrackInfoOverlay("/music/artist/song.flac", map[string]any{"Title": "T"}, 0, nil)
	m.overlay.trackInfo = &overlay

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil command for clipboard copy")
	}
	if nm.overlay.trackInfo == nil {
		t.Fatal("overlay should remain active after yank")
	}
	if nm.statusBar.TopNotification().Msg != "" {
		t.Fatalf("unexpected notification before clipboard result: %q", nm.statusBar.TopNotification().Msg)
	}
}

func TestTrackInfoUsesIndexedMetadataAndScrolls(t *testing.T) {
	m := startTrackInfoForTest(t)
	m.termWidth, m.termHeight = 60, 15
	next, _ := m.Update(overlay.TrackInfoMsg{Request: 1, Path: "/music/test.flac", Tags: map[string]any{"Title": "Original title", "Comment": strings.Repeat("Comment ", 30)}})
	m = next.(Model)
	first := m.View().Content
	if !strings.Contains(first, "Test Song") {
		t.Fatal("library title missing")
	}
	next, _, handled := m.handleTrackInfoKeyPress(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !handled || next.View().Content == first {
		t.Fatal("scroll key did not reach overlay")
	}
	if m.View().Content != first {
		t.Fatal("scroll mutated previous model")
	}
}

func startTrackInfoForTest(t *testing.T) Model {
	t.Helper()
	m := setupTrackInfoModel(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	return next.(Model)
}

func TestTrackInfoLateResultsNeverReopenOrOverwrite(t *testing.T) {
	m := startTrackInfoForTest(t)
	result := overlay.TrackInfoMsg{Request: 1, Path: "/music/test.flac", Tags: map[string]any{"Comment": "STALE"}}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	m = m.handleTrackInfoResult(result)
	if m.overlay.trackInfo != nil {
		t.Fatal("dismissed info reopened")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	before := m.overlay.trackInfo.Render(100, 30, *m.styles)
	m = m.handleTrackInfoResult(result)
	if m.overlay.trackInfo.Render(100, 30, *m.styles) != before {
		t.Fatal("older request replaced new inspection of same track")
	}
	result.Request = 2
	result.Path = "/music/other.flac"
	m = m.handleTrackInfoResult(result)
	if m.overlay.trackInfo.Render(100, 30, *m.styles) != before {
		t.Fatal("different track replaced inspection")
	}
}

func TestTrackInfoFixtureNeverProbesFiles(t *testing.T) {
	m, err := BuildFixture(FixtureNarrow)
	if err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	if cmd != nil || m.overlay.trackInfo == nil {
		t.Fatal("fixture must open stored info without probing a file")
	}
}

func TestTrackInfoCanInspectMissingQueueFile(t *testing.T) {
	m := setupTrackInfoModel(t)
	m.queue.Entries[0].Missing = true
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	if m.overlay.trackInfo == nil || cmd == nil {
		t.Fatal("missing file prevented stored metadata from opening")
	}
	out := m.overlay.trackInfo.Render(100, 30, *m.styles)
	if !strings.Contains(out, "Test Song") || !strings.Contains(out, "/music/test.flac") {
		t.Fatal(out)
	}
}
