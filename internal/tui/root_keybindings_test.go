package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestViewLocalBindingTakesPrecedenceOverGlobal verifies that when the active
// view defines a local binding for a key, the view-local action fires and the
// global action for that key is suppressed.
//
// The 'r' key is bound globally to actionRepeatCycle and locally to
// actionPlaylistRename in the Playlists view. When the Playlists view is
// focused, pressing 'r' must fire rename (not toggle repeat mode).
func TestViewLocalBindingTakesPrecedenceOverGlobal(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists

	// The global 'r' binding would normally toggle repeat mode. The
	// Playlists-local binding for 'r' (rename) must intercept first.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("view-local 'r' in Playlists must return a rename cmd")
	}
	msg := cmd()
	if _, ok := msg.(PlaylistRenameMsg); !ok {
		t.Fatalf("view-local 'r' in Playlists: expected PlaylistRenameMsg, got %T", msg)
	}
	if nm.repeatMode != RepeatOff {
		t.Fatal("view-local 'r' in Playlists must not cycle global repeat mode")
	}
}

// TestGlobalBindingFiresWhenNoViewLocalBinding verifies that keys not
// overridden by the active view still trigger their global action.
//
// The Library and Queue views have no local binding for 'R', so pressing 'R'
// in those views falls through to the global actionRescan.
func TestGlobalBindingFiresWhenNoViewLocalBinding(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Queue", viewQueue},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			m := New(context.Background(), WithLibraryRoot(root), WithExtractor(stubExtractor{}))
			m.activeView = tc.viewID

			_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
			if cmd == nil {
				t.Fatalf("%s: global 'R' must return a rescan cmd when no view-local binding exists", tc.name)
			}
		})
	}
}

// TestEqualKeyFiresVolumeUp verifies that pressing '=' calls SetVolumeRelative
// with a positive delta on the player, updates the volume field, and sets a
// transient notification.
func TestEqualKeyFiresVolumeUp(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.volume = 50

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if p.volumeDelta != 5 {
		t.Fatalf("= key: expected SetVolumeRelative(5), got delta=%v", p.volumeDelta)
	}
	if nm.volume != 55 {
		t.Fatalf("= key: expected volume=55, got %d", nm.volume)
	}
	if nm.statusBar.notification.msg != "Volume: 55%" {
		t.Fatalf("= key: expected notification 'Volume: 55%%', got %q", nm.statusBar.notification.msg)
	}
}

// TestLowerRCyclesRepeatModeInNonPlaylistViews verifies that pressing 'r' in
// Library and Queue views advances repeatMode through Off→All→One→Off, and
// does NOT trigger rescan.
func TestLowerRCyclesRepeatModeInNonPlaylistViews(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Queue", viewQueue},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background())
			m.activeView = tc.viewID

			// First press: RepeatOff → RepeatAll.
			next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm := next.(Model)
			if cmd != nil {
				t.Fatalf("%s: 'r' repeat cycle must return nil cmd, got non-nil", tc.name)
			}
			if nm.repeatMode != RepeatAll {
				t.Fatalf("%s: first press: expected RepeatAll, got %v", tc.name, nm.repeatMode)
			}

			// Second press: RepeatAll → RepeatOne.
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.repeatMode != RepeatOne {
				t.Fatalf("%s: second press: expected RepeatOne, got %v", tc.name, nm.repeatMode)
			}

			// Third press: RepeatOne → RepeatOff (wraps).
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.repeatMode != RepeatOff {
				t.Fatalf("%s: third press: expected RepeatOff, got %v", tc.name, nm.repeatMode)
			}
		})
	}
}

// TestRepeatCycleCallsWriteRepeatMode verifies that actionRepeatCycle calls
// sessionWriter.WriteRepeatMode with the correct mode string on each press
// and cycles in the order "none" → "all" → "one" → "none".
func TestRepeatCycleCallsWriteRepeatMode(t *testing.T) {
	m := New(context.Background())
	sw := &stubSessionWriter{}
	m.SetSessionWriter(sw)

	wantCalls := []string{"all", "one", "none"}

	var nm Model = m
	for i, want := range wantCalls {
		next, _ := nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		nm = next.(Model)
		if len(sw.calls) != i+1 {
			t.Fatalf("press %d: expected %d WriteRepeatMode call(s), got %d", i+1, i+1, len(sw.calls))
		}
		if sw.calls[i] != want {
			t.Fatalf("press %d: expected WriteRepeatMode(%q), got %q", i+1, want, sw.calls[i])
		}
	}
}

// TestZTogglesShuffleGlobal verifies that pressing 'z' toggles the shuffle
// field on the root model.
func TestZTogglesShuffleGlobal(t *testing.T) {
	m := New(context.Background())

	// First press: shuffle off → on.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	nm := next.(Model)
	if cmd != nil {
		t.Fatal("z key: expected nil cmd for shuffle toggle")
	}
	if !nm.shuffle {
		t.Fatal("z key first press: expected shuffle=true, got false")
	}

	// Second press: shuffle on → off.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	nm = next.(Model)
	if nm.shuffle {
		t.Fatal("z key second press: expected shuffle=false, got true")
	}
}

// TestUDispatchesUndoMessage verifies that pressing 'u' returns a cmd that
// emits UndoMsg.
func TestUDispatchesUndoMessage(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("u key: expected non-nil cmd for undo dispatch")
	}
	msg := cmd()
	if _, ok := msg.(UndoMsg); !ok {
		t.Fatalf("u key: expected UndoMsg, got %T", msg)
	}
}

// TestSlashRoutesToActiveView verifies that pressing '/' returns a cmd that
// emits FilterBarMsg for routing to the active view.
func TestSlashRoutesToActiveView(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd == nil {
		t.Fatal("/ key: expected non-nil cmd for filter bar activation")
	}
	msg := cmd()
	if _, ok := msg.(FilterBarMsg); !ok {
		t.Fatalf("/ key: expected FilterBarMsg, got %T", msg)
	}
}

// TestSeekFineBackwardKey verifies that the left arrow key calls
// SeekRelative(-5) on the player (fine seek backward by 5 seconds).
func TestSeekFineBackwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New(context.Background(), WithPlayer(p))

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != -5 {
		t.Fatalf("left arrow: expected SeekRelative(-5), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekFineForwardKey verifies that the right arrow key calls
// SeekRelative(+5) on the player (fine seek forward by 5 seconds).
func TestSeekFineForwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New(context.Background(), WithPlayer(p))

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != 5 {
		t.Fatalf("right arrow: expected SeekRelative(5), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekCoarseBackwardKey verifies that Shift+Left calls SeekRelative(-30)
// on the player (coarse seek backward by 30 seconds).
func TestSeekCoarseBackwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New(context.Background(), WithPlayer(p))

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != -30 {
		t.Fatalf("shift+left: expected SeekRelative(-30), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekCoarseForwardKey verifies that Shift+Right calls SeekRelative(+30)
// on the player (coarse seek forward by 30 seconds).
func TestSeekCoarseForwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New(context.Background(), WithPlayer(p))

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != 30 {
		t.Fatalf("shift+right: expected SeekRelative(30), got deltas=%v", p.seekDeltas)
	}
}

// TestNextTrackKey verifies that pressing 'n' calls Next() on the player.
func TestNextTrackKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New(context.Background(), WithPlayer(p))

	m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})

	if p.nextCalls != 1 {
		t.Fatalf("n key: expected 1 Next() call, got %d", p.nextCalls)
	}
}

// TestPreviousTrackKey verifies that pressing 'p' calls Previous() on the player.
func TestPreviousTrackKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New(context.Background(), WithPlayer(p))

	m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})

	if p.prevCalls != 1 {
		t.Fatalf("p key: expected 1 Previous() call, got %d", p.prevCalls)
	}
}

// TestMinusKeyFiresVolumeDown verifies that pressing '-' calls
// SetVolumeRelative(-5) on the player, updates the volume field, and sets a
// transient notification.
func TestMinusKeyFiresVolumeDown(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.volume = 50

	next, _ := m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	nm := next.(Model)

	if p.volumeDelta != -5 {
		t.Fatalf("- key: expected SetVolumeRelative(-5), got delta=%v", p.volumeDelta)
	}
	if nm.volume != 45 {
		t.Fatalf("- key: expected volume=45, got %d", nm.volume)
	}
	if nm.statusBar.notification.msg != "Volume: 45%" {
		t.Fatalf("- key: expected notification 'Volume: 45%%', got %q", nm.statusBar.notification.msg)
	}
}

// TestVolumeUpClampsAt100 verifies that volume up from 95 clamps to 100.
func TestVolumeUpClampsAt100(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.volume = 95

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if nm.volume != 100 {
		t.Fatalf("volume up from 95: expected 100, got %d", nm.volume)
	}
	if nm.statusBar.notification.msg != "Volume: 100%" {
		t.Fatalf("expected notification 'Volume: 100%%', got %q", nm.statusBar.notification.msg)
	}
}

// TestVolumeUpStaysAt100 verifies that volume up from 100 stays at 100.
func TestVolumeUpStaysAt100(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.volume = 100

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if nm.volume != 100 {
		t.Fatalf("volume up from 100: expected 100, got %d", nm.volume)
	}
}

// TestVolumeDownClampsAt0 verifies that volume down from 5 clamps to 0.
func TestVolumeDownClampsAt0(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.volume = 5

	next, _ := m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	nm := next.(Model)

	if nm.volume != 0 {
		t.Fatalf("volume down from 5: expected 0, got %d", nm.volume)
	}
	if nm.statusBar.notification.msg != "Volume: 0%" {
		t.Fatalf("expected notification 'Volume: 0%%', got %q", nm.statusBar.notification.msg)
	}
}

// TestVolumeNotificationUsesShorterExpiry verifies that volume notifications
// use volumeFeedbackDuration (2s), not the default transientDuration (5s).
func TestVolumeNotificationUsesShorterExpiry(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.volume = 50

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	expiry := nm.statusBar.notification.expiresAt
	// The expiry should be roughly 2s from now, not 5s. Allow 1s tolerance.
	if d := time.Until(expiry); d > 3*time.Second || d < 1*time.Second {
		t.Fatalf("volume notification expiry: expected ~2s from now, got %v", d)
	}
}

// TestHKeyIsUnbound verifies that 'h' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestHKeyIsUnbound(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if cmd != nil {
		t.Fatal("'h' key must be unbound: expected nil cmd")
	}
}

// TestLKeyIsUnbound verifies that 'l' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestLKeyIsUnbound(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if cmd != nil {
		t.Fatal("'l' key must be unbound: expected nil cmd")
	}
}

// TestGlobalRepeatBindingFiresInViewsWithoutLocalOverride verifies that
// pressing lowercase 'r' in Library and Queue views — which have no view-local
// 'r' binding — cycles repeatMode from RepeatOff to RepeatAll via the global
// actionRepeatCycle handler. The Playlists view is excluded because it defines
// a local 'r' binding (rename) that shadows the global action.
func TestGlobalRepeatBindingFiresInViewsWithoutLocalOverride(t *testing.T) {
	views := []struct {
		name   string
		viewID viewID
	}{
		{"Library", viewLibrary},
		{"Queue", viewQueue},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background())
			m.activeView = tc.viewID

			if m.repeatMode != RepeatOff {
				t.Fatalf("%s: initial repeatMode must be RepeatOff, got %v", tc.name, m.repeatMode)
			}

			next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm := next.(Model)

			if nm.repeatMode != RepeatAll {
				t.Fatalf("%s: pressing 'r' must cycle repeatMode from RepeatOff to RepeatAll, got %v", tc.name, nm.repeatMode)
			}
		})
	}
}
