package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
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
	AddFavoritesByTypeAndValues(typ string, values []string) error
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
	ctx  context.Context
	db   *sql.DB
	cmds chan writeCmd
	done chan struct{}
	mu   sync.RWMutex

	stopping bool

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
		ctx:  ctx,
		db:   db,
		cmds: make(chan writeCmd, writerChannelBuffer),
		done: make(chan struct{}),
	}
	w.recoverPendingQueueEntries()
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
	case <-w.ctx.Done():
		return fmt.Errorf("db writer: stopped")
	default:
	}

	w.mu.RLock()
	if w.stopping {
		w.mu.RUnlock()
		return fmt.Errorf("db writer: stopped")
	}
	select {
	case w.cmds <- cmd:
		w.mu.RUnlock()
	case <-w.ctx.Done():
		w.mu.RUnlock()
		return fmt.Errorf("db writer: stopped")
	case <-w.done:
		w.mu.RUnlock()
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
			w.stopAccepting()
			w.drain()
			return
		}
	}
}

func (w *Writer) stopAccepting() {
	w.mu.Lock()
	w.stopping = true
	w.mu.Unlock()
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
