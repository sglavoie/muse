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

// Compile-time check: *Writer must satisfy the sessionWriter interface
// used by the TUI layer (WriteRepeatMode, WriteShuffle, UpdateSessionPosition,
// WriteVolume). This catches method-signature drift without requiring a runtime
// assignment.
var _ interface {
	WriteRepeatMode(mode string) error
	WriteShuffle(enabled bool) error
	UpdateSessionPosition(hash string, positionMs int64) error
	WriteVolume(volume int) error
} = (*Writer)(nil)

// Compile-time check: *Writer must implement UpsertBatch so that the
// scan-to-DB bridge in main.go and any future consumer cannot diverge from
// the actual method signature without a build failure.
var _ interface {
	UpsertBatch(tracks []library.Track) error
} = (*Writer)(nil)

// Compile-time check: *Writer must satisfy the queueDequeuer interface used
// by the TUI layer (DequeueTrackSoft, UndoDequeue, ClearQueueSoft,
// UndoClearQueue, PurgeDeletedQueueEntries). Mirrors the sessionWriter and
// UpsertBatch assertions above to catch method-signature drift at build time.
var _ interface {
	DequeueTrackSoft(id int64) error
	UndoDequeue(id int64) error
	ClearQueueSoft() ([]int64, error)
	UndoClearQueue(ids []int64) error
	PurgeDeletedQueueEntries(before int64) error
} = (*Writer)(nil)

// Compile-time check: *Writer must satisfy the favoriteWriter interface used
// by the TUI layer (AddFavorite, RemoveFavorite, RemoveFavoritesByTypeAndValues).
var _ interface {
	AddFavorite(typ, value string) error
	RemoveFavorite(typ, value string) error
	RemoveFavoritesByTypeAndValues(typ string, values []string) error
} = (*Writer)(nil)

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

// UpdateTrackMetadata updates the title, artist, and album for the track
// identified by hash and sets user_edited=1 so that future rescans preserve
// the manually edited metadata.
func (w *Writer) UpdateTrackMetadata(hash, title, artist, album string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET title=?, artist=?, album=?, user_edited=1, updated_at=? WHERE hash=?`,
			title, artist, album, time.Now().Unix(), hash,
		)
		return err
	})
}

// UpsertBatch writes tracks into the tracks table inside a single transaction
// using an INSERT … ON CONFLICT(hash) DO UPDATE so that existing rows are
// updated in place without cascading deletes on their FK dependants (queue,
// playlist_tracks). created_at is preserved on conflict; all other fields are
// updated to the values supplied by the scanner. Duration is converted from
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
			_, err := tx.Exec(`
				INSERT INTO tracks (hash, path, title, artist, album, duration_ms, size_bytes, mod_time, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(hash) DO UPDATE SET
					path        = excluded.path,
					title       = CASE WHEN tracks.user_edited = 0 THEN excluded.title ELSE tracks.title END,
					artist      = CASE WHEN tracks.user_edited = 0 THEN excluded.artist ELSE tracks.artist END,
					album       = CASE WHEN tracks.user_edited = 0 THEN excluded.album ELSE tracks.album END,
					duration_ms = excluded.duration_ms,
					size_bytes  = excluded.size_bytes,
					mod_time    = excluded.mod_time,
					missing     = 0,
					updated_at  = excluded.updated_at`,
				t.Hash, t.Path, t.Title, t.Artist, t.Album, durationMs, t.SizeBytes, t.ModTime, now, now,
			)
			if err != nil {
				return err
			}
		}
		return tx.Commit()
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
