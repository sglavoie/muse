package app

import (
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/config"
)

// ShutdownTimeout is the maximum time allowed for graceful shutdown before
// the process is forcibly terminated.
const ShutdownTimeout = 10 * time.Second

// ShutdownDeps holds injectable teardown operations for the graceful shutdown
// sequence. A nil ClosePlayer field is treated as a no-op.
// CancelDBWriter and WaitDBWriter should both be set whenever a DB writer was
// started; omitting WaitDBWriter risks missing in-flight writes.
type ShutdownDeps struct {
	// ClosePlayer stops playback and terminates the mpv process.
	// Nil means no-op.
	ClosePlayer func() error

	// StopLibraryPipeline cancels the asynchronous library pipeline (scan and
	// watch bridges plus the orchestrator) and blocks until it has exited,
	// giving pending scan/watch database batches their final bounded flush.
	// It runs after the player is closed but before the DB writer is drained so
	// those final upserts are still enqueued while the writer is alive. Nil
	// means no library pipeline was started.
	StopLibraryPipeline func()

	// CancelDBWriter cancels the context driving the DB writer goroutine,
	// causing it to stop accepting new work.
	CancelDBWriter func()

	// WaitDBWriter blocks until the DB writer goroutine has fully exited,
	// ensuring all in-flight writes complete before the database is closed.
	WaitDBWriter func()

	// CloseDB closes the database connection after the writer has drained.
	// Nil means no-op.
	CloseDB func()

	// CtrlListener is the Unix domain socket listener for remote commands.
	// Closed first during shutdown to stop accepting new commands. Nil means
	// remote control was not started.
	CtrlListener net.Listener

	// Lockfile is the PID lockfile to release on shutdown.
	Lockfile *config.Lockfile

	// SocketPath is the path to the mpv Unix socket to delete.
	SocketPath string

	// CloseLog flushes and closes the log file. Called last so that all
	// shutdown log entries are written before the handle is released.
	CloseLog func()
}

// Shutdown executes the graceful teardown sequence with a timeout. If the
// sequence does not complete within timeout, the process is forcibly exited.
// Pass ShutdownTimeout for the normal production timeout.
//
// The teardown sequence is:
//  0. Close the remote control listener (stop accepting new commands)
//  1. Close the player (stop playback + terminate mpv)
//  2. Stop and flush the library pipeline (final watcher/scan upserts)
//  3. Flush pending DB writes (cancel writer context, wait for goroutine exit)
//  4. Close database connection
//  5. Release PID lockfile
//  6. Delete mpv socket file
//  7. Close log file (last, so all shutdown entries are captured)
//
// All steps are attempted regardless of prior errors so that resources are
// always released. Errors from individual steps are logged at Warn level.
func Shutdown(deps ShutdownDeps, timeout time.Duration) {
	// Guard CloseLog so it runs at most once, whether the normal sequence
	// completes or the timeout path forces exit. Without this, a slow
	// shutdownSequence racing against the timeout could double-close the log
	// (e.g. flushing/closing a file handle twice or racing slog handlers).
	var closeLogOnce sync.Once
	closeLog := func() {
		if deps.CloseLog == nil {
			return
		}
		closeLogOnce.Do(deps.CloseLog)
	}
	guarded := deps
	guarded.CloseLog = closeLog

	done := make(chan struct{})
	go func() {
		defer close(done)
		shutdownSequence(guarded)
	}()
	select {
	case <-done:
		return
	case <-time.After(timeout):
		slog.Error("shutdown timed out, forcing exit", "timeout", timeout)
		closeLog()
		os.Exit(1)
	}
}

// shutdownSequence performs the ordered teardown steps.
func shutdownSequence(deps ShutdownDeps) {
	// Step 0: Close the remote control listener to stop accepting new commands.
	if deps.CtrlListener != nil {
		slog.Info("shutdown: closing control listener")
		if err := deps.CtrlListener.Close(); err != nil {
			slog.Warn("shutdown: close control listener", "err", err)
		}
		slog.Info("shutdown: control listener closed")
	}

	// Step 1: Close the player (stop playback + terminate mpv).
	slog.Info("shutdown: closing player")
	if deps.ClosePlayer != nil {
		if err := deps.ClosePlayer(); err != nil {
			slog.Warn("shutdown: close player", "err", err)
		}
	}
	slog.Info("shutdown: player closed")

	// Step 2: Stop and flush the library pipeline. This cancels the scan/watch
	// bridges and orchestrator and blocks until they exit, enqueuing their final
	// bounded batch flush into the DB writer. It must run before the DB writer is
	// drained (step 3) so those final upserts are not lost.
	if deps.StopLibraryPipeline != nil {
		slog.Info("shutdown: stopping library pipeline")
		deps.StopLibraryPipeline()
		slog.Info("shutdown: library pipeline stopped")
	}

	// Step 3: Flush pending DB writes by cancelling the writer context and
	// waiting for the goroutine to drain and exit.
	slog.Info("shutdown: flushing db writes")
	if deps.CancelDBWriter != nil {
		deps.CancelDBWriter()
	}
	if deps.WaitDBWriter != nil {
		deps.WaitDBWriter()
	}
	slog.Info("shutdown: db writes flushed")

	// Step 4: Close the database connection now that all writes have drained.
	slog.Info("shutdown: closing database")
	if deps.CloseDB != nil {
		deps.CloseDB()
	}
	slog.Info("shutdown: database closed")

	// Step 5: Release PID lockfile.
	slog.Info("shutdown: releasing lockfile")
	if deps.Lockfile != nil {
		deps.Lockfile.Release()
	}
	slog.Info("shutdown: lockfile released")

	// Step 6: Delete mpv socket file.
	slog.Info("shutdown: removing mpv socket")
	if deps.SocketPath != "" {
		if err := removeSocketFile(deps.SocketPath); err != nil {
			slog.Warn("shutdown: remove mpv socket", "err", err)
		}
	}
	slog.Info("shutdown: mpv socket removed")

	// Step 7: Close log file last so all shutdown log entries are captured.
	if deps.CloseLog != nil {
		deps.CloseLog()
	}
}
