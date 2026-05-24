package tui

import (
	"charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"
)

// TestQueueRenderEmptyShowsPlaceholder verifies the empty-queue card surfaces
// the per-view CTAs (key + label rows).
func TestQueueRenderEmptyShowsPlaceholder(t *testing.T) {
	v := QueueView{}
	got := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if !strings.Contains(got, "Queue is empty") {
		t.Fatalf("empty queue must show title; got: %q", got)
	}
	for _, want := range []string{"go to library", "random album", "help"} {
		if !strings.Contains(got, want) {
			t.Errorf("empty queue card missing CTA %q; got: %q", want, got)
		}
	}
}

// TestQueueRenderFourColumns checks that all four columns are present in each
// rendered row.
func TestQueueRenderFourColumns(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Song One", Artist: "Band A", DurationMs: 200_000, Position: 1000},
		{Title: "Song Two", Artist: "Band B", DurationMs: 65_000, Position: 2000},
	})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)

	if !strings.Contains(out, "1") {
		t.Error("position column must contain entry index 1")
	}
	if !strings.Contains(out, "2") {
		t.Error("position column must contain entry index 2")
	}
	if !strings.Contains(out, "Song One") {
		t.Error("title column must contain 'Song One'")
	}
	if !strings.Contains(out, "Song Two") {
		t.Error("title column must contain 'Song Two'")
	}
	if !strings.Contains(out, "Band A") {
		t.Error("artist column must contain 'Band A'")
	}
	if !strings.Contains(out, "Band B") {
		t.Error("artist column must contain 'Band B'")
	}
	if !strings.Contains(out, "3:20") {
		t.Error("duration column must contain '3:20' for 200 000 ms")
	}
	if !strings.Contains(out, "1:05") {
		t.Error("duration column must contain '1:05' for 65 000 ms")
	}
}

// TestQueueRenderPositionIgnoresTrackNumber guards against a regression where
// the position column displayed the file's TrackNumber metadata instead of the
// row's queue index. The bug surfaced for DJ/Radio-enqueued tracks whose audio
// tags carried a numeric track number — e.g. queue position 329 rendered as
// "11" because the underlying file was track 11 on its album.
func TestQueueRenderPositionIgnoresTrackNumber(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Cassation", Artist: "Mozart", DurationMs: 60_000, Position: 1000, TrackNumber: 11},
		{Title: "Bittersweet", Artist: "Blank", DurationMs: 60_000, Position: 2000, TrackNumber: 3},
	})
	out := ansi.Strip(v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil))
	cases := []struct {
		title    string
		wantPos  int
		wrongPos int
	}{
		{"Cassation", 1, 11},
		{"Bittersweet", 2, 3},
	}
	for _, tc := range cases {
		want := fmt.Sprintf("%4d %s", tc.wantPos, tc.title)
		if !strings.Contains(out, want) {
			t.Errorf("queue row for %q must show queue index %d before the title; missing substring %q", tc.title, tc.wantPos, want)
		}
		wrong := fmt.Sprintf("%4d %s", tc.wrongPos, tc.title)
		if strings.Contains(out, wrong) {
			t.Errorf("queue row for %q must not show album TrackNumber %d before the title; found substring %q", tc.title, tc.wrongPos, wrong)
		}
	}
}

// TestQueueRenderAscendingOrder verifies that entries appear in the order
// supplied to QueueView (callers are responsible for ascending position order;
// render preserves slice order).
func TestQueueRenderAscendingOrder(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "First", Artist: "A", DurationMs: 60_000, Position: 1000},
		{Title: "Second", Artist: "B", DurationMs: 120_000, Position: 2000},
		{Title: "Third", Artist: "C", DurationMs: 180_000, Position: 3000},
	})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	firstIdx := strings.Index(out, "First")
	secondIdx := strings.Index(out, "Second")
	thirdIdx := strings.Index(out, "Third")
	if firstIdx < 0 || secondIdx < 0 || thirdIdx < 0 {
		t.Fatal("all three titles must appear in the output")
	}
	if !(firstIdx < secondIdx && secondIdx < thirdIdx) {
		t.Errorf("entries must appear in ascending order: First(%d) Second(%d) Third(%d)",
			firstIdx, secondIdx, thirdIdx)
	}
}

// TestQueueRenderMissingEntryDim verifies that a missing entry is rendered with
// dim styling and shows the "[missing]" label.
func TestQueueRenderMissingEntryDim(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Gone", Artist: "Nobody", DurationMs: 0, Missing: true, Position: 1000},
	})
	s := styles.NewStyles(true)
	out := v.render(s, -1, PlaybackIdle, nil, nil, nil)
	if !strings.Contains(out, "[missing]") {
		t.Error("missing entry must contain '[missing]' label")
	}
	dimSample := styledText(s.Dim, "x")
	if ansi.Strip(dimSample) == dimSample {
		return
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(line), "[missing]") {
			if line == ansi.Strip(line) {
				t.Error("missing entry must be rendered with dim ANSI styling")
			}
			return
		}
	}
	t.Error("missing entry '[missing]' label not found in any output line")
}

// TestQueueRenderSelectedNonFocusedRowUsesAccent verifies that a selected entry
// that is not the focused row renders its selection marker with Accent styling.
func TestQueueRenderSelectedNonFocusedRowUsesAccent(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Focused", Artist: "A", DurationMs: 60_000, Selected: false, Position: 1000},
		{Title: "Selected", Artist: "B", DurationMs: 60_000, Selected: true, Position: 2000},
	})
	v.cursor = 0
	s := styles.NewStyles(true)
	out := v.render(s, -1, PlaybackIdle, nil, nil, nil)

	want := styledText(s.AccentMark.Background(s.SelectionBg), "●")
	if ansi.Strip(want) == want {
		return
	}
	if !strings.Contains(out, want) {
		t.Errorf("selected non-focused row must contain accent-styled marker %q; got:\n%s", want, out)
	}
}

// TestQueueRenderProportionalColumnWidths verifies that the title and artist
// column widths change when the terminal width changes via WindowSizeMsg.
func TestQueueRenderProportionalColumnWidths(t *testing.T) {
	entry := QueueEntry{
		Title:      strings.Repeat("T", 60),
		Artist:     strings.Repeat("A", 60),
		DurationMs: 90_000,
		Position:   1000,
	}

	narrow := makeQueue(80, []QueueEntry{entry})
	wide := makeQueue(160, []QueueEntry{entry})

	narrowOut := narrow.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	wideOut := wide.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)

	narrowTs := strings.Count(narrowOut, "T")
	wideTs := strings.Count(wideOut, "T")
	if wideTs <= narrowTs {
		t.Errorf("wide terminal must show more title chars: narrow=%d, wide=%d", narrowTs, wideTs)
	}
}

// TestQueueRenderWindowSizeMsg verifies that sending a WindowSizeMsg to the
// root model and then rendering does not panic and produces non-empty content.
func TestQueueRenderWindowSizeMsg(t *testing.T) {
	m := New(context.Background(), WithQueueEntries([]QueueEntry{
		{Title: "Track", Artist: "Artist", DurationMs: 30_000, Position: 1000},
	}))
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	resized, _ := nm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := resized.(Model)

	v := rm.View()
	if v.Content == "" {
		t.Fatal("View() after WindowSizeMsg must produce non-empty content")
	}
	if !strings.Contains(v.Content, "Track") {
		t.Error("queue view must contain the enqueued track title after resize")
	}
}

// TestQueueRenderSelectedEntryShowsMarker verifies that a bulk-selected queue
// entry is rendered with a "●" selection marker.
func TestQueueRenderSelectedEntryShowsMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false},
		{Title: "Selected", Artist: "B", DurationMs: 120_000, Selected: true},
	})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if !strings.Contains(out, "●") {
		t.Fatal("selected queue entry must show a ● marker")
	}
}

// TestQueueRenderUnselectedEntryHasNoMarker verifies that unselected queue
// entries do not contain a selection marker glyph.
func TestQueueRenderUnselectedEntryHasNoMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false}})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if strings.Contains(out, "●") {
		t.Fatal("unselected queue entry must not show a ● marker")
	}
}

// TestQueueRenderNowPlayingMarker verifies that the currently playing track
// gets a "▶" marker in the rendered output.
func TestQueueRenderNowPlayingMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Not Playing", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Playing", Artist: "B", Hash: "bbb", DurationMs: 120_000},
	})
	v.cursor = 0
	out := v.render(styles.NewStyles(true), 1, PlaybackPlaying, nil, nil, nil)
	stripped := ansi.Strip(out)

	lines := strings.Split(stripped, "\n")
	var playingLine string
	for _, l := range lines {
		if strings.Contains(l, "Playing") && !strings.Contains(l, "Not Playing") {
			playingLine = l
			break
		}
	}
	if playingLine == "" {
		t.Fatal("playing entry must appear in the output")
	}
	if !strings.Contains(playingLine, "▶") {
		t.Errorf("now-playing entry must contain ▶ marker; got: %q", playingLine)
	}
}

// TestQueueRenderNonPlayingEntryHasNoPlayMarker verifies that entries that are
// not currently playing do not get a "▶" marker.
func TestQueueRenderNonPlayingEntryHasNoPlayMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000}})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	stripped := ansi.Strip(out)

	for l := range strings.SplitSeq(stripped, "\n") {
		if strings.Contains(l, "Normal") {
			if strings.Contains(l, "▶") {
				t.Errorf("non-playing entry must not contain ▶ marker; got: %q", l)
			}
			return
		}
	}
	t.Fatal("entry must appear in the output")
}

// TestQueueRenderPlayedRowDimGlyph verifies that an entry before the
// now-playing index renders with the played-tier ╵ left glyph and dim styling.
func TestQueueRenderPlayedRowDimGlyph(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Past", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Now", Artist: "B", Hash: "bbb", DurationMs: 60_000},
		{Title: "Next", Artist: "C", Hash: "ccc", DurationMs: 60_000},
	})
	v.cursor = 1
	s := styles.NewStyles(true)
	out := v.render(s, 1, PlaybackPlaying, nil, nil, nil)

	var pastLine, nextLine string
	for l := range strings.SplitSeq(out, "\n") {
		stripped := ansi.Strip(l)
		switch {
		case strings.Contains(stripped, "Past"):
			pastLine = l
		case strings.Contains(stripped, "Next"):
			nextLine = l
		}
	}
	if pastLine == "" {
		t.Fatal("played entry must appear in output")
	}
	if !strings.Contains(ansi.Strip(pastLine), "╵") {
		t.Errorf("played row must contain ╵ glyph; got: %q", ansi.Strip(pastLine))
	}
	if nextLine != "" && strings.Contains(ansi.Strip(nextLine), "╵") {
		t.Errorf("upcoming row must not contain ╵ glyph; got: %q", ansi.Strip(nextLine))
	}

	playedRef := s.PlayedRow.Render("REF")
	if ansi.Strip(playedRef) == playedRef {
		return
	}
	playedPrefix, _, _ := strings.Cut(playedRef, "REF")
	if !strings.Contains(pastLine, playedPrefix) {
		t.Errorf("played row must use PlayedRow style; got: %q", pastLine)
	}
}

// TestQueueRenderUpcomingRowNotDimmed verifies that an entry after the
// now-playing index keeps full-contrast styling (no PlayedRow override).
func TestQueueRenderUpcomingRowNotDimmed(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Now", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Upcoming", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	v.cursor = 0
	s := styles.NewStyles(true)
	out := v.render(s, 0, PlaybackPlaying, nil, nil, nil)

	var line string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(l), "Upcoming") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("upcoming entry must appear in output")
	}
	if strings.Contains(ansi.Strip(line), "╵") {
		t.Errorf("upcoming row must not contain ╵ glyph; got: %q", ansi.Strip(line))
	}
}

// TestQueueRenderNoPlayedTierWhenIdle verifies that with no now-playing index
// (-1), no rows are treated as played.
func TestQueueRenderNoPlayedTierWhenIdle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "One", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Two", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if strings.Contains(ansi.Strip(out), "╵") {
		t.Errorf("no rows should be dimmed when idle; got: %q", ansi.Strip(out))
	}
}

// TestQueueRenderSelectedPlayedShowsSelectionMarker verifies that a row which
// is both selected and in the played tier keeps the selection "●" marker
// (selection precedence wins over the played ╵ glyph).
func TestQueueRenderSelectedPlayedShowsSelectionMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "PastSel", Artist: "A", Hash: "aaa", DurationMs: 60_000, Selected: true},
		{Title: "Now", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	v.cursor = 1
	s := styles.NewStyles(true)
	out := v.render(s, 1, PlaybackPlaying, nil, nil, nil)

	var line string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(l), "PastSel") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("selected played entry must appear in output")
	}
	stripped := ansi.Strip(line)
	if !strings.Contains(stripped, "●") {
		t.Errorf("selected+played row must keep ● marker; got: %q", stripped)
	}
	if strings.Contains(stripped, "╵") {
		t.Errorf("selected+played row must not show ╵ marker (selection wins); got: %q", stripped)
	}
}

// TestQueueRenderEndOfQueueSentinel verifies that the "— end of queue —"
// sentinel line appears when the last entry is in the viewport.
func TestQueueRenderEndOfQueueSentinel(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "First", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Last", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if !strings.Contains(ansi.Strip(out), "— end of queue —") {
		t.Errorf("sentinel must appear when last entry is on screen; got: %q", ansi.Strip(out))
	}
}

// TestQueueRenderEndOfQueueSentinelHiddenWhenScrolledAbove verifies that the
// sentinel does NOT appear when the viewport does not reach the last entry.
func TestQueueRenderEndOfQueueSentinelHiddenWhenScrolledAbove(t *testing.T) {
	entries := make([]QueueEntry, 30)
	for i := range entries {
		entries[i] = QueueEntry{Title: "T", Artist: "A", Hash: "h", DurationMs: 60_000}
	}
	v := QueueView{entries: entries, width: 120, height: 8}
	v.scrollOffset = 0
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if strings.Contains(ansi.Strip(out), "— end of queue —") {
		t.Errorf("sentinel must be hidden when last entry is below viewport; got: %q", ansi.Strip(out))
	}
}

// TestQueueRenderEndOfQueueSentinelHiddenWhenEmpty verifies that the sentinel
// is not appended to the empty-queue card.
func TestQueueRenderEndOfQueueSentinelHiddenWhenEmpty(t *testing.T) {
	v := QueueView{}
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	if strings.Contains(ansi.Strip(out), "— end of queue —") {
		t.Errorf("sentinel must not appear in empty-queue card; got: %q", ansi.Strip(out))
	}
}

// TestQueueRenderSourceColumnStableAcrossScroll guards against a regression
// where hasSource was computed from visible entries only, causing the Source
// column to appear/disappear (and the Title column to shift width) as the user
// scrolled. The column must be present whenever ANY entry has a source.
func TestQueueRenderSourceColumnStableAcrossScroll(t *testing.T) {
	entries := make([]QueueEntry, 30)
	for i := range entries {
		entries[i] = QueueEntry{Title: "T", Artist: "A", Hash: "h", DurationMs: 60_000}
	}
	// Only an off-screen entry has a source.
	entries[29].SourcePlaylist = "MyPlaylist"
	v := QueueView{entries: entries, width: 120, height: 8}
	v.scrollOffset = 0
	out := ansi.Strip(v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil))
	if !strings.Contains(out, "Source") {
		t.Errorf("Source column header must appear when any entry has a source even if scrolled away; got: %q", out)
	}
}
