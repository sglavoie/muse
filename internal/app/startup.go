package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
)

const mpvSocketName = "mpv.sock"

// Deps holds injectable operations for the startup sequence. The LaunchMPV
// and RunScan fields are required. CheckPrereqs defaults to
// config.CheckPrereqs when nil.
type Deps struct {
	// Config holds the loaded application configuration. LaunchMPV and
	// RunScan closures can read cfg.MPVExtraFlags, cfg.MusicDirectory, etc.
	Config config.Config

	// CheckPrereqs verifies that required binaries are available.
	// If nil, config.CheckPrereqs is used.
	CheckPrereqs func() error

	// LaunchMPV starts the mpv process listening on socketPath.
	// IMPORTANT: in production this function MUST be non-blocking — it should
	// spawn the subprocess and return immediately. If LaunchMPV blocks until
	// mpv exits, wg.Wait() inside Run will stall and the TUI will never start.
	LaunchMPV func(ctx context.Context, socketPath string) error

	// RunScan performs the library scan against the open database.
	RunScan func(ctx context.Context, database *db.DB) error
}

// RunResult holds resources created during startup that must survive beyond
// Run's return so that main.go can pass them to Shutdown.
type RunResult struct {
	// Lockfile is the acquired PID lockfile. The caller must release it via
	// ShutdownDeps.Lockfile (never call Release directly before Shutdown).
	Lockfile *config.Lockfile

	// SocketPath is the mpv Unix socket path, for deletion during shutdown.
	SocketPath string

	// DB is the open database connection. The caller must close it via
	// ShutdownDeps.CloseDB after all writes have drained.
	DB *db.DB

	// Writer is the single-writer goroutine. The caller stops it by calling
	// CancelWriter then Writer.Wait.
	Writer *db.Writer

	// CancelWriter cancels the context driving the Writer goroutine.
	CancelWriter context.CancelFunc
}

// Run executes the startup sequence in the required order:
//  1. Acquire PID lockfile
//  2. Remove stale mpv socket
//  3. Run pending DB migrations
//  4. Verify prerequisite binaries (mpv, ExifTool)
//  5. Launch mpv and library scan concurrently
//
// Run blocks until both parallel tasks complete or an error occurs. A failure
// at any step returns immediately without executing subsequent steps.
// On success the caller owns the returned RunResult and is responsible for
// calling Shutdown so that the lockfile and socket are cleaned up.
func Run(ctx context.Context, dirs config.Dirs, deps Deps) (*RunResult, error) {
	checkPrereqs := deps.CheckPrereqs
	if checkPrereqs == nil {
		checkPrereqs = config.CheckPrereqs
	}

	// Step 1: Acquire PID lockfile.
	slog.Info("startup: acquiring lockfile")
	lf := config.NewLockfile(dirs.Runtime)
	if err := lf.Acquire(); err != nil {
		return nil, fmt.Errorf("startup: %w", err)
	}
	// NOTE: do NOT defer lf.Release() here — the caller holds the lockfile
	// for the full application lifetime and releases it via Shutdown.
	slog.Info("startup: lockfile acquired")

	// failf releases the lockfile and returns a wrapped error. Used for all
	// error paths after step 1 so the lockfile is never leaked on failure.
	failf := func(format string, args ...any) (*RunResult, error) {
		lf.Release()
		return nil, fmt.Errorf(format, args...)
	}

	// Step 2: Remove stale mpv socket file.
	socketPath := deps.Config.MPVSocketPath
	if socketPath == "" {
		socketPath = filepath.Join(dirs.Runtime, mpvSocketName)
	}
	slog.Info("startup: removing stale mpv socket", "path", socketPath)
	if err := removeStaleSocket(socketPath); err != nil {
		return failf("startup: %w", err)
	}
	slog.Info("startup: stale mpv socket removed")

	// Step 3: Open database and run pending migrations.
	slog.Info("startup: running database migrations")
	dbPath := filepath.Join(dirs.Data, "muse.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return failf("startup: open database: %w", err)
	}
	// From here, failf must also close the database.
	failf = func(format string, args ...any) (*RunResult, error) {
		database.Close()
		lf.Release()
		return nil, fmt.Errorf(format, args...)
	}

	if err := db.Migrate(database.DB); err != nil {
		return failf("startup: migrate: %w", err)
	}
	slog.Info("startup: database migrations complete")

	// Start the single-writer goroutine. The caller owns writerCtx and must
	// call CancelWriter then Writer.Wait during shutdown.
	writerCtx, cancelWriter := context.WithCancel(ctx)
	w := db.NewWriter(writerCtx, database.DB)
	// From here, failf must also stop the writer before closing the DB.
	failf = func(format string, args ...any) (*RunResult, error) {
		cancelWriter()
		w.Wait()
		database.Close()
		lf.Release()
		return nil, fmt.Errorf(format, args...)
	}

	// Step 4: Verify prerequisite binaries.
	slog.Info("startup: checking prerequisites")
	if err := checkPrereqs(); err != nil {
		return failf("startup: %w", err)
	}
	slog.Info("startup: prerequisites verified")

	// Step 5: Launch mpv and library scan concurrently.
	slog.Info("startup: launching mpv and library scan")
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Go(func() {
		if err := deps.LaunchMPV(ctx, socketPath); err != nil {
			errs <- fmt.Errorf("startup: mpv: %w", err)
		}
	})

	wg.Go(func() {
		if err := deps.RunScan(ctx, database); err != nil {
			errs <- fmt.Errorf("startup: scan: %w", err)
		}
	})

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return failf("%w", err)
		}
	}

	return &RunResult{
		Lockfile:     lf,
		SocketPath:   socketPath,
		DB:           database,
		Writer:       w,
		CancelWriter: cancelWriter,
	}, nil
}

// removeStaleSocket removes the mpv socket file if it exists. A missing file
// is not an error.
func removeStaleSocket(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale mpv socket %q: %w", path, err)
	}
	return nil
}
