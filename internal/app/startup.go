package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

const mpvSocketName = "mpv.sock"

// RestoredSession holds session state loaded from the database at startup.
// All queue entries have been validated against disk; unreachable files are
// marked with Missing=true. An empty RestoredSession (no entries, default
// flags) is returned when no prior session data exists.
type RestoredSession struct {
	// QueueEntries are the TUI-ready queue entries ordered by position.
	// Missing is set to true for any entry whose file cannot be stat-ed.
	QueueEntries []queue.Entry

	// PlayerTracks are the ordered player queue tracks, parallel to QueueEntries.
	// BLAKE3 content hash is used as the track identity key.
	PlayerTracks []player.QueueTrack

	// RepeatMode from the persisted session row (default: RepeatOff).
	RepeatMode playback.RepeatMode

	// Shuffle from the persisted session row (default: false).
	Shuffle bool

	// CurrentHash is the BLAKE3 content hash of the last-playing track.
	// Empty when no track was playing at the time of the last save.
	CurrentHash string

	// PositionMs is the elapsed playback position in milliseconds.
	PositionMs int64

	// RadioMode from the persisted session row (default: false).
	RadioMode bool

	// RecentHashes are the most recently played track hashes, oldest-first,
	// used to seed radio's recent-history avoidance so it survives a restart.
	// Empty when play history is empty or unreadable.
	RecentHashes []string

	// Volume is the last-saved volume percentage (0–100).
	// Negative means no volume was persisted; callers should fall back to default.
	Volume int

	// DJ is the persisted Auto-DJ session: the applied settings snapshot, the
	// seed captured when the session started, and the discovery rounding carry.
	// Zero when nothing was saved or when DJStateErr is non-nil.
	DJ db.DJSession

	// DJStateErr reports saved Auto-DJ state that could not be used — invalid
	// persisted settings, or state written by a newer build. It is surfaced to
	// the user rather than repaired: replacing unreadable filters with stock
	// defaults would silently turn a restrictive session into an unrestricted
	// one. The rest of the session still restores normally.
	DJStateErr error
}

// Deps holds injectable operations for the startup sequence. LaunchMPV is
// required. CheckPrereqs defaults to config.CheckPrereqs when nil.
type Deps struct {
	// Config holds the loaded application configuration. LaunchMPV and
	// RunScan closures can read cfg.MPVExtraFlags, cfg.MusicDirectory, etc.
	Config config.Config

	// CheckPrereqs verifies that required binaries are available. It returns
	// advisory warnings (logged, never fatal) separately from the error that
	// aborts startup. If nil, config.CheckPrereqs is used.
	CheckPrereqs func(context.Context) ([]string, error)

	// LaunchMPV starts the mpv process listening on socketPath.
	// IMPORTANT: in production this function MUST be non-blocking — it should
	// spawn the subprocess and return immediately. If LaunchMPV blocks until
	// mpv exits, wg.Wait() inside Run will stall and the TUI will never start.
	LaunchMPV func(ctx context.Context, socketPath string) error

	// RunScan, when non-nil, is called concurrently with LaunchMPV during
	// step 6 of Run. In production this is nil because the real library scan
	// pipeline is launched by the caller after Run returns (the scan needs
	// direct access to the Writer which is an internal implementation detail
	// of Run). Tests set RunScan to validate the concurrent execution
	// contract between LaunchMPV and RunScan.
	RunScan func(ctx context.Context, database *db.DB) error

	// RestorePlayer, when non-nil, is called after LaunchMPV succeeds with
	// the file path and elapsed position (milliseconds) of the last-played
	// track. The implementation must load the file and seek to position_ms in
	// a paused state without calling Play(). Pass nil to skip player restore.
	RestorePlayer func(path string, positionMs int64) error
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

	// Restored holds the session state loaded from the database. Never nil.
	Restored *RestoredSession

	// PrereqWarnings are the advisory messages returned by CheckPrereqs in
	// step 5 — a degraded metadata backend or an unavailable notification
	// backend. They are logged by Run, but they also describe behaviour the
	// user will actually notice, so they are handed back for the caller to
	// surface on screen. Nil when every prerequisite is healthy.
	PrereqWarnings []string
}

// Run executes the startup sequence in the required order:
//  1. Acquire PID lockfile
//  2. Remove stale mpv socket
//  3. Run pending DB migrations, then purge rows soft-deleted by a previous
//     process (failures are logged, never fatal)
//  4. Restore previous session from DB (validate tracks against disk)
//  5. Verify prerequisite binaries (mpv, ExifTool); advisory warnings are
//     logged and returned in RunResult.PrereqWarnings for the caller to display
//  6. Launch mpv (via LaunchMPV) and call RunScan concurrently; in production
//     RunScan is a no-op — the library scan pipeline is started by the caller
//     after Run returns because it requires the Writer that Run creates
//     internally.
//  7. Seek player to last position in paused state (if session data present)
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
	if deps.LaunchMPV == nil {
		return nil, fmt.Errorf("startup: LaunchMPV is required")
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
	previewSocket := filepath.Join(dirs.Runtime, "muse-preview.sock")
	if err := removeStaleSocket(previewSocket); err != nil {
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
		if cerr := database.Close(); cerr != nil {
			slog.Warn("startup: failed to close database during cleanup", "err", cerr)
		}
		lf.Release()
		return nil, fmt.Errorf(format, args...)
	}

	if err := db.Migrate(database.DB); err != nil {
		return failf("startup: migrate: %w", err)
	}
	slog.Info("startup: database migrations complete")

	// Start the single-writer goroutine. The caller owns writerCtx and must
	// call CancelWriter then Writer.Wait during shutdown.
	writerCtx, cancelWriter := context.WithCancel(context.Background())
	w := db.NewWriter(writerCtx, database.DB)
	// From here, failf must also stop the writer before closing the DB.
	failf = func(format string, args ...any) (*RunResult, error) {
		cancelWriter()
		w.Wait()
		if cerr := database.Close(); cerr != nil {
			slog.Warn("startup: failed to close database during cleanup", "err", cerr)
		}
		lf.Release()
		return nil, fmt.Errorf(format, args...)
	}

	// Purge rows a previous process soft-deleted: their in-memory undo windows
	// died with it. The cutoff is one second ahead so tombstones stamped this
	// second, including the abandoned preparations NewWriter just recovered,
	// go too; nothing in this process has offered an undo yet.
	slog.Info("startup: purging stale soft-deleted rows")
	purgeStaleSoftDeletes(w, time.Now().Unix()+1)

	// Step 4: Restore previous session from the database.
	// This is a read-only DB operation (plus missing-entry writes) that runs
	// before prereq checks so the TUI can display restored queue state
	// immediately on first render.
	slog.Info("startup: restoring session")
	restored, err := RestoreSession(database, w)
	if err != nil {
		slog.Warn("startup: session restore failed, starting fresh", "err", err)
		restored = &RestoredSession{}
	}
	slog.Info("startup: session restored", "queue_entries", len(restored.QueueEntries))

	// Step 5: Verify prerequisite binaries.
	slog.Info("startup: checking prerequisites")
	warnings, err := checkPrereqs(ctx)
	for _, w := range warnings {
		slog.Warn("startup: prerequisite warning", "warning", w)
	}
	if err != nil {
		return failf("startup: %w", err)
	}
	slog.Info("startup: prerequisites verified")

	// Step 6: Launch mpv and library scan concurrently.
	slog.Info("startup: launching mpv and library scan")
	if err := runStartupTasks(ctx, database, socketPath, deps); err != nil {
		return failf("%w", err)
	}

	// Step 7: Restore player to the last saved position in a paused state.
	// LaunchMPV has returned by this point so the player Manager's command
	// queue is initialised; RestorePlayer may enqueue commands for replay
	// once the IPC socket connects.
	restoreStartupPlayer(restored, deps.RestorePlayer)

	return &RunResult{
		Lockfile:       lf,
		SocketPath:     socketPath,
		DB:             database,
		Writer:         w,
		CancelWriter:   cancelWriter,
		Restored:       restored,
		PrereqWarnings: warnings,
	}, nil
}
