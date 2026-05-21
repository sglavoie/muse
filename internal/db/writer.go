package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const (
	// writerDrainTimeout is how long Close waits for in-flight writes to
	// finish before giving up. Keep it short so the app exits promptly.
	writerDrainTimeout = 5 * time.Second

	// writerChannelBuffer is the capacity of the async command channel.
	// 64 is large enough to absorb burst writes (e.g. batch enqueue)
	// without blocking the TUI goroutine.
	writerChannelBuffer = 64

	// queuePositionGap is the integer gap between consecutive queue
	// positions. Using 1000 instead of 1 allows single-row reorders
	// (move up/down) to be expressed as simple position swaps without
	// renumbering the entire queue.
	queuePositionGap = 1000

	// sessionGapSecs is the inactivity threshold used to detect a new
	// listening session. Plays separated by more than this many seconds
	// are assigned a fresh session ID.
	sessionGapSecs = 1800
)

// SessionWriter persists session state (repeat mode, shuffle, position, volume).
type SessionWriter interface {
	WriteRepeatMode(mode string) error
	WriteShuffle(enabled bool) error
	WriteRadioMode(enabled bool) error
	UpdateSessionPosition(hash string, positionMs int64) error
	WriteVolume(volume int) error
}

// QueueDequeuer handles soft-delete, undo, and purge for queue entries.
type QueueDequeuer interface {
	DequeueTrackSoft(id int64) error
	UndoDequeue(id int64) error
	ClearQueueSoft() ([]int64, error)
	UndoClearQueue(ids []int64) error
	PurgeDeletedQueueEntries(before int64) error
}

// FavoriteWriter manages favorite entries.
type FavoriteWriter interface {
	AddFavorite(typ, value string) error
	RemoveFavorite(typ, value string) error
	RemoveFavoritesByTypeAndValues(typ string, values []string) error
}

// DismissalWriter manages recommendation dismissal entries.
type DismissalWriter interface {
	DismissTrack(hash string) error
	UndismissTrack(hash string) error
}

var _ SessionWriter = (*Writer)(nil)
var _ QueueDequeuer = (*Writer)(nil)
var _ FavoriteWriter = (*Writer)(nil)
var _ DismissalWriter = (*Writer)(nil)

// writeCmd is a single unit of work sent to the writer goroutine.
// The caller blocks on resp until the goroutine sends the result back.
type writeCmd struct {
	fn   func(*sql.DB) error
	resp chan error
}

// Writer serializes all DB mutations through a single goroutine, eliminating
// SQLITE_BUSY contention. Callers submit work via Exec and block on a
// per-request response channel until the goroutine completes the write.
type Writer struct {
	db   *sql.DB
	cmds chan writeCmd
	done chan struct{}

	// Session tracking state. Only read/written inside Exec closures (writer
	// goroutine) or before the goroutine starts (initSessionState). No mutex
	// required.
	lastSessionID string
	lastPlayedAt  int64
}

// NewWriter creates a Writer and starts its background goroutine. The goroutine
// runs until ctx is cancelled. The caller should call Wait after cancelling ctx
// to ensure a clean shutdown.
func NewWriter(ctx context.Context, db *sql.DB) *Writer {
	w := &Writer{
		db:   db,
		cmds: make(chan writeCmd, writerChannelBuffer),
		done: make(chan struct{}),
	}
	w.initSessionState()
	go w.run(ctx)
	return w
}

// initSessionState queries the most recent play_history row to restore
// lastSessionID and lastPlayedAt after a restart. Called once before the
// writer goroutine starts, so no locking is needed.
func (w *Writer) initSessionState() {
	var sessionID sql.NullString
	var playedAt int64
	row := w.db.QueryRow(
		`SELECT session_id, played_at FROM play_history
		 WHERE session_id IS NOT NULL
		 ORDER BY played_at DESC LIMIT 1`,
	)
	if err := row.Scan(&sessionID, &playedAt); err != nil {
		return // no rows or migration not yet applied — start fresh
	}
	if sessionID.Valid {
		w.lastSessionID = sessionID.String
		w.lastPlayedAt = playedAt
	}
}

// Exec submits fn to the writer goroutine and blocks until it completes.
// fn receives the underlying *sql.DB and should perform exactly one logical
// write (INSERT, UPDATE, DELETE, or a transaction wrapping several).
// Returns an error if fn fails or if the writer has already stopped.
func (w *Writer) Exec(fn func(*sql.DB) error) error {
	resp := make(chan error, 1)
	cmd := writeCmd{fn: fn, resp: resp}

	select {
	case w.cmds <- cmd:
	case <-w.done:
		return fmt.Errorf("db writer: stopped")
	}

	select {
	case err := <-resp:
		return err
	case <-w.done:
		// The writer goroutine closed done after drain(). If drain
		// processed our command, resp already has the result. Check it
		// with a non-blocking receive to avoid a spurious "stopped"
		// error when the write actually succeeded.
		select {
		case err := <-resp:
			return err
		default:
			return fmt.Errorf("db writer: stopped while waiting for result")
		}
	}
}

// Wait blocks until the writer goroutine has fully exited. Call this after
// cancelling the context passed to NewWriter to ensure a clean shutdown.
func (w *Writer) Wait() {
	<-w.done
}

// run is the single writer goroutine. It drains cmds until ctx is cancelled,
// then flushes any buffered commands before closing done.
func (w *Writer) run(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case cmd := <-w.cmds:
			cmd.resp <- cmd.fn(w.db)
		case <-ctx.Done():
			w.drain()
			return
		}
	}
}

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
		if sessionID == "" || playedAt-w.lastPlayedAt > sessionGapSecs {
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

// PopulateTrackGenres refreshes (track_hash, genre) rows in track_genres for
// each track. Stale rows for the same hash are deleted first so that genre
// changes (rescan after the file's metadata was edited externally) are not
// shadowed by leftover rows; this keeps ReadTopGenres, ReadHiddenGemsMix,
// ReadNeverPlayedMix and ReadHourGenreAffinities consistent with the canonical
// tracks.genre value. Tracks whose hash is empty are skipped (no junction row
// can reference them via FK).
func (w *Writer) PopulateTrackGenres(tracks []library.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		for _, t := range tracks {
			if t.Hash == "" {
				continue
			}
			if _, err := tx.Exec(
				`DELETE FROM track_genres WHERE track_hash = ?`,
				t.Hash,
			); err != nil {
				return err
			}
			genres := library.SplitGenres(t.Genre)
			for _, g := range genres {
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`,
					t.Hash, g,
				); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

// MarkTrackMissing sets missing=1 on the tracks row identified by hash and
// updates its updated_at timestamp. It is a no-op if no row matches.
func (w *Writer) MarkTrackMissing(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET missing=1, updated_at=? WHERE hash=?`,
			time.Now().Unix(), hash,
		)
		return err
	})
}

// ClearTrackMissing sets missing=0 on the tracks row identified by hash and
// updates its updated_at timestamp. Call this during rescan when a file with a
// matching BLAKE3 hash is located again.
func (w *Writer) ClearTrackMissing(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET missing=0, updated_at=? WHERE hash=?`,
			time.Now().Unix(), hash,
		)
		return err
	})
}

// UpdateTrackMetadata updates the title, artist, album, and genre for the track
// identified by hash and sets user_edited=1 so that future rescans preserve
// the manually edited metadata. The track_genres junction table is refreshed
// in the same transaction so that genre-driven recommendation queries
// (ReadTopGenres, ReadHiddenGemsMix, ReadNeverPlayedMix,
// ReadHourGenreAffinities) reflect the edit immediately instead of mixing the
// edited genre with stale entries from before the edit.
func (w *Writer) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if _, err := tx.Exec(
			`UPDATE tracks SET title=?, artist=?, album=?, genre=?, user_edited=1, updated_at=? WHERE hash=?`,
			title, artist, album, genre, time.Now().Unix(), hash,
		); err != nil {
			return err
		}
		if hash != "" {
			if _, err := tx.Exec(
				`DELETE FROM track_genres WHERE track_hash = ?`, hash,
			); err != nil {
				return err
			}
			for _, g := range library.SplitGenres(genre) {
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`,
					hash, g,
				); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

// UpdateTrackDuration updates the duration_ms for the track identified by hash.
// It is a no-op if no row matches.
func (w *Writer) UpdateTrackDuration(hash string, durationMs int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET duration_ms=?, updated_at=? WHERE hash=?`,
			durationMs, time.Now().Unix(), hash,
		)
		return err
	})
}

// UpsertBatch writes tracks into the tracks table inside a single transaction
// using an INSERT … ON CONFLICT(hash) DO UPDATE so that existing rows are
// updated in place without cascading deletes on their FK dependants (queue,
// playlist_tracks). created_at is preserved on conflict; all other fields are
// updated to the values supplied by the scanner. Duration is preserved when
// the scanner returns 0 (e.g. tag-less files). Duration is converted from
// seconds (float64) to milliseconds (int64).
// UpsertBatch is a no-op when tracks is empty.
func (w *Writer) UpsertBatch(tracks []library.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		now := time.Now().Unix()
		for _, t := range tracks {
			durationMs := int64(t.Duration * 1000)
			createdAt := t.CreatedAt
			if createdAt == 0 {
				createdAt = now
			}
			_, err := tx.Exec(`
				INSERT INTO tracks (hash, path, title, artist, album, genre, duration_ms, size_bytes, mod_time, created_at, updated_at, track_number, disc_number)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(hash) DO UPDATE SET
					path         = excluded.path,
					title        = CASE WHEN tracks.user_edited = 0 THEN excluded.title ELSE tracks.title END,
					artist       = CASE WHEN tracks.user_edited = 0 THEN excluded.artist ELSE tracks.artist END,
					album        = CASE WHEN tracks.user_edited = 0 THEN excluded.album ELSE tracks.album END,
					genre        = CASE WHEN tracks.user_edited = 0 THEN excluded.genre ELSE tracks.genre END,
					track_number = CASE WHEN tracks.user_edited = 0 THEN excluded.track_number ELSE tracks.track_number END,
					disc_number  = CASE WHEN tracks.user_edited = 0 THEN excluded.disc_number ELSE tracks.disc_number END,
					duration_ms  = CASE WHEN excluded.duration_ms > 0 THEN excluded.duration_ms ELSE tracks.duration_ms END,
					size_bytes   = excluded.size_bytes,
					mod_time     = excluded.mod_time,
					missing            = 0,
					duration_verified  = 0,
					updated_at         = excluded.updated_at`,
				t.Hash, t.Path, t.Title, t.Artist, t.Album, t.Genre, durationMs, t.SizeBytes, t.ModTime, createdAt, now, t.TrackNumber, t.DiscNumber,
			)
			if err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// VerifyTrackDuration updates duration_ms to the given value and sets
// duration_verified=1 for the track identified by hash. Used when ffprobe
// reports a duration that differs materially from the stored value.
func (w *Writer) VerifyTrackDuration(hash string, durationMs int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET duration_ms = ?, duration_verified = 1 WHERE hash = ?`,
			durationMs, hash,
		)
		return err
	})
}

// MarkDurationVerified sets duration_verified=1 for the track identified by
// hash without changing the stored duration. Used when ffprobe confirms the
// existing duration is accurate.
func (w *Writer) MarkDurationVerified(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET duration_verified = 1 WHERE hash = ?`,
			hash,
		)
		return err
	})
}

// rollback attempts to roll back tx, logging unexpected errors. The common
// sql.ErrTxDone (commit already succeeded) is silently ignored.
func rollback(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Warn("tx rollback failed", "err", err)
	}
}

// drain flushes all commands already buffered in the cmds channel after ctx is
// cancelled. A timeout guards against a single stuck write blocking shutdown
// indefinitely. New Exec calls after drain returns will see w.done closed and
// receive an error immediately. The timeout-exceeded warning is only emitted
// when commands remain in the channel; without that check a single long write
// that itself completed successfully (but pushed the loop past the deadline)
// would log a misleading "writes may be lost" warning even though the channel
// was already empty by the time the deadline expired.
func (w *Writer) drain() {
	deadline := time.Now().Add(writerDrainTimeout)
	for time.Now().Before(deadline) {
		select {
		case cmd := <-w.cmds:
			cmd.resp <- cmd.fn(w.db)
		default:
			return
		}
	}
	// Only warn if the deadline expired with at least one pending command.
	if len(w.cmds) > 0 {
		slog.Warn("db writer drain timed out, some writes may be lost", "pending", len(w.cmds))
	}
}
