package app

import (
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/safego"
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

	// DrainControl tells the hosted-session control server that muse is
	// quitting, so it refuses further requests and a host hears why. It runs
	// first, before anything is torn down. Nil means no control server.
	DrainControl func()

	// CtrlListener is the Unix domain socket listener for remote commands.
	// Closed first during shutdown to stop accepting new commands. Nil means
	// remote control was not started.
	CtrlListener net.Listener

	// Lockfile is the PID lockfile to release on shutdown.
	Lockfile *config.Lockfile

	// SocketPath is the path to the mpv Unix socket to delete.
	SocketPath string

	// CloseControl ends every control connection and removes its socket. It
	// runs after every other teardown step and just before CloseLog, so a host
	// that reads EOF on the socket knows shutdown has finished. Nil means no
	// control server.
	CloseControl func()

	// CloseLog flushes and closes the log file. Called last so that all
	// shutdown log entries are written before the handle is released.
	CloseLog func()
}

// Shutdown executes the graceful teardown sequence with a timeout. If the
// sequence does not complete within timeout, the process is forcibly exited.
// Pass ShutdownTimeout for the normal production timeout.
//
// The teardown sequence is:
//  0. Drain the control server (refuse requests, announce quitting), then
//     close the remote control listener (stop accepting new commands)
//  1. Close the player (stop playback + terminate mpv)
//  2. Stop and flush the library pipeline (final watcher/scan upserts)
//  3. Flush pending DB writes (cancel writer context, wait for goroutine exit)
//  4. Close database connection
//  5. Delete mpv socket file (while the lockfile still guards its path)
//  6. Release PID lockfile
//  7. Close the control server (its EOF tells a host shutdown finished)
//  8. Close log file (last, so all shutdown entries are captured)
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
		// Backstop only: every teardown step already recovers individually in
		// runShutdownStep, so this catches a panic raised by shutdownSequence's
		// own scaffolding rather than by a caller-supplied callback. Recovering
		// lets close(done) release the select below so Shutdown returns normally
		// instead of dying with the log unflushed.
		defer safego.Recover("app.shutdownSequence")
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

// runShutdownStep runs one teardown step with its own panic recovery.
//
// Isolating each step is what makes the "all steps are attempted regardless of
// prior errors" contract hold for panics too, not just for returned errors: the
// callbacks in ShutdownDeps are supplied by the caller (player, library
// pipeline, DB writer), so a panic in any of them would otherwise unwind out of
// shutdownSequence and skip every later step — the DB would never be closed, the
// lockfile never released, the log never flushed.
//
// The recovered panic is logged by safego.Recover under the goroutine name
// built here, so the log line names the step that failed.
func runShutdownStep(step string, fn func()) {
	defer safego.Recover("app.shutdownSequence step " + step)
	fn()
}

// shutdownSequence performs the ordered teardown steps. Each step runs under
// runShutdownStep, so a panic in one step is logged and the remaining steps
// still run exactly once.
func shutdownSequence(deps ShutdownDeps) {
	// Step 0: Drain the control server so a host stops sending requests and
	// learns why, then close the remote control listener to stop accepting new
	// commands.
	runShutdownStep("0a: drain control server", func() {
		if deps.DrainControl == nil {
			return
		}
		slog.Info("shutdown: draining control server")
		deps.DrainControl()
	})
	runShutdownStep("0b: close control listener", func() {
		if deps.CtrlListener == nil {
			return
		}
		slog.Info("shutdown: closing control listener")
		if err := deps.CtrlListener.Close(); err != nil {
			slog.Warn("shutdown: close control listener", "err", err)
		}
		slog.Info("shutdown: control listener closed")
	})

	// Step 1: Close the player (stop playback + terminate mpv).
	runShutdownStep("1: close player", func() {
		slog.Info("shutdown: closing player")
		if deps.ClosePlayer != nil {
			if err := deps.ClosePlayer(); err != nil {
				slog.Warn("shutdown: close player", "err", err)
			}
		}
		slog.Info("shutdown: player closed")
	})

	// Step 2: Stop and flush the library pipeline. This cancels the scan/watch
	// bridges and orchestrator and blocks until they exit, enqueuing their final
	// bounded batch flush into the DB writer. It must run before the DB writer is
	// drained (step 3) so those final upserts are not lost.
	runShutdownStep("2: stop library pipeline", func() {
		if deps.StopLibraryPipeline == nil {
			return
		}
		slog.Info("shutdown: stopping library pipeline")
		deps.StopLibraryPipeline()
		slog.Info("shutdown: library pipeline stopped")
	})

	// Step 3: Flush pending DB writes by cancelling the writer context and
	// waiting for the goroutine to drain and exit. Cancel and wait are separate
	// steps so a panicking CancelDBWriter cannot leave the writer goroutine
	// un-awaited, and a panicking WaitDBWriter cannot skip closing the DB.
	runShutdownStep("3a: cancel db writer", func() {
		slog.Info("shutdown: flushing db writes")
		if deps.CancelDBWriter != nil {
			deps.CancelDBWriter()
		}
	})
	runShutdownStep("3b: wait for db writer", func() {
		if deps.WaitDBWriter != nil {
			deps.WaitDBWriter()
		}
		slog.Info("shutdown: db writes flushed")
	})

	// Step 4: Close the database connection now that all writes have drained.
	runShutdownStep("4: close database", func() {
		slog.Info("shutdown: closing database")
		if deps.CloseDB != nil {
			deps.CloseDB()
		}
		slog.Info("shutdown: database closed")
	})

	// Step 5: Delete mpv socket file. mpv is already gone (step 1), and this
	// must run before the lockfile is released: a muse launched right after
	// the release launches its own mpv on the same path, and removing the
	// file then would cut that session off from its player.
	runShutdownStep("5: remove mpv socket", func() {
		slog.Info("shutdown: removing mpv socket")
		if deps.SocketPath != "" {
			if err := removeSocketFile(deps.SocketPath); err != nil {
				slog.Warn("shutdown: remove mpv socket", "err", err)
			}
		}
		slog.Info("shutdown: mpv socket removed")
	})

	// Step 6: Release PID lockfile.
	runShutdownStep("6: release lockfile", func() {
		slog.Info("shutdown: releasing lockfile")
		if deps.Lockfile != nil {
			deps.Lockfile.Release()
		}
		slog.Info("shutdown: lockfile released")
	})

	// Step 7: Close the control server. Everything else is torn down by now,
	// so the EOF a host reads on the socket means shutdown has finished.
	runShutdownStep("7: close control server", func() {
		if deps.CloseControl == nil {
			return
		}
		slog.Info("shutdown: closing control server")
		deps.CloseControl()
		slog.Info("shutdown: control server closed")
	})

	// Step 8: Close log file last so all shutdown log entries are captured.
	runShutdownStep("8: close log", func() {
		if deps.CloseLog != nil {
			deps.CloseLog()
		}
	})
}
