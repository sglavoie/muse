package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

func TestUpdateViewSwitching(t *testing.T) {
	m := New(context.Background())

	if m.activeView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, m.activeView)
	}

	// 2 -> Playlists
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.activeView != viewPlaylists {
		t.Fatalf("expected viewPlaylists (%d), got %d", viewPlaylists, nm.activeView)
	}

	// 3 -> Queue
	next, _ = nm.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm = next.(Model)
	if nm.activeView != viewQueue {
		t.Fatalf("expected viewQueue (%d), got %d", viewQueue, nm.activeView)
	}

	// 1 -> Library
	next, _ = nm.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm = next.(Model)
	if nm.activeView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, nm.activeView)
	}
}

func TestUpdateQDoesNotQuit(t *testing.T) {
	m := New(context.Background())
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatal("q key must not quit the application")
	}
}

func TestUpdateEscDoesNotQuit(t *testing.T) {
	m := New(context.Background())
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc key must not quit the application")
	}
}

func TestUpdateKeyPressDismissesConfigCreatedNotice(t *testing.T) {
	m := New(context.Background())
	m.configCreated = true

	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	if nm.configCreated {
		t.Fatal("keypress should dismiss first-run config notice before routing")
	}
	if !nm.helpVisible {
		t.Fatal("keypress should still route normally after dismissing config notice")
	}
}

func TestHandleUnhandledMsgForwardsToActiveInput(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue = m.queue.startSaveAsPlaylist(false)

	next, _ := m.handleUnhandledMsg(tea.KeyPressMsg{Code: 'n', Text: "n"})
	nm := next.(Model)
	if got := nm.queue.saveInput.Value(); got != "n" {
		t.Fatalf("expected forwarded message to update queue save input, got %q", got)
	}
}

// TestViewSwitchingCompletesUnder50ms verifies that each view-switch keypress
// and its first rendered frame complete within 50ms, and that the active view
// is correct after each switch.
func TestViewSwitchingCompletesUnder50ms(t *testing.T) {
	m := New(context.Background())

	steps := []struct {
		key  tea.KeyPressMsg
		want viewID
	}{
		{tea.KeyPressMsg{Code: '2', Text: "2"}, viewPlaylists},
		{tea.KeyPressMsg{Code: '3', Text: "3"}, viewQueue},
		{tea.KeyPressMsg{Code: '8', Text: "8"}, viewTags},
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
	// Unbuffered channel with no sender - Update must not block on it.
	ch := make(chan library.Track)
	m := New(context.Background(), WithScanChannel(ch))

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
