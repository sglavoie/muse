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
	// CheckPrereqs verifies that required binaries are available.
	// If nil, config.CheckPrereqs is used.
	CheckPrereqs func() error

	// LaunchMPV starts the mpv process listening on socketPath.
	LaunchMPV func(ctx context.Context, socketPath string) error

	// RunScan performs the library scan against the open database.
	RunScan func(ctx context.Context, database *db.DB) error
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
func Run(ctx context.Context, dirs config.Dirs, deps Deps) error {
	checkPrereqs := deps.CheckPrereqs
	if checkPrereqs == nil {
		checkPrereqs = config.CheckPrereqs
	}

	// Step 1: Acquire PID lockfile.
	slog.Info("startup: acquiring lockfile")
	lf := config.NewLockfile(dirs.Runtime)
	if err := lf.Acquire(); err != nil {
		return fmt.Errorf("startup: %w", err)
	}
	defer lf.Release()
	slog.Info("startup: lockfile acquired")

	// Step 2: Remove stale mpv socket file.
	socketPath := filepath.Join(dirs.Runtime, mpvSocketName)
	slog.Info("startup: removing stale mpv socket", "path", socketPath)
	if err := removeStaleSocket(socketPath); err != nil {
		return fmt.Errorf("startup: %w", err)
	}
	slog.Info("startup: stale mpv socket removed")

	// Step 3: Open database and run pending migrations.
	slog.Info("startup: running database migrations")
	dbPath := filepath.Join(dirs.Data, "muse.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("startup: open database: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(database.DB); err != nil {
		return fmt.Errorf("startup: migrate: %w", err)
	}
	slog.Info("startup: database migrations complete")

	// Step 4: Verify prerequisite binaries.
	slog.Info("startup: checking prerequisites")
	if err := checkPrereqs(); err != nil {
		return fmt.Errorf("startup: %w", err)
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
			return err
		}
	}

	return nil
}

// removeStaleSocket removes the mpv socket file if it exists. A missing file
// is not an error.
func removeStaleSocket(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale mpv socket %q: %w", path, err)
	}
	return nil
}
