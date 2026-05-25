package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueFilterRendersFilterBar verifies that the filter chunk appears in
// the rendered queue output when the filter is active.
func TestQueueFilterRendersFilterBar(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Song", Artist: "Art", Album: "Alb"},
		},
		width:  120,
		height: 20,
	}
	q.filter, _ = q.filter.Activate()
	for _, ch := range "song" {
		q.filter, _ = q.filter.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	q.cursor = 0
	q.scrollOffset = 0

	s := styles.NewStyles(true)
	out := q.render(s, -1, PlaybackIdle, nil, nil, nil)
	if !strings.Contains(out, `filter "`) {
		t.Errorf("rendered output should contain filter chunk, got:\n%s", out)
	}
}

// TestQueueFilterNoMatchesShowsMessage verifies that when filter matches
// nothing, "(no matches)" is displayed.
func TestQueueFilterNoMatchesShowsMessage(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Song", Artist: "Art"},
		},
	}
	q.filter, _ = q.filter.Activate()
	q.filter.Text = "zzzzz"
	q.cursor = 0
	q.scrollOffset = 0

	s := styles.NewStyles(true)
	out := q.render(s, -1, PlaybackIdle, nil, nil, nil)
	if !strings.Contains(out, "(no matches)") {
		t.Errorf("expected '(no matches)' when filter has no results, got:\n%s", out)
	}
}
