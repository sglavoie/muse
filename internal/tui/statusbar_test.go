package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestRenderPlaybackStateGlyphs verifies that each PlaybackState maps to the
// correct indicator glyph (▶ / ⏸ / ■) or no glyph when idle.
func TestRenderPlaybackStateGlyphs(t *testing.T) {
	cases := []struct {
		state     playback.State
		wantGlyph string
		noGlyph   bool
	}{
		{playback.StatePlaying, "▶", false},
		{playback.StatePaused, "⏸", false},
		{playback.StateStopped, "■", false},
		{playback.StateIdle, "", true},
	}
	for _, tc := range cases {
		out := renderStatusBarForTest(StatusBar{artist: "Artist", title: "Title", playbackState: tc.state}, 0)
		if tc.noGlyph {
			for _, glyph := range []string{"▶", "⏸", "■"} {
				if strings.Contains(out, glyph) {
					t.Errorf("state=%v: unexpected glyph %q in %q", tc.state, glyph, out)
				}
			}
			continue
		}
		if !strings.Contains(out, tc.wantGlyph) {
			t.Errorf("state=%v: expected glyph %q in %q", tc.state, tc.wantGlyph, out)
		}
	}
}

func TestRenderTitleTruncationPreservesArtist(t *testing.T) {
	sb := StatusBar{artist: "Full Artist", title: "A Very Long Track Title That Should Be Truncated", playbackState: playback.StatePlaying}
	out := renderStatusBarForTest(sb, 45)
	if !strings.Contains(out, "Full Artist") {
		t.Errorf("artist should be preserved: got %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected ellipsis after truncation: got %q", out)
	}
	if strings.Contains(out, "A Very Long Track Title That Should Be Truncated") {
		t.Errorf("full title should have been truncated: got %q", out)
	}
}

func TestRenderNoTruncationWhenSufficientWidth(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{artist: "Artist", title: "Short Title", playbackState: playback.StatePlaying}, 200)
	if !strings.Contains(out, "Short Title") {
		t.Errorf("expected full title at wide width: got %q", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("unexpected ellipsis at wide width: got %q", out)
	}
}

func TestRenderNoTruncationWhenWidthZero(t *testing.T) {
	out := renderStatusBarForTest(StatusBar{artist: "Artist", title: "A Very Long Track Title", playbackState: playback.StatePlaying}, 0)
	if !strings.Contains(out, "A Very Long Track Title") {
		t.Errorf("expected full title when width=0: got %q", out)
	}
}

func TestRenderPlaylistSource(t *testing.T) {
	cases := []struct {
		source     string
		wantAbsent string
	}{
		{source: "Queue"},
		{source: "My Playlist", wantAbsent: "Queue"},
	}

	for _, tc := range cases {
		out := renderStatusBarForTest(StatusBar{artist: "Artist", title: "Title", playbackState: playback.StatePlaying, playlistSource: tc.source}, 0)
		if !strings.Contains(out, tc.source) {
			t.Errorf("expected %q in output: got %q", tc.source, out)
		}
		if tc.wantAbsent != "" && strings.Contains(out, tc.wantAbsent) {
			t.Errorf("did not expect %q in output: got %q", tc.wantAbsent, out)
		}
	}
}
