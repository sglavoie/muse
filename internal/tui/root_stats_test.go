package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"

	"github.com/sglavoie/muse/internal/db"
)

func TestSinceUnixForPeriod_AllTime(t *testing.T) {
	got := sinceUnixForPeriod(0) // "All Time" → sinceDays = 0
	if got != 0 {
		t.Errorf("all-time period: got %d, want 0", got)
	}
}

func TestSinceUnixForPeriod_ValidPeriods(t *testing.T) {
	// Ensure each valid period index does not panic and returns a non-negative value.
	for i := range len(statsPeriods) {
		got := sinceUnixForPeriod(i)
		if got < 0 {
			t.Errorf("period %d (%s): got negative unix timestamp %d", i, statsPeriods[i].label, got)
		}
	}
}

func TestSinceUnixForPeriod_OutOfBounds(t *testing.T) {
	// Out-of-bounds indices must not panic and should return 0 (all-time).
	if got := sinceUnixForPeriod(-1); got != 0 {
		t.Errorf("negative index: got %d, want 0", got)
	}
	if got := sinceUnixForPeriod(len(statsPeriods)); got != 0 {
		t.Errorf("past-end index: got %d, want 0", got)
	}
	if got := sinceUnixForPeriod(999); got != 0 {
		t.Errorf("large index: got %d, want 0", got)
	}
}

func TestStatsUpdateDiscardsStalePeriodResult(t *testing.T) {
	m := New(context.Background())
	m.statsPeriod = 2 // Last 30 Days
	m.statsViewport = viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	m.statsViewport.SetContent("original stats")

	next, cmd := m.Update(listeningStatsMsg{
		periodIdx: 1, // Last 7 Days, from an older request
		stats:     db.ListeningStats{TotalPlays: 42},
	})
	if cmd != nil {
		t.Fatal("expected no command for stale stats result")
	}
	got := next.(Model).statsViewport.View()
	if !strings.Contains(got, "original stats") {
		t.Fatalf("stale stats result replaced viewport content: %q", got)
	}
}

func TestStatsUpdateUsesMessagePeriodLabel(t *testing.T) {
	m := New(context.Background())
	m.statsPeriod = 1
	m.statsViewport = viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))

	next, cmd := m.Update(listeningStatsMsg{
		periodIdx: 1,
		stats:     db.ListeningStats{TotalPlays: 1},
	})
	if cmd != nil {
		t.Fatal("expected no command for stats result")
	}
	got := next.(Model).statsViewport.View()
	if !strings.Contains(got, "Last 7 Days") {
		t.Fatalf("stats viewport missing requested period label: %q", got)
	}
}

func TestFormatListeningTime(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{0, "0s"},
		{-5, "0s"},
		{30, "30s"},
		{60, "1m"},
		{90, "1m"},
		{3600, "1h"},
		{3660, "1h 1m"},
		{7200, "2h"},
		{7260, "2h 1m"},
	}
	for _, tt := range tests {
		got := formatListeningTime(tt.secs)
		if got != tt.want {
			t.Errorf("formatListeningTime(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}
