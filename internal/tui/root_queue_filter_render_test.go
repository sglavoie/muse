package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueFilterRendersFilterBar verifies that the filter chunk appears in
// the rendered queue output when the filter is active.
func TestQueueFilterRendersFilterBar(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Title: "Song", Artist: "Art", Album: "Alb"},
		},
		Width:  120,
		Height: 20,
	}
	q.Filter, _ = q.Filter.Activate()
	for _, ch := range "song" {
		q.Filter, _ = q.Filter.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	q.Cursor = 0
	q.ScrollOffset = 0

	s := styles.NewStyles(true)
	out := q.Render(s, -1, playback.StateIdle, nil, nil, nil)
	if !strings.Contains(out, `filter "`) {
		t.Errorf("rendered output should contain filter chunk, got:\n%s", out)
	}
}

// TestQueueFilterNoMatchesShowsMessage verifies that when filter matches
// nothing, "No matches — Esc clears text filter" is displayed.
func TestQueueFilterNoMatchesShowsMessage(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Title: "Song", Artist: "Art"},
		},
	}
	q.Filter, _ = q.Filter.Activate()
	q.Filter.Text = "zzzzz"
	q.Cursor = 0
	q.ScrollOffset = 0

	s := styles.NewStyles(true)
	out := q.Render(s, -1, playback.StateIdle, nil, nil, nil)
	if !strings.Contains(out, "No matches — Esc clears text filter") {
		t.Errorf("expected text-filter recovery hint when filter has no results, got:\n%s", out)
	}
}
