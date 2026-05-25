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

// RecordPlay inserts a play_history row and atomically increments play_count
// and sets last_played_at on the tracks row. playedAt is a Unix timestamp;
// listenedSecs is the actual number of seconds listened; durationSecs is the
// total track duration in seconds (used to compute completion_rate).
//
// Negative listenedSecs values (e.g. from a clock skew between elapsed-time
// samples) are clamped to 0 so they do not corrupt SUM(listened_secs)
// aggregates used in stats and top-N queries.
func (w *Writer) RecordPlay(hash string, playedAt int64, listenedSecs int, durationSecs float64) error {
	if listenedSecs < 0 {
		listenedSecs = 0
	}
	completionRate := 0.0
	if durationSecs > 0 && listenedSecs > 0 {
		completionRate = math.Min(1.0, float64(listenedSecs)/durationSecs)
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
		if sessionID == "" || timeGapExceeds(playedAt, w.lastPlayedAt, sessionGapSecs) {
			var randBuf [4]byte
			if _, err := rand.Read(randBuf[:]); err != nil {
				return fmt.Errorf("session rand: %w", err)
			}
			sessionID = fmt.Sprintf("%d-%x", playedAt, randBuf)
			if _, err := tx.Exec(
				`INSERT INTO listening_sessions (id, started_at, ended_at) VALUES (?, ?, ?)`,
				sessionID, playedAt, playedAt,
			); err != nil {
				return err
			}
		} else {
			// Clamp with MAX so a backwards clock (NTP correction, manual
			// adjustment) cannot rewind a session's ended_at below its
			// existing value.
			if _, err := tx.Exec(
				`UPDATE listening_sessions SET ended_at = MAX(ended_at, ?) WHERE id = ?`,
				playedAt, sessionID,
			); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs, completion_rate, session_id) VALUES (?, ?, ?, ?, ?)`,
			hash, playedAt, listenedSecs, completionRate, sessionID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE tracks SET play_count = play_count + 1, last_played_at = MAX(COALESCE(last_played_at, 0), ?) WHERE hash = ?`,
			playedAt, hash,
		); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		w.lastSessionID = sessionID
		if playedAt > w.lastPlayedAt {
			w.lastPlayedAt = playedAt
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
