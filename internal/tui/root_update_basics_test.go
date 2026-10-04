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

	if m.ActiveView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, m.ActiveView)
	}

	// 2 -> Playlists
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.ActiveView != viewPlaylists {
		t.Fatalf("expected viewPlaylists (%d), got %d", viewPlaylists, nm.ActiveView)
	}

	// 3 -> Queue
	next, _ = nm.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm = next.(Model)
	if nm.ActiveView != viewQueue {
		t.Fatalf("expected viewQueue (%d), got %d", viewQueue, nm.ActiveView)
	}

	// 1 -> Library
	next, _ = nm.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm = next.(Model)
	if nm.ActiveView != viewLibrary {
		t.Fatalf("expected viewLibrary (%d), got %d", viewLibrary, nm.ActiveView)
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
		t.Fatal("keypress should dismiss first-run config notice")
	}
	if nm.helpVisible || !nm.welcomeDismissed {
		t.Fatal("first keypress should only dismiss the welcome overlay")
	}
	nm, _ = updateModel(nm, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !nm.helpVisible {
		t.Fatal("subsequent keypress should route normally")
	}
}

func TestHandleUnhandledMsgForwardsToActiveInput(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue = m.queue.StartSaveAsPlaylist(false)

	next, _ := m.handleUnhandledMsg(tea.KeyPressMsg{Code: 'n', Text: "n"})
	nm := next.(Model)
	if got := nm.queue.SaveInput.Value(); got != "n" {
		t.Fatalf("expected forwarded message to update queue save input, got %q", got)
	}
}

// viewSwitchBudget bounds a single view-switch keypress plus its first
// rendered frame. It guards against a switch acquiring work it has no business
// doing on the Elm update goroutine — a synchronous DB query, a full-library
// render — which costs hundreds of milliseconds to seconds.
//
// It is deliberately not a tight measure of this machine's speed. Under -race
// a switch measures ~10ms on a developer laptop and ~62ms on a shared
// ubuntu-latest runner, so the original 50ms bound asserted host performance
// rather than code behaviour and failed the first time CI ran on Linux. The
// budget below clears the slowest observed runner by ~4x while still catching
// the order-of-magnitude regressions the test exists to catch.
const viewSwitchBudget = 250 * time.Millisecond

// TestViewSwitchingStaysInteractive verifies that each view-switch keypress and
// its first rendered frame complete within viewSwitchBudget, and that the
// active view is correct after each switch.
//
// Two sources of noise are excluded, because neither says anything about how
// fast a view switch is: one-time render initialisation is paid by a discarded
// warm-up frame before any timing starts, and each switch is measured several
// times and judged on its fastest run so a single scheduler preemption cannot
// decide the result.
func TestViewSwitchingStaysInteractive(t *testing.T) {
	m := New(context.Background())

	// Warm-up: pay the one-time render initialisation outside the measurement.
	_ = m.View()

	steps := []struct {
		key  tea.KeyPressMsg
		want viewID
	}{
		{tea.KeyPressMsg{Code: '2', Text: "2"}, viewPlaylists},
		{tea.KeyPressMsg{Code: '3', Text: "3"}, viewQueue},
		{tea.KeyPressMsg{Code: '6', Text: "6"}, viewTags},
		{tea.KeyPressMsg{Code: '1', Text: "1"}, viewLibrary},
	}

	const attempts = 5

	for _, tc := range steps {
		var best time.Duration
		var nm Model

		for i := range attempts {
			start := time.Now()
			next, _ := m.Update(tc.key)
			nm = next.(Model)
			_ = nm.View()
			if elapsed := time.Since(start); i == 0 || elapsed < best {
				best = elapsed
			}
		}

		if best >= viewSwitchBudget {
			t.Fatalf("view switch to %d: fastest of %d runs took %v, must complete under %v",
				tc.want, attempts, best, viewSwitchBudget)
		}
		if nm.ActiveView != tc.want {
			t.Fatalf("expected activeView %d after key press, got %d", tc.want, nm.ActiveView)
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
