package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
	if !nm.playback.shuffle {
		t.Fatal("z key first press: expected shuffle=true, got false")
	}

	// Second press: shuffle on → off.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	nm = next.(Model)
	if nm.playback.shuffle {
		t.Fatal("z key second press: expected shuffle=false, got true")
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
