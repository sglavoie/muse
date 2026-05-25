package db_test

import (
	"testing"
	"time"
)

func TestRecordPlay_InsertsHistoryRow(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	h.recordPlay(t, "h1", time.Now().Unix(), 120, 0)

	if got := playHistoryRowCount(t, h.d, "h1"); got != 1 {
		t.Errorf("play_history rows = %d, want 1", got)
	}
}

func TestRecordPlay_IncrementsPlayCount(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	now := time.Now().Unix()
	for range 3 {
		h.recordPlay(t, "h1", now, 90, 0)
	}

	if got := trackPlayCount(t, h.d, "h1"); got != 3 {
		t.Errorf("play_count = %d, want 3", got)
	}
}

func TestRecordPlay_SetsLastPlayedAt(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	playedAt := time.Now().Unix()
	h.recordPlay(t, "h1", playedAt, 60, 0)

	if got := trackLastPlayedAt(t, h.d, "h1"); got != playedAt {
		t.Errorf("last_played_at = %d, want %d", got, playedAt)
	}
}

func TestRecordPlay_CompletionRate_Computed(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	h.recordPlay(t, "h1", time.Now().Unix(), 60, 120.0)

	if got := playHistoryCompletionRate(t, h.d, "h1"); got != 0.5 {
		t.Errorf("completion_rate = %f, want 0.5", got)
	}
}

func TestRecordPlay_CompletionRate_ZeroDuration(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	h.recordPlay(t, "h1", time.Now().Unix(), 60, 0)

	if got := playHistoryCompletionRate(t, h.d, "h1"); got != 0.0 {
		t.Errorf("completion_rate = %f, want 0.0 for zero duration", got)
	}
}

func TestRecordPlay_CompletionRate_CapsAt1(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	h.recordPlay(t, "h1", time.Now().Unix(), 180, 120.0)

	if got := playHistoryCompletionRate(t, h.d, "h1"); got != 1.0 {
		t.Errorf("completion_rate = %f, want 1.0 (capped)", got)
	}
}

func TestRecordPlay_CompletionRate_FloorsAtZero(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	// A negative listenedSecs must not produce a negative completion_rate,
	// which would drag down AVG() in deep-cuts / rates queries.
	h.recordPlay(t, "h1", time.Now().Unix(), -10, 120.0)

	if got := playHistoryCompletionRate(t, h.d, "h1"); got != 0.0 {
		t.Errorf("completion_rate = %f, want 0.0 for negative listenedSecs", got)
	}
}

func TestRecordPlay_ListenedSecs_FloorsAtZero(t *testing.T) {
	h := newPlayHistoryHarness(t)
	h.addTrack(t, "h1", "/a.flac")

	// A negative listenedSecs must also be clamped before insertion so it
	// does not corrupt SUM(listened_secs) aggregates used in stats and
	// top-N queries.
	h.recordPlay(t, "h1", time.Now().Unix(), -10, 120.0)

	if got := playHistoryListenedSecs(t, h.d, "h1"); got != 0 {
		t.Errorf("listened_secs = %d, want 0 for negative input", got)
	}

	h.recordPlay(t, "h1", time.Now().Unix(), 60, 120.0)
	stats := h.readListeningStats(t, 0)
	if stats.TotalListenedSecs != 60 {
		t.Errorf("TotalListenedSecs = %d, want 60 (negative play floored to 0)", stats.TotalListenedSecs)
	}
}
