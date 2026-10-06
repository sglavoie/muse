package queue

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"
)

func TestSelectionSummaryCountsEntriesIncludingHiddenDuplicates(t *testing.T) {
	v := View{Width: 120, Entries: []Entry{
		{ID: 1, Hash: "a", Title: "Alpha", DurationMs: 61500, Selected: true},
		{ID: 2, Hash: "a", Title: "Alpha", DurationMs: 61500, Selected: true},
		{ID: 3, Hash: "b", Title: "Beta", DurationMs: 0, Selected: true},
		{ID: 4, Hash: "c", Title: "Other", DurationMs: 99000},
	}}
	v.Filter.Text = "Alpha"
	summary := v.selectionSummary()
	if summary.Count != 3 || summary.Hidden != 1 || summary.Duration != "2:03+" {
		t.Fatalf("summary = %+v", summary)
	}
	v.ScrollOffset = 1
	if *v.selectionSummary() != *summary {
		t.Fatal("scroll changed summary")
	}
	v.TagFilterHashes = map[string]struct{}{"b": {}}
	if got := v.selectionSummary().Hidden; got != 3 {
		t.Fatalf("all hidden = %d", got)
	}
	out := ansi.Strip(v.Render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil))
	if !strings.Contains(out, "3 selected · 3 hidden · 2:03+") {
		t.Fatalf("no-match screen lost selection summary: %s", out)
	}
	v.TagFilterHashes = nil
	for _, width := range []int{60, 80, 120} {
		v.Width = width
		line := v.renderStateStrip(styles.NewStyles(true), 4, 2, false)
		if ansi.StringWidth(line) > width {
			t.Fatalf("overflow at %d: %s", width, line)
		}
		if !strings.Contains(ansi.Strip(line), "3 selected · 1 hidden") {
			t.Fatalf("missing selection: %s", line)
		}
	}
}
