package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestRemoteActionHandledAsNoOp confirms that when an unknown remote action is
// dispatched through handlePlaybackKeyAction, it is reported as unhandled and
// produces no command — the desired no-op behaviour.
func TestRemoteActionHandledAsNoOp(t *testing.T) {
	m := Model{}
	_, cmd, handled := m.handlePlaybackKeyAction(playback.RemoteAction("not-a-real-action"))
	if handled {
		t.Fatalf("expected unknown remote action to be unhandled, got handled=true")
	}
	if cmd != nil {
		t.Fatalf("expected nil tea.Cmd for unhandled remote action, got non-nil")
	}
}
