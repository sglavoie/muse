package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
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

// BatchUpserter persists scanned tracks in bulk.
type BatchUpserter interface {
	UpsertBatch(tracks []library.Track) error
}

var _ SessionWriter = (*Writer)(nil)
var _ QueueDequeuer = (*Writer)(nil)
var _ FavoriteWriter = (*Writer)(nil)
var _ BatchUpserter = (*Writer)(nil)

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
	go w.run(ctx)
	return w
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
// listenedSecs is the actual number of seconds listened.
func (w *Writer) RecordPlay(hash string, playedAt int64, listenedSecs int) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		if _, err := tx.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			hash, playedAt, listenedSecs,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE tracks SET play_count = play_count + 1, last_played_at = ? WHERE hash = ?`,
			playedAt, hash,
		); err != nil {
			return err
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
// the manually edited metadata.
func (w *Writer) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET title=?, artist=?, album=?, genre=?, user_edited=1, updated_at=? WHERE hash=?`,
			title, artist, album, genre, time.Now().Unix(), hash,
		)
		return err
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
				INSERT INTO tracks (hash, path, title, artist, album, genre, duration_ms, size_bytes, mod_time, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(hash) DO UPDATE SET
					path        = excluded.path,
					title       = CASE WHEN tracks.user_edited = 0 THEN excluded.title ELSE tracks.title END,
					artist      = CASE WHEN tracks.user_edited = 0 THEN excluded.artist ELSE tracks.artist END,
					album       = CASE WHEN tracks.user_edited = 0 THEN excluded.album ELSE tracks.album END,
					genre       = CASE WHEN tracks.user_edited = 0 THEN excluded.genre ELSE tracks.genre END,
					duration_ms = CASE WHEN excluded.duration_ms > 0 THEN excluded.duration_ms ELSE tracks.duration_ms END,
					size_bytes  = excluded.size_bytes,
					mod_time    = excluded.mod_time,
					missing            = 0,
					duration_verified  = 0,
					updated_at         = excluded.updated_at`,
				t.Hash, t.Path, t.Title, t.Artist, t.Album, t.Genre, durationMs, t.SizeBytes, t.ModTime, createdAt, now,
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
// receive an error immediately.
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
	slog.Warn("db writer drain timed out, some writes may be lost")
}
