package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
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
