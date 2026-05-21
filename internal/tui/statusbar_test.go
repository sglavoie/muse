package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestRenderPlaybackStateGlyphs verifies that each PlaybackState maps to the
// correct indicator glyph (▶ / ⏸ / ■) or no glyph when idle.
func TestRenderPlaybackStateGlyphs(t *testing.T) {
	tests := []struct {
		state     PlaybackState
		wantGlyph string
		noGlyph   bool
	}{
		{PlaybackPlaying, "▶", false},
		{PlaybackPaused, "⏸", false},
		{PlaybackStopped, "■", false},
		{PlaybackIdle, "", true},
	}

	for _, tc := range tests {
		sb := StatusBar{
			artist:        "Artist",
			title:         "Title",
			playbackState: tc.state,
		}
		out := sb.render(0, styles.Styles{})

		if tc.noGlyph {
			for _, g := range []string{"▶", "⏸", "■"} {
				if strings.Contains(out, g) {
					t.Errorf("state=%v: unexpected glyph %q in %q", tc.state, g, out)
				}
			}
		} else if !strings.Contains(out, tc.wantGlyph) {
			t.Errorf("state=%v: expected glyph %q in %q", tc.state, tc.wantGlyph, out)
		}
	}
}

// TestRenderTitleTruncationPreservesArtist verifies that when terminal width is
// insufficient the track title is truncated with "…" while the full artist name
// is preserved.
func TestRenderTitleTruncationPreservesArtist(t *testing.T) {
	sb := StatusBar{
		artist:        "Full Artist",
		title:         "A Very Long Track Title That Should Be Truncated",
		playbackState: PlaybackPlaying,
	}

	// Width just large enough for glyph + artist + separator but not the full title.
	// "▶ Full Artist – …  |  log: muse.log" should fit at around 40 chars.
	out := sb.render(45, styles.Styles{})

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

// TestRenderNoTruncationWhenSufficientWidth verifies that the full title is
// shown when width is large enough.
func TestRenderNoTruncationWhenSufficientWidth(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "Short Title",
		playbackState: PlaybackPlaying,
	}

	out := sb.render(200, styles.Styles{})

	if !strings.Contains(out, "Short Title") {
		t.Errorf("expected full title at wide width: got %q", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("unexpected ellipsis at wide width: got %q", out)
	}
}

// TestRenderNoTruncationWhenWidthZero verifies that width=0 means unconstrained
// and the full title is always shown.
func TestRenderNoTruncationWhenWidthZero(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "A Very Long Track Title",
		playbackState: PlaybackPlaying,
	}

	out := sb.render(0, styles.Styles{})

	if !strings.Contains(out, "A Very Long Track Title") {
		t.Errorf("expected full title when width=0: got %q", out)
	}
}

// TestRenderQueuePlaylistSource verifies that when playlistSource is empty the
// persistent left zone does not display a playlist name (the caller is
// responsible for setting "Queue" before calling render).
func TestRenderQueuePlaylistSource(t *testing.T) {
	sb := StatusBar{
		artist:         "Artist",
		title:          "Title",
		playbackState:  PlaybackPlaying,
		playlistSource: "Queue",
	}

	out := sb.render(0, styles.Styles{})

	if !strings.Contains(out, "Queue") {
		t.Errorf("expected 'Queue' in output: got %q", out)
	}
}

// TestRenderNamedPlaylistSource verifies that a named playlist source is
// displayed in the persistent left zone.
func TestRenderNamedPlaylistSource(t *testing.T) {
	sb := StatusBar{
		artist:         "Artist",
		title:          "Title",
		playbackState:  PlaybackPlaying,
		playlistSource: "My Playlist",
	}

	out := sb.render(0, styles.Styles{})

	if !strings.Contains(out, "My Playlist") {
		t.Errorf("expected playlist name in output: got %q", out)
	}
	if strings.Contains(out, "Queue") {
		t.Errorf("unexpected 'Queue' when named playlist is set: got %q", out)
	}
}

// TestRenderModeCluster verifies that the always-on 4-glyph RSND mode cluster
// is shown regardless of state, that verbose forms ("Repeat"/"Shuffle"/...)
// are no longer emitted, and that RepeatOne substitutes the first glyph
// (R → 1) to advertise the distinction within the fixed 4-column width.
func TestRenderModeCluster(t *testing.T) {
	base := StatusBar{
		artist:        "Artist",
		title:         "Title",
		playbackState: PlaybackPlaying,
	}

	// Both off → cluster shows "RSND" (all glyphs dim).
	out := base.render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster when modes off: got %q", out)
	}
	for _, verbose := range []string{"Repeat", "Shuffle", "Auto-DJ", "Stop Next"} {
		if strings.Contains(out, verbose) {
			t.Errorf("verbose label %q should no longer appear: got %q", verbose, out)
		}
	}

	// RepeatAll → cluster text unchanged ("RSND"); only R's style differs.
	sbR := base
	sbR.repeatMode = RepeatAll
	out = sbR.render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster with RepeatAll: got %q", out)
	}

	// RepeatOne → first glyph becomes "1" so cluster reads "1SND".
	sbR1 := base
	sbR1.repeatMode = RepeatOne
	out = sbR1.render(0, styles.Styles{})
	if !strings.Contains(out, "1SND") {
		t.Errorf("expected 1SND cluster with RepeatOne: got %q", out)
	}
	if strings.Contains(out, "RSND") {
		t.Errorf("RSND should not appear with RepeatOne (R replaced by 1): got %q", out)
	}

	// Shuffle on → cluster letters unchanged; only S's style differs.
	sbS := base
	sbS.shuffle = true
	out = sbS.render(0, styles.Styles{})
	if !strings.Contains(out, "RSND") {
		t.Errorf("expected RSND cluster with shuffle on: got %q", out)
	}
}

// TestRenderModeClusterWidthStable verifies that the 4-glyph RSND cluster has
// a stable visual width (4 columns) regardless of which modes are active —
// the entire point of always rendering all four glyphs.
func TestRenderModeClusterWidthStable(t *testing.T) {
	measure := func(sb StatusBar) int {
		out := sb.render(0, styles.Styles{})
		// Cluster is "  RSND" or "  1SND"; isolate the 4-char run after the
		// double-space lead.
		i := strings.Index(out, "  ")
		if i < 0 {
			t.Fatalf("expected '  ' lead-in for cluster: got %q", out)
		}
		// Skip the two spaces; next 4 visible columns are the cluster.
		return ansi.StringWidth(out[i+2 : i+6])
	}
	base := StatusBar{artist: "A", title: "T", playbackState: PlaybackPlaying}
	cases := []StatusBar{
		base,
		func() StatusBar { b := base; b.repeatMode = RepeatAll; return b }(),
		func() StatusBar { b := base; b.repeatMode = RepeatOne; return b }(),
		func() StatusBar { b := base; b.shuffle = true; return b }(),
		func() StatusBar { b := base; b.stopAfterCurrent = true; return b }(),
		func() StatusBar { b := base; b.radioMode = true; return b }(),
		func() StatusBar {
			b := base
			b.repeatMode, b.shuffle, b.stopAfterCurrent, b.radioMode = RepeatAll, true, true, true
			return b
		}(),
	}
	for i, sb := range cases {
		if w := measure(sb); w != 4 {
			t.Errorf("case %d: cluster width=%d, want 4", i, w)
		}
	}
}

// TestRenderTransientDoesNotOverwritePersistent verifies that when a transient
// message is present the persistent left zone (artist, title, glyph) is still
// visible in the output.
func TestRenderTransientDoesNotOverwritePersistent(t *testing.T) {
	sb := StatusBar{
		artist:        "Persistent Artist",
		title:         "Persistent Title",
		playbackState: PlaybackPlaying,
		notifications: []transientNotification{{msg: "Scanning… 100 tracks"}},
	}

	out := sb.render(0, styles.Styles{})

	if !strings.Contains(out, "Persistent Artist") {
		t.Errorf("artist hidden by transient message: got %q", out)
	}
	if !strings.Contains(out, "Persistent Title") {
		t.Errorf("title hidden by transient message: got %q", out)
	}
	if !strings.Contains(out, "▶") {
		t.Errorf("glyph hidden by transient message: got %q", out)
	}
	if !strings.Contains(out, "Scanning… 100 tracks") {
		t.Errorf("transient message missing: got %q", out)
	}
}

// TestRenderTransientVisibleWithWidthConstraint verifies that when a width is
// set and a transient message is present, persistent fields remain visible
// (title may be truncated but artist and glyph are preserved).
func TestRenderTransientVisibleWithWidthConstraint(t *testing.T) {
	sb := StatusBar{
		artist:        "My Artist",
		title:         "A Very Long Title Indeed",
		playbackState: PlaybackPlaying,
		notifications: []transientNotification{{msg: "Some notification"}},
	}

	out := sb.render(45, styles.Styles{})

	if !strings.Contains(out, "My Artist") {
		t.Errorf("artist should be preserved with width constraint: got %q", out)
	}
	if !strings.Contains(out, "▶") {
		t.Errorf("glyph should be preserved with width constraint: got %q", out)
	}
	if !strings.Contains(out, "Some notification") {
		t.Errorf("transient message should remain: got %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("title should be truncated with ellipsis under width constraint: got %q", out)
	}
}

// TestRenderIdleStateShowsNoGlyphAndNoTrackInfo verifies that when state is
// idle and no artist/title are set the persistent left zone is omitted.
func TestRenderIdleStateShowsNoGlyphAndNoTrackInfo(t *testing.T) {
	sb := StatusBar{
		playbackState: PlaybackIdle,
	}

	out := sb.render(0, styles.Styles{})

	for _, g := range []string{"▶", "⏸", "■"} {
		if strings.Contains(out, g) {
			t.Errorf("unexpected glyph %q in idle state: got %q", g, out)
		}
	}
	if strings.Contains(out, "log:") {
		t.Errorf("log path should not be shown: got %q", out)
	}
}

// TestRenderIdleEmptyQueueShowsHint verifies that when the queue is empty and
// playback is idle, the status bar shows a persistent hint to browse the library.
func TestRenderIdleEmptyQueueShowsHint(t *testing.T) {
	sb := StatusBar{
		playbackState: PlaybackIdle,
		queueEmpty:    true,
	}

	out := sb.render(0, styles.Styles{})
	if !strings.Contains(out, "Queue is empty") {
		t.Errorf("expected empty-queue hint: got %q", out)
	}
	if !strings.Contains(out, "press 1 to browse Library") {
		t.Errorf("expected library hint: got %q", out)
	}
}

// TestRenderIdleNonEmptyQueueNoHint verifies that when the queue has entries
// and playback is idle (e.g. stopped), no empty-queue hint is shown.
func TestRenderIdleNonEmptyQueueNoHint(t *testing.T) {
	sb := StatusBar{
		playbackState: PlaybackIdle,
		queueEmpty:    false,
	}

	out := sb.render(0, styles.Styles{})
	if strings.Contains(out, "Queue is empty") {
		t.Errorf("unexpected empty-queue hint when queue has entries: got %q", out)
	}
}

// TestRenderElapsedTimeDisplay verifies that when elapsed and total are set the
// status bar shows "MM:SS / MM:SS" in the output.
func TestRenderElapsedTimeDisplay(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "Title",
		playbackState: PlaybackPlaying,
		elapsed:       "01:23",
		total:         "04:56",
	}

	out := sb.render(0, styles.Styles{})

	if !strings.Contains(out, "01:23 / 04:56") {
		t.Errorf("expected elapsed/total in output: got %q", out)
	}
}

// TestRenderNoTimeDisplayWhenBothZero verifies that when elapsed and total are
// empty strings the time display is suppressed.
func TestRenderNoTimeDisplayWhenBothZero(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "Title",
		playbackState: PlaybackPlaying,
	}

	out := sb.render(0, styles.Styles{})

	// Should not contain any MM:SS pattern when no time is set.
	if strings.Contains(out, " / ") {
		t.Errorf("unexpected time display when elapsed/total empty: got %q", out)
	}
}

// TestRenderTimeDisplayOnLine2 verifies that time display is on line 2 and
// that line 1 fits within the specified width.
func TestRenderTimeDisplayOnLine2(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "A Very Long Track Title That Should Be Truncated",
		playbackState: PlaybackPlaying,
		elapsed:       "01:23",
		total:         "04:56",
		elapsedSecs:   83,
		totalSecs:     296,
	}

	out := sb.render(60, styles.Styles{})

	lines := strings.SplitN(out, "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in render output, got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[1], "01:23 / 04:56") {
		t.Errorf("time display should be on line 2: got %q", lines[1])
	}
	if !strings.Contains(lines[0], "…") {
		t.Errorf("title should be truncated on line 1: got %q", lines[0])
	}
	if ansi.StringWidth(lines[0]) > 60 {
		t.Errorf("line 1 visual width exceeds %d: width=%d, got %q", 60, ansi.StringWidth(lines[0]), lines[0])
	}
}

// TestRenderAlbumAppearsOnLine2 verifies that album name appears on line 2
// when elapsedSecs > 0.
func TestRenderAlbumAppearsOnLine2(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "Title",
		album:         "My Album",
		playbackState: PlaybackPlaying,
		elapsed:       "1:00",
		total:         "3:30",
		elapsedSecs:   60,
		totalSecs:     210,
	}

	out := sb.render(0, styles.Styles{})

	lines := strings.SplitN(out, "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in render output, got %d: %q", len(lines), out)
	}
	if strings.Contains(lines[0], "My Album") {
		t.Errorf("album should not appear on line 1: got %q", lines[0])
	}
	if !strings.Contains(lines[1], "My Album") {
		t.Errorf("album should appear on line 2: got %q", lines[1])
	}
}

// TestRenderProgressBarOnLine2 verifies that the progress bar is rendered on
// line 2 (filling the remaining width) when elapsedSecs > 0.
func TestRenderProgressBarOnLine2(t *testing.T) {
	sb := StatusBar{
		artist:        "Artist",
		title:         "Title",
		album:         "Album",
		playbackState: PlaybackPlaying,
		elapsed:       "0:30",
		total:         "1:00",
		elapsedSecs:   30,
		totalSecs:     60,
	}

	out := sb.render(40, styles.Styles{})

	lines := strings.SplitN(out, "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
	// Progress bar uses █ (filled) and ░ (empty); at least one should be present.
	if !strings.Contains(lines[1], "█") && !strings.Contains(lines[1], "░") {
		t.Errorf("line 2 should contain progress bar glyphs: got %q", lines[1])
	}
}
