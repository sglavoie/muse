package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueRenderEndOfQueueSentinel verifies that the "— end of queue —"
// sentinel line appears when the last entry is in the viewport.
func TestQueueRenderEndOfQueueSentinel(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "First", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Last", Artist: "B", Hash: "bbb", DurationMs: 60_000},
	})
	out := v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
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
	out := v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
	if strings.Contains(ansi.Strip(out), "— end of queue —") {
		t.Errorf("sentinel must be hidden when last entry is below viewport; got: %q", ansi.Strip(out))
	}
}

// TestQueueRenderEndOfQueueSentinelHiddenWhenEmpty verifies that the sentinel
// is not appended to the empty-queue card.
func TestQueueRenderEndOfQueueSentinelHiddenWhenEmpty(t *testing.T) {
	v := QueueView{}
	out := v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil)
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
	out := ansi.Strip(v.render(styles.NewStyles(true), -1, playback.StateIdle, nil, nil, nil))
	if !strings.Contains(out, "Source") {
		t.Errorf("Source column header must appear when any entry has a source even if scrolled away; got: %q", out)
	}
}
