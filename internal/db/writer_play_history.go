package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math"
)

// IncrementPlayCount increments play_count by 1 for the track identified by
// hash. It is a no-op if no row matches.
func (w *Writer) IncrementPlayCount(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET play_count = play_count + 1 WHERE hash = ?`,
			hash,
		)
		return err
	})
}

// PlayRecord describes one listen to persist. PlayedAt is a Unix timestamp;
// ListenedSecs is the actual number of seconds listened; DurationSecs is the
// total track duration in seconds (used to compute completion_rate).
//
// Counted marks whether the listen was substantial enough to count as a play.
// Uncounted listens are still logged so the completion distribution contains
// negative examples, but they must not touch tracks.play_count or
// tracks.last_played_at: those columns drive the unheard check, the Never
// Played mix, the Rediscover/HiddenGems/DeepCuts ranges and smart playlists.
type PlayRecord struct {
	Hash         string
	PlayedAt     int64
	ListenedSecs int
	DurationSecs float64
	Counted      bool
}

// RecordListen inserts a play_events row for rec. When rec.Counted is true it
// also atomically increments play_count and sets last_played_at on the tracks
// row, and opens or extends a listening session.
//
// Negative ListenedSecs values (e.g. from a clock skew between elapsed-time
// samples) are clamped to 0 so they do not corrupt SUM(listened_secs)
// aggregates used in stats and top-N queries.
func (w *Writer) RecordListen(rec PlayRecord) error {
	listenedSecs := rec.ListenedSecs
	if listenedSecs < 0 {
		listenedSecs = 0
	}
	completionRate := 0.0
	if rec.DurationSecs > 0 && listenedSecs > 0 {
		completionRate = math.Min(1.0, float64(listenedSecs)/rec.DurationSecs)
	}
	counted := 0
	if rec.Counted {
		counted = 1
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		// Determine session ID. Runs inside the writer goroutine, so
		// w.lastSessionID and w.lastPlayedAt are safe to read/write.
		sessionID := w.lastSessionID
		sessionLive := sessionID != "" && !timeGapExceeds(rec.PlayedAt, w.lastPlayedAt, sessionGapSecs)
		switch {
		case sessionLive:
			// Clamp with MAX so a backwards clock (NTP correction, manual
			// adjustment) cannot rewind a session's ended_at below its
			// existing value.
			if _, err := tx.Exec(
				`UPDATE listening_sessions SET ended_at = MAX(ended_at, ?) WHERE id = ?`,
				rec.PlayedAt, sessionID,
			); err != nil {
				return err
			}
		case rec.Counted:
			var randBuf [4]byte
			if _, err := rand.Read(randBuf[:]); err != nil {
				return fmt.Errorf("session rand: %w", err)
			}
			sessionID = fmt.Sprintf("%d-%x", rec.PlayedAt, randBuf)
			if _, err := tx.Exec(
				`INSERT INTO listening_sessions (id, started_at, ended_at) VALUES (?, ?, ?)`,
				sessionID, rec.PlayedAt, rec.PlayedAt,
			); err != nil {
				return err
			}
		default:
			// An uncounted listen extends a live session but never opens one:
			// a four-second skip would otherwise manufacture a one-row session
			// that corrupts ReadInsightSessions averages.
			sessionID = ""
		}

		var sessionCol any
		if sessionID != "" {
			sessionCol = sessionID
		}
		if _, err := tx.Exec(
			`INSERT INTO play_events (track_hash, played_at, listened_secs, completion_rate, session_id, counted) VALUES (?, ?, ?, ?, ?, ?)`,
			rec.Hash, rec.PlayedAt, listenedSecs, completionRate, sessionCol, counted,
		); err != nil {
			return err
		}
		if rec.Counted {
			if _, err := tx.Exec(
				`UPDATE tracks SET play_count = play_count + 1, last_played_at = MAX(COALESCE(last_played_at, 0), ?) WHERE hash = ?`,
				rec.PlayedAt, rec.Hash,
			); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if rec.Counted {
			w.lastSessionID = sessionID
			if rec.PlayedAt > w.lastPlayedAt {
				w.lastPlayedAt = rec.PlayedAt
			}
		}
		return nil
	})
}

func timeGapExceeds(a, b, threshold int64) bool {
	if threshold < 0 {
		return true
	}
	const maxInt64 int64 = 1<<63 - 1
	if a >= b {
		if b < 0 && a > maxInt64+b {
			return true
		}
		return a-b > threshold
	}
	if a < 0 && b > maxInt64+a {
		return true
	}
	return b-a > threshold
}
