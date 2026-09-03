package queue

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// titlePattern matches the synthetic titles the timeline render tests use, so a
// test can read the rendered row order straight off the panel.
var titlePattern = regexp.MustCompile(`T\d+`)

// renderedRows returns one entry per rendered data row: its title and the
// playback tier its markers report.
type renderedRow struct {
	title  string
	played bool
	now    bool
}

func renderRows(t *testing.T, out string) []renderedRow {
	t.Helper()
	// The state strip is the panel's first line and can quote a filter term
	// that looks exactly like a row title, so drop it before scanning.
	_, body, _ := strings.Cut(out, "\n")
	var rows []renderedRow
	for line := range strings.SplitSeq(body, "\n") {
		stripped := ansi.Strip(line)
		title := titlePattern.FindString(stripped)
		if title == "" {
			continue
		}
		rows = append(rows, renderedRow{
			title:  title,
			played: strings.Contains(stripped, "╵"),
			now:    strings.Contains(stripped, "▶"),
		})
	}
	return rows
}

func renderedTitles(rows []renderedRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.title
	}
	return out
}

// numberedTitles builds n entries titled T0..Tn-1.
func numberedTitles(n int) []string {
	titles := make([]string, n)
	for i := range titles {
		titles[i] = fmt.Sprintf("T%d", i)
	}
	return titles
}

// timelineQueue builds a view whose rows are ordered by the given shuffled
// timeline, with no focused row so every marker reports playback state alone.
func timelineQueue(n, current int, order ...int) View {
	v := makeQueue(120, entriesTitled(numberedTitles(n)...))
	v.Cursor = -1
	v.Traversal = shuffledTimeline(current, order...)
	return v
}

// TestRenderRowOrderFollowsTimeline verifies the panel renders the order the
// player will visit, not the order rows are stored in.
func TestRenderRowOrderFollowsTimeline(t *testing.T) {
	v := timelineQueue(4, 1, 2, 0, 3, 1)
	rows := renderRows(t, v.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil))

	if want := []string{"T2", "T0", "T3", "T1"}; !slices.Equal(renderedTitles(rows), want) {
		t.Errorf("rendered order = %v, want %v", renderedTitles(rows), want)
	}
}

// TestRenderPlayedTierFollowsTraversalRank is the core of the timeline render:
// a row late in the queue that already played must read as spent, and a row
// early in the queue that has not played yet must not. Underlying index order
// says the opposite in both cases.
func TestRenderPlayedTierFollowsTraversalRank(t *testing.T) {
	// Timeline: T3 played, T0 playing, T1 and T2 still to come.
	v := timelineQueue(4, 1, 3, 0, 1, 2)
	rows := renderRows(t, v.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil))

	want := map[string]struct{ played, now bool }{
		"T3": {played: true},
		"T0": {now: true},
		"T1": {},
		"T2": {},
	}
	for _, row := range rows {
		w, ok := want[row.title]
		if !ok {
			t.Fatalf("unexpected row %q", row.title)
		}
		if row.played != w.played || row.now != w.now {
			t.Errorf("%s: played=%v now=%v, want played=%v now=%v",
				row.title, row.played, row.now, w.played, w.now)
		}
	}
}

// TestRenderForwardHistoryAfterPreviousIsQueued verifies that history the user
// stepped back over reads as scheduled: Previous did not unplay those rows, but
// they will play again, so dimming them would be a lie about what is coming.
func TestRenderForwardHistoryAfterPreviousIsQueued(t *testing.T) {
	// The cursor sits at rank 0 with two already-visited rows still ahead of it.
	v := timelineQueue(3, 0, 0, 1, 2)
	rows := renderRows(t, v.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil))

	for _, row := range rows {
		if row.title == "T0" {
			continue
		}
		if row.played {
			t.Errorf("%s: forward history must render as queued, not played", row.title)
		}
	}
}

// TestRenderUnscheduledRowsRenderAsQueued verifies that rows the cycle no
// longer schedules — which the player appends after everything else — stay
// visible and read as queued rather than as played.
func TestRenderUnscheduledRowsRenderAsQueued(t *testing.T) {
	// T2 played, T0 playing, T1 and T3 unscheduled and appended at the end.
	v := timelineQueue(4, 1, 2, 0, 1, 3)
	rows := renderRows(t, v.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil))

	if want := []string{"T2", "T0", "T1", "T3"}; !slices.Equal(renderedTitles(rows), want) {
		t.Fatalf("rendered order = %v, want %v", renderedTitles(rows), want)
	}
	for _, row := range rows[2:] {
		if row.played {
			t.Errorf("%s: unscheduled row must not render as played", row.title)
		}
	}
}

// TestRenderMissingStyleDominatesTimeline verifies that an unavailable file
// still reads as missing even when the timeline has already moved past it.
func TestRenderMissingStyleDominatesTimeline(t *testing.T) {
	v := timelineQueue(3, 1, 2, 0, 1)
	v.Entries[2].Missing = true
	out := v.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil)

	var line string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(l), "T2") {
			line = ansi.Strip(l)
			break
		}
	}
	if line == "" {
		t.Fatal("missing entry must still appear in the panel")
	}
	if !strings.Contains(line, "[missing]") {
		t.Errorf("played missing row must keep the [missing] suffix; got %q", line)
	}
	if strings.Contains(line, "╵") {
		t.Errorf("missing styling must win over the played tier; got %q", line)
	}
}

// TestRenderIdleTimelineMarksNothingPlayed verifies that a timeline naming no
// current row leaves every row queued, matching the no-provider idle case.
func TestRenderIdleTimelineMarksNothingPlayed(t *testing.T) {
	v := timelineQueue(3, -1, 2, 0, 1)
	out := v.Render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
	if strings.Contains(ansi.Strip(out), "╵") {
		t.Errorf("no rows may be played when the timeline names no current row; got %q", ansi.Strip(out))
	}
}

// TestRenderPositionsNumberTheTimeline verifies the "#" column counts down the
// effective timeline rather than restating storage positions.
func TestRenderPositionsNumberTheTimeline(t *testing.T) {
	v := timelineQueue(4, -1, 3, 1, 0, 2)
	out := v.Render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)

	wantByPos := []string{"T3", "T1", "T0", "T2"}
	for line := range strings.SplitSeq(out, "\n") {
		stripped := strings.TrimSpace(ansi.Strip(line))
		title := titlePattern.FindString(stripped)
		if title == "" {
			continue
		}
		pos := slices.Index(wantByPos, title) + 1
		if !strings.HasPrefix(stripped, fmt.Sprintf("%d ", pos)) {
			t.Errorf("%s: row must be numbered %d; got %q", title, pos, stripped)
		}
	}
}

// TestRenderStateStripAnnouncesShuffleOrder verifies the strip tells the user
// the numbering is a playback timeline, and only while that is true.
func TestRenderStateStripAnnouncesShuffleOrder(t *testing.T) {
	const label = "Queue · shuffle order"

	v := timelineQueue(3, 0, 2, 0, 1)
	out := ansi.Strip(v.Render(styles.NewStyles(true), 2, playback.StatePlaying, nil, nil, nil))
	if !strings.Contains(out, label) {
		t.Errorf("active timeline must announce %q; got %q", label, firstLine(out))
	}

	sorted := v
	sorted.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	sorted = sorted.RecomputeDisplayOrder()
	out = ansi.Strip(sorted.Render(styles.NewStyles(true), 2, playback.StatePlaying, nil, nil, nil))
	if strings.Contains(out, label) {
		t.Errorf("explicit sort must not claim shuffle order; got %q", firstLine(out))
	}

	off := makeQueue(120, entriesTitled(numberedTitles(3)...))
	off.Traversal = sequentialTimeline(0, 3)
	out = ansi.Strip(off.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil))
	if strings.Contains(out, label) {
		t.Errorf("shuffle off must not claim shuffle order; got %q", firstLine(out))
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// TestRenderTimelineNarrowWidth verifies the timeline path survives a layout
// too narrow for the position column.
func TestRenderTimelineNarrowWidth(t *testing.T) {
	for _, width := range []int{1, 6, 10, 24} {
		v := timelineQueue(4, 1, 3, 0, 2, 1)
		v.Width = width
		out := v.Render(styles.NewStyles(true), 0, playback.StatePlaying, nil, nil, nil)
		if out == "" {
			t.Errorf("width %d: render must produce output", width)
		}
	}
}

// TestRenderTimelineLargeScrollWindow verifies that scrolling deep into a long
// queue shows the timeline slice, not the storage slice.
func TestRenderTimelineLargeScrollWindow(t *testing.T) {
	const n, offset = 200, 100
	order := make([]int, n)
	for i := range order {
		order[i] = n - 1 - i
	}
	v := timelineQueue(n, 40, order...)
	v.Height = 13
	v.ScrollOffset = offset

	end := min(offset+v.VisibleRows(), len(order))
	got := renderedTitles(renderRows(t, v.Render(styles.NewStyles(true), order[40], playback.StatePlaying, nil, nil, nil)))
	want := make([]string, 0, end-offset)
	for _, idx := range order[offset:end] {
		want = append(want, fmt.Sprintf("T%d", idx))
	}
	if !slices.Equal(got, want) {
		t.Errorf("scrolled window = %v, want %v", got, want)
	}
}

// TestRenderShuffleOffMatchesNoProvider verifies that a player reporting
// sequential traversal renders byte-identically to one with no timeline at all.
func TestRenderShuffleOffMatchesNoProvider(t *testing.T) {
	s := styles.NewStyles(true)
	base := makeQueue(120, entriesTitled(numberedTitles(5)...))
	base.Cursor = 2
	base.Entries[3].Selected = true

	withTimeline := base
	withTimeline.Traversal = sequentialTimeline(1, 5)

	want := base.Render(s, 1, playback.StatePlaying, nil, nil, nil)
	got := withTimeline.Render(s, 1, playback.StatePlaying, nil, nil, nil)
	if got != want {
		t.Error("a sequential timeline must render exactly like no timeline at all")
	}
}

// TestRenderTimelineWithFilterKeepsTimelineTiers verifies that filtering out
// rows changes neither the surviving rows' order nor their playback tier.
func TestRenderTimelineWithFilterKeepsTimelineTiers(t *testing.T) {
	v := timelineQueue(6, 2, 4, 5, 1, 0, 3, 2)
	v.Filter.Text = "AT" // matches every synthetic artist ("AT0", "AT1", ...)
	all := renderRows(t, v.Render(styles.NewStyles(true), 1, playback.StatePlaying, nil, nil, nil))

	v.Filter.Text = "AT1"
	filtered := renderRows(t, v.Render(styles.NewStyles(true), 1, playback.StatePlaying, nil, nil, nil))
	if len(filtered) != 1 {
		t.Fatalf("filter must keep one row, got %d", len(filtered))
	}
	idx := slices.IndexFunc(all, func(r renderedRow) bool { return r.title == "T1" })
	if idx < 0 {
		t.Fatal("T1 must be present unfiltered")
	}
	if filtered[0] != all[idx] {
		t.Errorf("filtered row = %+v, want the unfiltered tier %+v", filtered[0], all[idx])
	}
}
