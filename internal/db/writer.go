package db

import (
	"context"
	"database/sql"
	"fmt"
)

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
		cmds: make(chan writeCmd, 64),
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
		return fmt.Errorf("db writer: stopped while waiting for result")
	}
}

// Wait blocks until the writer goroutine has fully exited. Call this after
// cancelling the context passed to NewWriter to ensure a clean shutdown.
func (w *Writer) Wait() {
	<-w.done
}

// run is the single writer goroutine. It drains cmds until ctx is cancelled,
// then exits and closes done.
func (w *Writer) run(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case cmd := <-w.cmds:
			cmd.resp <- cmd.fn(w.db)
		case <-ctx.Done():
			return
		}
	}
}
