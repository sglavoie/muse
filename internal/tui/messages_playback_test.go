package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestRemoteActionKnownNames verifies that every documented remote action name
// maps to its expected action constant.
func TestRemoteActionKnownNames(t *testing.T) {
	cases := map[string]keys.Action{
		"toggle-play": keys.ActionPlayPause,
		"next":        keys.ActionNext,
		"previous":    keys.ActionPrevious,
		"volume-up":   keys.ActionVolumeUp,
		"volume-down": keys.ActionVolumeDown,
	}
	for name, want := range cases {
		if got := remoteAction(name); got != want {
			t.Errorf("remoteAction(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestRemoteActionUnknownNameReturnsNone ensures an unknown remote action name
// does NOT silently fall back to actionPlayPause. The old default could turn
// any typo (or a future protocol drift between the remote client and the
// running instance) into an unintended play/pause toggle.
func TestRemoteActionUnknownNameReturnsNone(t *testing.T) {
	unknowns := []string{
		"",
		"play",                     // close to but not "toggle-play"
		"pause",                    // not a defined remote verb
		"TOGGLE-PLAY",              // case-sensitive intentionally
		"volume_up",                // wrong separator
		"future-feature-not-known", // protocol drift
	}
	for _, name := range unknowns {
		if got := remoteAction(name); got != keys.ActionNone {
			t.Errorf("remoteAction(%q) = %v, want actionNone (no silent play/pause fallback)", name, got)
		}
	}
}

// TestRemoteActionHandledAsNoOp confirms that when an unknown remote action is
// dispatched through handlePlaybackKeyAction, it is reported as unhandled and
// produces no command — the desired no-op behaviour.
func TestRemoteActionHandledAsNoOp(t *testing.T) {
	m := Model{}
	_, cmd, handled := m.handlePlaybackKeyAction(remoteAction("not-a-real-action"))
	if handled {
		t.Fatalf("expected unknown remote action to be unhandled, got handled=true")
	}
	if cmd != nil {
		t.Fatalf("expected nil tea.Cmd for unhandled remote action, got non-nil")
	}
}
