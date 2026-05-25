package db_test

import (
	"testing"
	"time"
)

func TestReadListeningStats_Empty(t *testing.T) {
	h := newPlayHistoryHarness(t)

	stats := h.readListeningStats(t, 0)
	if stats.TotalPlays != 0 {
		t.Errorf("TotalPlays = %d, want 0", stats.TotalPlays)
	}
	if stats.TotalListenedSecs != 0 {
		t.Errorf("TotalListenedSecs = %d, want 0", stats.TotalListenedSecs)
	}
	if stats.CurrentStreak != 0 {
		t.Errorf("CurrentStreak = %d, want 0", stats.CurrentStreak)
	}

	hiddenGemsDescription := ""
	for _, entry := range stats.MixStats {
		if entry.Name == "Hidden Gems" {
			hiddenGemsDescription = entry.Description
			break
		}
	}
	if hiddenGemsDescription != "Low-played tracks by your favorite artists" {
		t.Errorf("Hidden Gems description = %q, want low-played wording", hiddenGemsDescription)
	}
}

func TestReadListeningStats_Totals(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	now := time.Now().Unix()
	h.recordPlay(t, "h1", now, 120, 0)
	h.recordPlay(t, "h1", now+1, 80, 0)

	stats := h.readListeningStats(t, 0)
	if stats.TotalPlays != 2 {
		t.Errorf("TotalPlays = %d, want 2", stats.TotalPlays)
	}
	if stats.TotalListenedSecs != 200 {
		t.Errorf("TotalListenedSecs = %d, want 200", stats.TotalListenedSecs)
	}
}

func TestReadListeningStats_SinceFilter(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	old := time.Now().Add(-48 * time.Hour).Unix()
	recent := time.Now().Unix()
	h.recordPlay(t, "h1", old, 100, 0)
	h.recordPlay(t, "h1", recent, 50, 0)

	yesterday := time.Now().Add(-24 * time.Hour).Unix()
	stats := h.readListeningStats(t, yesterday)
	if stats.TotalPlays != 1 {
		t.Errorf("TotalPlays = %d, want 1", stats.TotalPlays)
	}
	if stats.TotalListenedSecs != 50 {
		t.Errorf("TotalListenedSecs = %d, want 50", stats.TotalListenedSecs)
	}
}

func TestReadListeningStats_TopArtists(t *testing.T) {
	h := newPlayHistoryHarness(t)
	now := time.Now().Unix()

	insertFullTrack(t, h.d, "h1", "/a.flac", "Song1", "ArtistA", "AlbumA", "rock", 180000, 5000000, now)
	insertFullTrack(t, h.d, "h2", "/b.flac", "Song2", "ArtistB", "AlbumB", "pop", 200000, 6000000, now)

	for range 3 {
		h.recordPlay(t, "h1", now, 60, 0)
	}
	h.recordPlay(t, "h2", now, 60, 0)

	stats := h.readListeningStats(t, 0)
	if len(stats.TopArtists) == 0 {
		t.Fatal("TopArtists is empty")
	}
	if stats.TopArtists[0].Name != "ArtistA" {
		t.Errorf("TopArtists[0].Name = %q, want %q", stats.TopArtists[0].Name, "ArtistA")
	}
	if stats.TopArtists[0].PlayCount != 3 {
		t.Errorf("TopArtists[0].PlayCount = %d, want 3", stats.TopArtists[0].PlayCount)
	}
}

func TestReadListeningStats_StreakToday(t *testing.T) {
	assertCurrentStreak(t, []int{0, -1, -2}, 3)
}

func TestReadListeningStats_StreakStartingYesterday(t *testing.T) {
	assertCurrentStreak(t, []int{-1, -2}, 2)
}

func TestReadListeningStats_StreakBroken(t *testing.T) {
	assertCurrentStreak(t, []int{-3}, 0)
}

func assertCurrentStreak(t *testing.T, dayOffsets []int, want int) {
	t.Helper()
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	now := time.Now()
	for _, dayOffset := range dayOffsets {
		insertPlayHistoryRow(t, h.d, "h1", playHistoryMidday(now, dayOffset), 60)
	}

	stats := h.readListeningStats(t, 0)
	if stats.CurrentStreak != want {
		t.Errorf("CurrentStreak = %d, want %d for day offsets %v", stats.CurrentStreak, want, dayOffsets)
	}
}

func playHistoryMidday(now time.Time, dayOffset int) int64 {
	return time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).
		AddDate(0, 0, dayOffset).
		Unix()
}
