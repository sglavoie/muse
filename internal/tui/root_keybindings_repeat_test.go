package tui

import (
	"context"
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
	if nm.playback.repeatMode != RepeatOff {
		t.Fatal("view-local 'r' in Playlists must not cycle global repeat mode")
	}
}

// TestGlobalBindingFiresWhenNoViewLocalBinding verifies that keys not
// overridden by the active view still trigger their global action.
//
// The Library and Queue views have no local binding for 'ctrl+r', so pressing
// ctrl+r in those views falls through to the global actionRescan.
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
			m := New(context.Background(), WithLibraryRoots([]string{root}), WithExtractor(stubExtractor{}))
			m.activeView = tc.viewID

			_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if cmd == nil {
				t.Fatalf("%s: global 'ctrl+r' must return a rescan cmd when no view-local binding exists", tc.name)
			}
		})
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
			if nm.playback.repeatMode != RepeatAll {
				t.Fatalf("%s: first press: expected RepeatAll, got %v", tc.name, nm.playback.repeatMode)
			}

			// Second press: RepeatAll → RepeatOne.
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.playback.repeatMode != RepeatOne {
				t.Fatalf("%s: second press: expected RepeatOne, got %v", tc.name, nm.playback.repeatMode)
			}

			// Third press: RepeatOne → RepeatOff (wraps).
			next, _ = nm.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm = next.(Model)
			if nm.playback.repeatMode != RepeatOff {
				t.Fatalf("%s: third press: expected RepeatOff, got %v", tc.name, nm.playback.repeatMode)
			}
		})
	}
}

// TestRepeatCycleCallsWriteRepeatMode verifies that actionRepeatCycle calls
// sessionWriter.WriteRepeatMode with the correct mode string on each press
// and cycles in the order "none" → "all" → "one" → "none".
func TestRepeatCycleCallsWriteRepeatMode(t *testing.T) {
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithSessionWriter(sw))

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

			if m.playback.repeatMode != RepeatOff {
				t.Fatalf("%s: initial repeatMode must be RepeatOff, got %v", tc.name, m.playback.repeatMode)
			}

			next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			nm := next.(Model)

			if nm.playback.repeatMode != RepeatAll {
				t.Fatalf("%s: pressing 'r' must cycle repeatMode from RepeatOff to RepeatAll, got %v", tc.name, nm.playback.repeatMode)
			}
		})
	}
}
