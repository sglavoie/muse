package tui

import (
	"testing"

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
	m := New("", false, nil, nil, "", nil, nil)
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
			m := New("", false, nil, nil, root, stubExtractor{}, nil)
			m.activeView = tc.viewID

			_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
			if cmd == nil {
				t.Fatalf("%s: global 'R' must return a rescan cmd when no view-local binding exists", tc.name)
			}
		})
	}
}

// TestEqualKeyFiresVolumeUp verifies that pressing '=' calls SetVolumeRelative
// with a positive delta on the player.
func TestEqualKeyFiresVolumeUp(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: '=', Text: "="})

	if p.volumeDelta != 5 {
		t.Fatalf("= key: expected SetVolumeRelative(5), got delta=%v", p.volumeDelta)
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
			m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, "", nil, nil)
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
	m := New("", false, nil, nil, "", nil, nil)

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
	m := New("", false, nil, nil, "", nil, nil)

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
	m := New("", false, nil, nil, "", nil, nil)

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
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != -5 {
		t.Fatalf("left arrow: expected SeekRelative(-5), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekFineForwardKey verifies that the right arrow key calls
// SeekRelative(+5) on the player (fine seek forward by 5 seconds).
func TestSeekFineForwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != 5 {
		t.Fatalf("right arrow: expected SeekRelative(5), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekCoarseBackwardKey verifies that Shift+Left calls SeekRelative(-30)
// on the player (coarse seek backward by 30 seconds).
func TestSeekCoarseBackwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != -30 {
		t.Fatalf("shift+left: expected SeekRelative(-30), got deltas=%v", p.seekDeltas)
	}
}

// TestSeekCoarseForwardKey verifies that Shift+Right calls SeekRelative(+30)
// on the player (coarse seek forward by 30 seconds).
func TestSeekCoarseForwardKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})

	if len(p.seekDeltas) != 1 || p.seekDeltas[0] != 30 {
		t.Fatalf("shift+right: expected SeekRelative(30), got deltas=%v", p.seekDeltas)
	}
}

// TestNextTrackKey verifies that pressing 'n' calls Next() on the player.
func TestNextTrackKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})

	if p.nextCalls != 1 {
		t.Fatalf("n key: expected 1 Next() call, got %d", p.nextCalls)
	}
}

// TestPreviousTrackKey verifies that pressing 'p' calls Previous() on the player.
func TestPreviousTrackKey(t *testing.T) {
	p := &stubSeekPlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})

	if p.prevCalls != 1 {
		t.Fatalf("p key: expected 1 Previous() call, got %d", p.prevCalls)
	}
}

// TestMinusKeyFiresVolumeDown verifies that pressing '-' calls
// SetVolumeRelative(-5) on the player (volume down by 5%).
func TestMinusKeyFiresVolumeDown(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New("", false, nil, nil, "", nil, p)

	m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})

	if p.volumeDelta != -5 {
		t.Fatalf("- key: expected SetVolumeRelative(-5), got delta=%v", p.volumeDelta)
	}
}

// TestHKeyIsUnbound verifies that 'h' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestHKeyIsUnbound(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if cmd != nil {
		t.Fatal("'h' key must be unbound: expected nil cmd")
	}
}

// TestLKeyIsUnbound verifies that 'l' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestLKeyIsUnbound(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)

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
			m := New("", false, nil, nil, "", nil, nil)
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
