package db_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestRecordListen_Uncounted_LeavesTrackStatsAlone verifies that an uncounted
// listen is logged without touching play_count or last_played_at, which drive
// the unheard check, the Never Played mix and smart playlists.
func TestRecordListen_Uncounted_LeavesTrackStatsAlone(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: now, ListenedSecs: 4, DurationSecs: 180,
	}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}

	if got := trackPlayCount(t, d, "h1"); got != 0 {
		t.Errorf("play_count = %d, want 0", got)
	}
	// tracks.last_played_at is NOT NULL DEFAULT 0, so 0 is the never-played state.
	if got := trackLastPlayedAt(t, d, "h1"); got != 0 {
		t.Errorf("last_played_at = %d, want 0", got)
	}

	var counted int
	if err := d.QueryRow(`SELECT counted FROM play_events WHERE track_hash = ?`, "h1").Scan(&counted); err != nil {
		t.Fatalf("query counted: %v", err)
	}
	if counted != 0 {
		t.Errorf("counted = %d, want 0", counted)
	}
}

// TestRecordListen_Uncounted_PreservesEarlierPlayStats verifies that an
// uncounted listen does not rewind or advance stats set by an earlier play.
func TestRecordListen_Uncounted_PreservesEarlierPlayStats(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: now, ListenedSecs: 120, DurationSecs: 180, Counted: true,
	}); err != nil {
		t.Fatalf("RecordListen counted: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: now + 60, ListenedSecs: 4, DurationSecs: 180,
	}); err != nil {
		t.Fatalf("RecordListen uncounted: %v", err)
	}

	if got := trackPlayCount(t, d, "h1"); got != 1 {
		t.Errorf("play_count = %d, want 1", got)
	}
	if got := trackLastPlayedAt(t, d, "h1"); got != now {
		t.Errorf("last_played_at = %d, want %d", got, now)
	}
}

// TestRecordListen_Uncounted_NoLiveSession_CreatesNoSession verifies that a
// skip cannot open a listening session: a one-row session would corrupt the
// ReadInsightSessions averages.
func TestRecordListen_Uncounted_NoLiveSession_CreatesNoSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: time.Now().Unix(), ListenedSecs: 4, DurationSecs: 180,
	}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}

	var sessionID sql.NullString
	if err := d.QueryRow(`SELECT session_id FROM play_events WHERE track_hash = ?`, "h1").Scan(&sessionID); err != nil {
		t.Fatalf("query session_id: %v", err)
	}
	if sessionID.Valid {
		t.Errorf("session_id = %q, want NULL", sessionID.String)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM listening_sessions`).Scan(&count); err != nil {
		t.Fatalf("query listening_sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("listening_sessions rows = %d, want 0", count)
	}
}

// TestRecordListen_Uncounted_ExtendsLiveSession verifies that a skip during an
// open session extends its ended_at without creating a second session.
func TestRecordListen_Uncounted_ExtendsLiveSession(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: now, ListenedSecs: 120, DurationSecs: 180, Counted: true,
	}); err != nil {
		t.Fatalf("RecordListen counted: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h2", PlayedAt: now + 300, ListenedSecs: 4, DurationSecs: 180,
	}); err != nil {
		t.Fatalf("RecordListen uncounted: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM listening_sessions`).Scan(&count); err != nil {
		t.Fatalf("query listening_sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("listening_sessions rows = %d, want 1", count)
	}

	var endedAt int64
	if err := d.QueryRow(`SELECT ended_at FROM listening_sessions`).Scan(&endedAt); err != nil {
		t.Fatalf("query ended_at: %v", err)
	}
	if endedAt != now+300 {
		t.Errorf("ended_at = %d, want %d", endedAt, now+300)
	}

	var sessionID sql.NullString
	if err := d.QueryRow(`SELECT session_id FROM play_events WHERE track_hash = ?`, "h2").Scan(&sessionID); err != nil {
		t.Fatalf("query session_id: %v", err)
	}
	if !sessionID.Valid {
		t.Error("session_id is NULL, want the live session")
	}
}

// TestRecordListen_Uncounted_DoesNotAdvanceLastPlayedAt verifies that a skip
// far in the future does not move the writer's session clock: the following
// counted play must still land in the original session.
func TestRecordListen_Uncounted_DoesNotAdvanceLastPlayedAt(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	insertTrack(t, d, "h3", "/c.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: now, ListenedSecs: 120, DurationSecs: 180, Counted: true,
	}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	// Beyond the 1800s session gap, so this skip finds no live session.
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h2", PlayedAt: now + 3600, ListenedSecs: 4, DurationSecs: 180,
	}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h3", PlayedAt: now + 60, ListenedSecs: 120, DurationSecs: 180, Counted: true,
	}); err != nil {
		t.Fatalf("RecordListen h3: %v", err)
	}

	var sid1, sid3 string
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h1").Scan(&sid1); err != nil {
		t.Fatalf("query h1 session_id: %v", err)
	}
	if err := d.QueryRow(`SELECT session_id FROM play_history WHERE track_hash = ?`, "h3").Scan(&sid3); err != nil {
		t.Fatalf("query h3 session_id: %v", err)
	}
	if sid1 != sid3 {
		t.Errorf("session IDs differ (%q vs %q), want the skip to leave the session clock alone", sid1, sid3)
	}
}

// TestPlayEvents_CountedDefaultsToOne proves the 022 backfill: rows written
// without an explicit counted value stay visible through the play_history view.
func TestPlayEvents_CountedDefaultsToOne(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")

	if _, err := d.Exec(
		`INSERT INTO play_events (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
		"h1", time.Now().Unix(), 60,
	); err != nil {
		t.Fatalf("insert play_events: %v", err)
	}

	var counted int
	if err := d.QueryRow(`SELECT counted FROM play_events WHERE track_hash = ?`, "h1").Scan(&counted); err != nil {
		t.Fatalf("query counted: %v", err)
	}
	if counted != 1 {
		t.Errorf("counted = %d, want 1 (column default)", counted)
	}
}

// TestPlayHistoryView_ExcludesUncountedRows verifies the view preserves today's
// read semantics for the ~34 existing play_history consumers.
func TestPlayHistoryView_ExcludesUncountedRows(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h1", PlayedAt: now, ListenedSecs: 120, DurationSecs: 180, Counted: true,
	}); err != nil {
		t.Fatalf("RecordListen counted: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{
		Hash: "h2", PlayedAt: now + 60, ListenedSecs: 4, DurationSecs: 180,
	}); err != nil {
		t.Fatalf("RecordListen uncounted: %v", err)
	}

	var events, history int
	if err := d.QueryRow(`SELECT COUNT(*) FROM play_events`).Scan(&events); err != nil {
		t.Fatalf("query play_events: %v", err)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM play_history`).Scan(&history); err != nil {
		t.Fatalf("query play_history: %v", err)
	}
	if events != 2 {
		t.Errorf("play_events rows = %d, want 2", events)
	}
	if history != 1 {
		t.Errorf("play_history rows = %d, want 1 (uncounted row must be hidden)", history)
	}

	var hash string
	if err := d.QueryRow(`SELECT track_hash FROM play_history`).Scan(&hash); err != nil {
		t.Fatalf("query play_history track_hash: %v", err)
	}
	if hash != "h1" {
		t.Errorf("play_history track_hash = %q, want %q", hash, "h1")
	}
}
