package statusbar

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestRenderIdleStateShowsNoGlyphAndNoTrackInfo(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{PlaybackState: playback.StateIdle}, 0)
	for _, glyph := range []string{"▶", "⏸", "■"} {
		if strings.Contains(out, glyph) {
			t.Errorf("unexpected glyph %q in idle state: got %q", glyph, out)
		}
	}
	if strings.Contains(out, "log:") {
		t.Errorf("log path should not be shown: got %q", out)
	}
}

func TestRenderIdleQueueHints(t *testing.T) {
	if out := renderStatusBarForTest(StatusBar{PlaybackState: playback.StateIdle, QueueEmpty: true}, 0); !strings.Contains(out, "Queue is empty") || !strings.Contains(out, "press 1 to browse Library") {
		t.Errorf("expected empty-queue hint, got %q", out)
	}
	if out := renderStatusBarForTest(StatusBar{PlaybackState: playback.StateIdle, QueueEmpty: false}, 0); strings.Contains(out, "Queue is empty") {
		t.Errorf("unexpected empty-queue hint when queue has entries: got %q", out)
	}
}
