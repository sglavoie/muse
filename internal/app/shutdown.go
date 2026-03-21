package app

import (
	"log/slog"
	"os"
	"time"

	"github.com/sglavoie/muse/internal/config"
)

// ShutdownTimeout is the maximum time allowed for graceful shutdown before
// the process is forcibly terminated.
const ShutdownTimeout = 10 * time.Second

// ShutdownDeps holds injectable teardown operations for the graceful shutdown
// sequence. Nil StopPlayback or TerminateMPV fields are treated as no-ops.
// CancelDBWriter and WaitDBWriter should both be set whenever a DB writer was
// started; omitting WaitDBWriter risks missing in-flight writes.
type ShutdownDeps struct {
	// StopPlayback halts the current playback session before the mpv process
	// is terminated. Nil means no-op.
	StopPlayback func() error

	// TerminateMPV kills the mpv process. Nil means no-op.
	TerminateMPV func() error

	// CancelDBWriter cancels the context driving the DB writer goroutine,
	// causing it to stop accepting new work.
	CancelDBWriter func()

	// WaitDBWriter blocks until the DB writer goroutine has fully exited,
	// ensuring all in-flight writes complete before the database is closed.
	WaitDBWriter func()

	// CloseDB closes the database connection after the writer has drained.
	// Nil means no-op.
	CloseDB func()

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
//  1. Stop playback
//  2. Terminate mpv process
//  3. Flush pending DB writes (cancel writer context, wait for goroutine exit)
//  4. Remove PID lockfile
//  5. Delete mpv socket file
//
// All steps are attempted regardless of prior errors so that resources are
// always released. Errors from individual steps are logged at Warn level.
func Shutdown(deps ShutdownDeps, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		shutdownSequence(deps)
	}()
	select {
	case <-done:
		return
	case <-time.After(timeout):
		slog.Error("shutdown timed out after " + timeout.String() + ", forcing exit")
		if deps.CloseLog != nil {
			deps.CloseLog()
		}
		os.Exit(1)
	}
}

// shutdownSequence performs the ordered teardown steps.
func shutdownSequence(deps ShutdownDeps) {
	// Step 1: Stop playback.
	slog.Info("shutdown: stopping playback")
	if deps.StopPlayback != nil {
		if err := deps.StopPlayback(); err != nil {
			slog.Warn("shutdown: stop playback", "err", err)
		}
	}
	slog.Info("shutdown: playback stopped")

	// Step 2: Terminate mpv process.
	slog.Info("shutdown: terminating mpv")
	if deps.TerminateMPV != nil {
		if err := deps.TerminateMPV(); err != nil {
			slog.Warn("shutdown: terminate mpv", "err", err)
		}
	}
	slog.Info("shutdown: mpv terminated")

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

	// Step 3.5: Close the database connection now that all writes have drained.
	slog.Info("shutdown: closing database")
	if deps.CloseDB != nil {
		deps.CloseDB()
	}
	slog.Info("shutdown: database closed")

	// Step 4: Release PID lockfile.
	slog.Info("shutdown: releasing lockfile")
	if deps.Lockfile != nil {
		deps.Lockfile.Release()
	}
	slog.Info("shutdown: lockfile released")

	// Step 5: Delete mpv socket file.
	slog.Info("shutdown: removing mpv socket")
	if deps.SocketPath != "" {
		if err := os.Remove(deps.SocketPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("shutdown: remove mpv socket", "err", err)
		}
	}
	slog.Info("shutdown: mpv socket removed")

	// Step 6: Close log file last so all shutdown log entries are captured.
	if deps.CloseLog != nil {
		deps.CloseLog()
	}
}
