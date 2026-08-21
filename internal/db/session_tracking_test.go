package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestRecordListen_SameSession verifies that two plays within the 30-minute gap
// threshold share the same session_id.
func TestRecordListen_SameSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	// 60s apart — well within the 1800s gap.
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 60, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	var sid1, sid2 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1 session_id: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h2").Scan(&sid2); err != nil {
		t.Fatalf("query h2 session_id: %v", err)
	}

	if sid1 == "" {
		t.Error("session_id is empty for h1")
	}
	if sid1 != sid2 {
		t.Errorf("session IDs differ (%q vs %q), want same session (60s gap)", sid1, sid2)
	}
}

// TestRecordListen_DifferentSession verifies that two plays separated by more
// than 30 minutes receive distinct session_ids.
func TestRecordListen_DifferentSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	// 1980s = 33 minutes — exceeds the 1800s threshold.
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 1980, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	var sid1, sid2 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1 session_id: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h2").Scan(&sid2); err != nil {
		t.Fatalf("query h2 session_id: %v", err)
	}

	if sid1 == "" || sid2 == "" {
		t.Error("session_id is empty")
	}
	if sid1 == sid2 {
		t.Errorf("session IDs are the same (%q), want different sessions (33 min gap)", sid1)
	}
}

// TestRecordListen_BackwardGapStartsNewSession verifies that session boundaries
// use the absolute time gap between consecutive recorded plays. Before the fix,
// a large backwards clock jump was folded into the current session because only
// forward gaps were compared to the threshold.
func TestRecordListen_BackwardGapStartsNewSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now - 2000, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	var sid1, sid2 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1 session_id: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h2").Scan(&sid2); err != nil {
		t.Fatalf("query h2 session_id: %v", err)
	}

	if sid1 == "" || sid2 == "" {
		t.Error("session_id is empty")
	}
	if sid1 == sid2 {
		t.Errorf("session IDs are the same (%q), want different sessions (backward 2000s gap)", sid1)
	}
}

func TestRecordListen_ExtremeBackwardGapStartsNewSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: maxInt64, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: minInt64, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	var sid1, sid2 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1 session_id: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h2").Scan(&sid2); err != nil {
		t.Fatalf("query h2 session_id: %v", err)
	}

	if sid1 == "" || sid2 == "" {
		t.Error("session_id is empty")
	}
	if sid1 == sid2 {
		t.Errorf("session IDs are the same (%q), want different sessions (extreme backward gap)", sid1)
	}
}

// TestWriter_InitSessionState_ContinuesSameSession verifies that a new Writer
// restores lastSessionID from the DB and continues the same session when the
// gap since the last play is within the threshold.
func TestWriter_InitSessionState_ContinuesSameSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")

	now := time.Now().Unix()

	ctx1, cancel1 := context.WithCancel(t.Context())
	w1 := db.NewWriter(ctx1, d.DB)
	if err := w1.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("w1 RecordListen: %v", err)
	}
	cancel1()
	w1.Wait()

	// Second Writer — initSessionState restores state from DB.
	// 60s after the last play: within the 1800s gap → same session.
	w2 := db.NewWriter(t.Context(), d.DB)
	t.Cleanup(w2.Wait)
	if err := w2.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 60, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("w2 RecordListen: %v", err)
	}

	var sid1, sid2 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h2").Scan(&sid2); err != nil {
		t.Fatalf("query h2: %v", err)
	}

	if sid1 != sid2 {
		t.Errorf("session IDs differ after restart (%q vs %q), want same session (60s gap)", sid1, sid2)
	}
}

// TestWriter_InitSessionState_NewSessionAfterGap verifies that a new Writer
// starts a fresh session when the gap since the last play exceeds the threshold.
func TestWriter_InitSessionState_NewSessionAfterGap(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")

	now := time.Now().Unix()

	ctx1, cancel1 := context.WithCancel(t.Context())
	w1 := db.NewWriter(ctx1, d.DB)
	if err := w1.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("w1 RecordListen: %v", err)
	}
	cancel1()
	w1.Wait()

	// 2000s later — exceeds the 1800s gap → new session.
	w2 := db.NewWriter(t.Context(), d.DB)
	t.Cleanup(w2.Wait)
	if err := w2.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 2000, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("w2 RecordListen: %v", err)
	}

	var sid1, sid2 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h2").Scan(&sid2); err != nil {
		t.Fatalf("query h2: %v", err)
	}

	if sid1 == sid2 {
		t.Errorf("session IDs are the same (%q), want different sessions (2000s gap)", sid1)
	}
}

// TestListeningSessions_RowCreated verifies that RecordListen inserts a row into
// listening_sessions for a new session.
func TestListeningSessions_RowCreated(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: time.Now().Unix(), ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM listening_sessions`).Scan(&count); err != nil {
		t.Fatalf("query listening_sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("listening_sessions rows = %d, want 1", count)
	}
}

// TestListeningSessions_EndedAtUpdated verifies that the second play in the
// same session updates ended_at on the listening_sessions row.
func TestListeningSessions_EndedAtUpdated(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 300, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	var startedAt, endedAt int64
	if err := d.QueryRow(`SELECT started_at, ended_at FROM listening_sessions`).Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query listening_sessions: %v", err)
	}
	if startedAt != now {
		t.Errorf("started_at = %d, want %d", startedAt, now)
	}
	if endedAt != now+300 {
		t.Errorf("ended_at = %d, want %d", endedAt, now+300)
	}
}

// TestListeningSessions_EndedAtNotRewoundByClockSkew verifies that a backwards
// system clock cannot pull ended_at (or tracks.last_played_at) earlier than
// values already persisted for the same session.
func TestListeningSessions_EndedAtNotRewoundByClockSkew(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + 300, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	// Clock moves backwards 60s but still within the session gap.
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 240, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	var endedAt int64
	if err := d.QueryRow(`SELECT ended_at FROM listening_sessions`).Scan(&endedAt); err != nil {
		t.Fatalf("query listening_sessions: %v", err)
	}
	if endedAt < now+300 {
		t.Errorf("ended_at = %d, want >= %d (must not rewind on backwards clock)", endedAt, now+300)
	}

	var lastPlayed int64
	if err := d.QueryRow(`SELECT last_played_at FROM tracks WHERE hash = ?`, "h1").Scan(&lastPlayed); err != nil {
		t.Fatalf("query tracks h1: %v", err)
	}
	if lastPlayed != now+300 {
		t.Errorf("last_played_at for h1 = %d, want %d (must not rewind on backwards clock)", lastPlayed, now+300)
	}
}
