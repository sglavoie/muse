package radio

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

var testNow = time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)

func daysAgo(days int) int64 {
	return testNow.AddDate(0, 0, -days).Unix()
}

func TestDiscoveryCutoff_Is90DaysBack(t *testing.T) {
	got := DiscoveryCutoff(testNow)
	want := testNow.AddDate(0, 0, -90)
	if !got.Equal(want) {
		t.Errorf("DiscoveryCutoff = %v, want %v", got, want)
	}
	if DiscoveryCutoffDays != 90 {
		t.Errorf("cutoff = %d days, want 90", DiscoveryCutoffDays)
	}
}

func TestClassify_CutoffBoundaries(t *testing.T) {
	clock := FixedClock(testNow)
	cases := []struct {
		name         string
		playCount    int
		lastPlayedAt int64
		want         DiscoveryClass
	}{
		{"never played", 0, 0, DiscoveryNever},
		{"never played with stray timestamp", 0, daysAgo(1), DiscoveryNever},
		{"negative play count", -2, daysAgo(1), DiscoveryNever},
		{"one second before the cutoff", 3, daysAgo(90) + 1, DiscoveryFamiliar},
		{"exactly at the cutoff", 3, daysAgo(90), DiscoveryStale},
		{"one second past the cutoff", 3, daysAgo(90) - 1, DiscoveryStale},
		{"long ago", 3, daysAgo(365), DiscoveryStale},
		{"yesterday", 1, daysAgo(1), DiscoveryFamiliar},
		{"played but timestamp missing", 4, 0, DiscoveryFamiliar},
		{"played but timestamp negative", 4, -5, DiscoveryFamiliar},
		{"future timestamp", 4, testNow.AddDate(0, 0, 5).Unix(), DiscoveryFamiliar},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.playCount, tc.lastPlayedAt, clock)
			if got != tc.want {
				t.Errorf("Classify(%d, %d) = %q, want %q", tc.playCount, tc.lastPlayedAt, got, tc.want)
			}
		})
	}
}

func TestDiscoveryClass_IsDiscovery(t *testing.T) {
	if !DiscoveryNever.IsDiscovery() || !DiscoveryStale.IsDiscovery() {
		t.Errorf("never-played and stale tracks are discovery")
	}
	if DiscoveryFamiliar.IsDiscovery() {
		t.Errorf("familiar tracks are not discovery")
	}
}

func TestClassifyTrack_UsesCountedPlayFields(t *testing.T) {
	clock := FixedClock(testNow)
	stale := library.Track{Hash: "a", PlayCount: 2, LastPlayedAt: daysAgo(120)}
	if got := ClassifyTrack(stale, clock); got != DiscoveryStale {
		t.Errorf("ClassifyTrack = %q, want stale", got)
	}
	if !IsDiscovery(stale, clock) {
		t.Errorf("stale track must count as discovery")
	}
	fresh := library.Track{Hash: "b", PlayCount: 2, LastPlayedAt: daysAgo(2)}
	if IsDiscovery(fresh, clock) {
		t.Errorf("recently played track must not count as discovery")
	}
}

func TestClassify_ClockIsInjectable(t *testing.T) {
	track := library.Track{PlayCount: 1, LastPlayedAt: daysAgo(45)}
	if got := ClassifyTrack(track, FixedClock(testNow)); got != DiscoveryFamiliar {
		t.Fatalf("45 days ago is familiar, got %q", got)
	}
	later := FixedClock(testNow.AddDate(0, 0, 60))
	if got := ClassifyTrack(track, later); got != DiscoveryStale {
		t.Errorf("the same track must age into discovery as the clock advances, got %q", got)
	}
}

func TestNowFrom_NilClockFallsBackToSystemTime(t *testing.T) {
	before := time.Now()
	got := nowFrom(nil)
	if got.Before(before.Add(-time.Second)) {
		t.Errorf("nil clock must fall back to the system clock, got %v", got)
	}
	if got := nowFrom(SystemClock()); got.IsZero() {
		t.Errorf("SystemClock must report a real time")
	}
}

func TestDiscoveryTargetCount(t *testing.T) {
	cases := []struct {
		batch int
		pct   int
		want  int
	}{
		{5, 20, 1}, // Familiar
		{5, 40, 2}, // Balanced
		{5, 80, 4}, // Explore
		{5, 0, 0},
		{5, 100, 5},
		{5, 50, 3}, // 2.5 rounds up
		{0, 40, 0},
		{-3, 40, 0},
		{5, -10, 0}, // clamped
		{5, 250, 5}, // clamped
		{10, 40, 4},
	}
	for _, tc := range cases {
		if got := DiscoveryTargetCount(tc.batch, tc.pct); got != tc.want {
			t.Errorf("DiscoveryTargetCount(%d, %d) = %d, want %d", tc.batch, tc.pct, got, tc.want)
		}
	}
}

func TestCountDiscovery(t *testing.T) {
	clock := FixedClock(testNow)
	tracks := []library.Track{
		{Hash: "a"}, // never played
		{Hash: "b", PlayCount: 1, LastPlayedAt: daysAgo(1)},  // familiar
		{Hash: "c", PlayCount: 1, LastPlayedAt: daysAgo(90)}, // stale
	}
	if got := CountDiscovery(tracks, clock); got != 2 {
		t.Errorf("CountDiscovery = %d, want 2", got)
	}
	if got := CountDiscovery(nil, clock); got != 0 {
		t.Errorf("CountDiscovery(nil) = %d, want 0", got)
	}
}
