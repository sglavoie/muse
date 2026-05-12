package tui

import (
	"math"
	"strings"
	"testing"
	"time"

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

// TestRenderRepeatAndShuffleIndicators verifies that R and S indicators appear
// when the corresponding modes are active, and are absent otherwise.
func TestRenderRepeatAndShuffleIndicators(t *testing.T) {
	base := StatusBar{
		artist:        "Artist",
		title:         "Title",
		playbackState: PlaybackPlaying,
	}

	// Both off.
	out := base.render(0, styles.Styles{})
	if strings.Contains(out, "  R") || strings.Contains(out, "  S") {
		t.Errorf("expected no R/S when both off: got %q", out)
	}

	// Repeat all (RepeatAll shows "Repeat").
	sbR := base
	sbR.repeatMode = RepeatAll
	out = sbR.render(0, styles.Styles{})
	if !strings.Contains(out, "  Repeat") {
		t.Errorf("expected Repeat indicator: got %q", out)
	}
	if strings.Contains(out, "  Shuffle") {
		t.Errorf("unexpected Shuffle indicator: got %q", out)
	}

	// Repeat one (RepeatOne shows "Repeat 1").
	sbR1 := base
	sbR1.repeatMode = RepeatOne
	out = sbR1.render(0, styles.Styles{})
	if !strings.Contains(out, "  Repeat 1") {
		t.Errorf("expected Repeat 1 indicator: got %q", out)
	}
	if strings.Contains(out, "  Shuffle") {
		t.Errorf("unexpected Shuffle indicator: got %q", out)
	}

	// Shuffle on.
	sbS := base
	sbS.shuffle = true
	out = sbS.render(0, styles.Styles{})
	if !strings.Contains(out, "  Shuffle") {
		t.Errorf("expected Shuffle indicator: got %q", out)
	}
	if strings.Contains(out, "  Repeat") {
		t.Errorf("unexpected Repeat indicator: got %q", out)
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

// TestFormatTime verifies the MM:SS formatting of formatTime.
func TestFormatTime(t *testing.T) {
	tests := []struct {
		secs float64
		want string
	}{
		{0, "0:00"},
		{59, "0:59"},
		{60, "1:00"},
		{61.9, "1:01"},
		{3600, "1:00:00"},
		{3723.5, "1:02:03"},
		{math.NaN(), "0:00"},
		{math.Inf(1), "0:00"},
		{math.Inf(-1), "0:00"},
	}
	for _, tc := range tests {
		got := formatTime(tc.secs)
		if got != tc.want {
			t.Errorf("formatTime(%v) = %q, want %q", tc.secs, got, tc.want)
		}
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

// TestTruncateTitle covers the truncateTitle helper directly.
func TestTruncateTitle(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		// CJK characters are 2 columns each; maxWidth is now in columns.
		{"日本語タイトル", 14, "日本語タイトル"}, // exact fit (7×2 = 14)
		{"日本語タイトル", 7, "日本語…"},     // budget 6 → 3 CJK chars (6) + "…" (1)
		{"日本語タイトル", 4, "日…"},       // budget 3 → 1 CJK char (2) + "…" (1)
	}

	for _, tc := range tests {
		got := truncateTitle(tc.input, tc.max)
		if got != tc.expected {
			t.Errorf("truncateTitle(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.expected)
		}
	}
}

// TestNotifyUsesConfiguredDuration verifies that StatusBar.notify respects the
// configured transientDur instead of hardcoding 5 seconds.
func TestNotifyUsesConfiguredDuration(t *testing.T) {
	sb := StatusBar{transientDur: 15 * time.Second}
	before := time.Now()
	sb.notify("test message")

	top := sb.topNotification()
	if top.msg != "test message" {
		t.Errorf("expected msg %q, got %q", "test message", top.msg)
	}
	// Expiry should be ~15s from now, not ~5s.
	diff := top.expiresAt.Sub(before)
	if diff < 14*time.Second || diff > 16*time.Second {
		t.Errorf("expected expiry ~15s from now, got %v", diff)
	}
}

// TestNotifyFallsBackToDefault verifies that StatusBar.notify falls back to the
// 5-second default constant when transientDur is zero (unset).
func TestNotifyFallsBackToDefault(t *testing.T) {
	sb := StatusBar{} // transientDur is zero
	before := time.Now()
	sb.notify("fallback test")

	top := sb.topNotification()
	diff := top.expiresAt.Sub(before)
	if diff < 4*time.Second || diff > 6*time.Second {
		t.Errorf("expected expiry ~5s (fallback) from now, got %v", diff)
	}
}
