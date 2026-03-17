package tui

import (
	"image/color"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

func TestUpdateViewSwitching(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	if m.activeView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, m.activeView)
	}

	// 2 → Playlists
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.activeView != viewPlaylists {
		t.Fatalf("expected viewPlaylists (%d), got %d", viewPlaylists, nm.activeView)
	}

	// 3 → Queue
	next, _ = nm.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm = next.(Model)
	if nm.activeView != viewQueue {
		t.Fatalf("expected viewQueue (%d), got %d", viewQueue, nm.activeView)
	}

	// 1 → Library
	next, _ = nm.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm = next.(Model)
	if nm.activeView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, nm.activeView)
	}
}

func TestUpdateCtrlCQuits(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c: expected a quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c: expected tea.QuitMsg, got %T", msg)
	}
}

func TestUpdateQDoesNotQuit(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatal("q key must not quit the application")
	}
}

func TestUpdateEscDoesNotQuit(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc key must not quit the application")
	}
}

func TestViewReturnsTeaView(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	v := m.View()
	// Compile-time check: View() returns tea.View (not string).
	_ = v.Content
}

// TestModelRepeatModeDefaultOff verifies that repeat defaults to RepeatOff and
// shuffle defaults to false on a freshly created model.
func TestModelRepeatModeDefaultOff(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	if m.repeatMode != RepeatOff {
		t.Fatalf("repeatMode must default to RepeatOff, got %v", m.repeatMode)
	}
	if m.shuffle {
		t.Fatal("shuffle must default to false")
	}
}

// TestViewSwitchingCompletesUnder50ms verifies that each view-switch keypress
// and its first rendered frame complete within 50ms, and that the active view
// is correct after each switch.
func TestViewSwitchingCompletesUnder50ms(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	steps := []struct {
		key  tea.KeyPressMsg
		want viewID
	}{
		{tea.KeyPressMsg{Code: '2', Text: "2"}, viewPlaylists},
		{tea.KeyPressMsg{Code: '3', Text: "3"}, viewQueue},
		{tea.KeyPressMsg{Code: '1', Text: "1"}, viewLibrary},
	}

	for _, tc := range steps {
		start := time.Now()
		next, _ := m.Update(tc.key)
		nm := next.(Model)
		_ = nm.View()
		elapsed := time.Since(start)

		if elapsed >= 50*time.Millisecond {
			t.Fatalf("view switch to %d: took %v, must complete under 50ms", tc.want, elapsed)
		}
		if nm.activeView != tc.want {
			t.Fatalf("expected activeView %d after key press, got %d", tc.want, nm.activeView)
		}
		m = nm
	}
}

// TestBackgroundBatchUpdateLoopNonBlocking verifies that the Update loop
// returns immediately when handling a ScanBatchMsg. The blocking channel read
// for the next batch is deferred to the returned Cmd, keeping the Elm update
// loop free to process further messages.
func TestBackgroundBatchUpdateLoopNonBlocking(t *testing.T) {
	// Unbuffered channel with no sender — Update must not block on it.
	ch := make(chan library.Track)
	m := New("", false, ch, nil, "", nil, nil)

	start := time.Now()
	_, cmd := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "A", Title: "T"},
	}})
	elapsed := time.Since(start)

	if elapsed >= 50*time.Millisecond {
		t.Fatalf("Update with ScanBatchMsg blocked for %v; must return immediately", elapsed)
	}
	if cmd == nil {
		t.Fatal("ScanBatchMsg must return a follow-up cmd for the next batch read")
	}
}

// TestRapidWindowSizeMsgDoesNotPanic verifies that sending many successive
// WindowSizeMsg events in a tight loop does not cause a panic and always
// returns a valid model.
func TestRapidWindowSizeMsgDoesNotPanic(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	sizes := [][2]int{
		{80, 24}, {120, 40}, {40, 10}, {200, 50}, {60, 20},
		{80, 24}, {100, 30}, {50, 15}, {180, 45}, {70, 22},
	}
	for range 5 {
		for _, s := range sizes {
			next, _ := m.Update(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
			m = next.(Model)
		}
	}

	// If we reach here, no panic occurred. Confirm final termWidth is the last sent value.
	lastWidth := sizes[len(sizes)-1][0]
	if m.termWidth != lastWidth {
		t.Fatalf("after rapid resize: expected termWidth=%d, got %d", lastWidth, m.termWidth)
	}
}

// TestWindowSizeMsgPreservesApplicationState verifies that a terminal resize
// does not alter cursor position, playback state, or scan state.
func TestWindowSizeMsgPreservesApplicationState(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
		{Path: "/music/c.flac", Display: "C"},
	}
	m.queue.cursor = 2
	m.playbackState = PlaybackPlaying
	m.scanning = true
	m.activeView = viewQueue

	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	nm := next.(Model)

	if nm.queue.cursor != 2 {
		t.Fatalf("resize: queue cursor changed; expected 2, got %d", nm.queue.cursor)
	}
	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("resize: playbackState changed; expected PlaybackPlaying, got %v", nm.playbackState)
	}
	if !nm.scanning {
		t.Fatal("resize: scanning changed; expected true, got false")
	}
	if nm.activeView != viewQueue {
		t.Fatalf("resize: activeView changed; expected viewQueue (%d), got %d", viewQueue, nm.activeView)
	}
}

// TestWindowSizeMsgUpdatesTermWidthAndRendersContent verifies that after a
// WindowSizeMsg the model stores the new terminal width and View() produces
// non-empty rendered content.
func TestWindowSizeMsgUpdatesTermWidthAndRendersContent(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	nm := next.(Model)

	if nm.termWidth != 120 {
		t.Fatalf("termWidth: expected 120, got %d", nm.termWidth)
	}

	v := nm.View()
	if v.Content == "" {
		t.Fatal("View() after WindowSizeMsg must produce non-empty content")
	}
}

// TestWindowSizeMsgPropagatesWidthToChildViews verifies that a WindowSizeMsg
// updates the width fields of both the library and playlists child views, and
// that column widths computed from the updated width are proportionally larger
// than those computed from a smaller width.
func TestWindowSizeMsgPropagatesWidthToChildViews(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	const newWidth = 160
	next, _ := m.Update(tea.WindowSizeMsg{Width: newWidth, Height: 40})
	nm := next.(Model)

	if nm.library.width != newWidth {
		t.Fatalf("library.width: expected %d, got %d", newWidth, nm.library.width)
	}
	if nm.playlists.width != newWidth {
		t.Fatalf("playlists.width: expected %d, got %d", newWidth, nm.playlists.width)
	}

	wide := libtui.Compute(120)
	narrow := libtui.Compute(80)

	if wide.Artist <= narrow.Artist {
		t.Fatalf("Artist column: expected Compute(120).Artist (%d) > Compute(80).Artist (%d)", wide.Artist, narrow.Artist)
	}
	if wide.Title <= narrow.Title {
		t.Fatalf("Title column: expected Compute(120).Title (%d) > Compute(80).Title (%d)", wide.Title, narrow.Title)
	}
}

// TestCtrlCQuitsWhileFilterBarActive verifies that ctrl+c dispatches tea.Quit
// even when the library filter bar is active and intercepting all keypresses.
func TestCtrlCQuitsWhileFilterBarActive(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.activeView = viewLibrary
	m.library.filterActive = true
	m.library.filterText = "test"

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c while filter bar active: expected non-nil Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while filter bar active: expected QuitMsg, got %T", msg)
	}
}

// TestCtrlCQuitsWhilePlaylistCreateActive verifies that ctrl+c dispatches
// tea.Quit even when the inline playlist create input is active.
func TestCtrlCQuitsWhilePlaylistCreateActive(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists

	// Activate create mode.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if !m.playlists.isCreating() {
		t.Fatal("precondition: expected isCreating=true")
	}

	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c while create input active: expected non-nil Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while create input active: expected QuitMsg, got %T", msg)
	}
}

// TestCtrlCQuitsWhilePlaylistRenameActive verifies that ctrl+c dispatches
// tea.Quit even when the inline playlist rename input is active.
func TestCtrlCQuitsWhilePlaylistRenameActive(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.activeView = viewPlaylists
	m.SetPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Test"},
	})

	// Activate rename mode via PlaylistRenameMsg.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if !m.playlists.isRenaming() {
		t.Fatal("precondition: expected isRenaming=true")
	}

	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c while rename input active: expected non-nil Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while rename input active: expected QuitMsg, got %T", msg)
	}
}

func TestUpdateBackgroundColorMsgLightBackground(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	// White = light background → IsDark() == false.
	lightMsg := tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}}
	next, _ := m.Update(lightMsg)
	nm := next.(Model)

	if nm.styles.HasDark != false {
		t.Fatalf("expected HasDark=false after light BackgroundColorMsg, got %v", nm.styles.HasDark)
	}
}

func TestUpdateBackgroundColorMsgDarkBackground(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	// Black = dark background → IsDark() == true.
	darkMsg := tea.BackgroundColorMsg{Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}}
	next, _ := m.Update(darkMsg)
	nm := next.(Model)

	if nm.styles.HasDark != true {
		t.Fatalf("expected HasDark=true after dark BackgroundColorMsg, got %v", nm.styles.HasDark)
	}
}

// TestBackgroundColorMsgReconstructsStyles verifies that each
// tea.BackgroundColorMsg reconstructs m.styles, replacing the previous value.
// Sending dark then light must leave HasDark == false.
func TestBackgroundColorMsgReconstructsStyles(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	// First set dark background.
	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}})
	m = next.(Model)

	// Now send light background — styles must be reconstructed with HasDark == false.
	next, _ = m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})
	nm := next.(Model)

	if nm.styles.HasDark != false {
		t.Fatalf("expected m.styles.HasDark == false after light BackgroundColorMsg, got %v", nm.styles.HasDark)
	}
}
