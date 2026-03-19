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
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

const mpvSocketName = "mpv.sock"

// RestoredSession holds session state loaded from the database at startup.
// All queue entries have been validated against disk; unreachable files are
// marked with Missing=true. An empty RestoredSession (no entries, default
// flags) is returned when no prior session data exists.
type RestoredSession struct {
	// QueueEntries are the TUI-ready queue entries ordered by position.
	// Missing is set to true for any entry whose file cannot be stat-ed.
	QueueEntries []tui.QueueEntry

	// PlayerTracks are the ordered player queue tracks, parallel to QueueEntries.
	// BLAKE3 content hash is used as the track identity key.
	PlayerTracks []player.QueueTrack

	// RepeatMode from the persisted session row (default: RepeatOff).
	RepeatMode tui.RepeatMode

	// Shuffle from the persisted session row (default: false).
	Shuffle bool

	// CurrentHash is the BLAKE3 content hash of the last-playing track.
	// Empty when no track was playing at the time of the last save.
	CurrentHash string

	// PositionMs is the elapsed playback position in milliseconds.
	PositionMs int64
}

// Deps holds injectable operations for the startup sequence. The LaunchMPV
// and RunScan fields are required. CheckPrereqs defaults to
// config.CheckPrereqs when nil.
type Deps struct {
	// Config holds the loaded application configuration. LaunchMPV and
	// RunScan closures can read cfg.MPVExtraFlags, cfg.MusicDirectory, etc.
	Config config.Config

	// CheckPrereqs verifies that required binaries are available.
	// If nil, config.CheckPrereqs is used.
	CheckPrereqs func(context.Context) error

	// LaunchMPV starts the mpv process listening on socketPath.
	// IMPORTANT: in production this function MUST be non-blocking — it should
	// spawn the subprocess and return immediately. If LaunchMPV blocks until
	// mpv exits, wg.Wait() inside Run will stall and the TUI will never start.
	LaunchMPV func(ctx context.Context, socketPath string) error

	// RunScan is called concurrently with LaunchMPV during step 6 of Run.
	// In production (main.go) this is a no-op because the real library scan
	// pipeline is launched by the caller after Run returns (the scan needs
	// direct access to the Writer which is an internal implementation detail
	// of Run). Injecting a real scan here in tests validates the concurrent
	// execution contract between LaunchMPV and RunScan.
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
}

// Run executes the startup sequence in the required order:
//  1. Acquire PID lockfile
//  2. Remove stale mpv socket
//  3. Run pending DB migrations
//  4. Restore previous session from DB (validate tracks against disk)
//  5. Verify prerequisite binaries (mpv, ExifTool)
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
	if err := checkPrereqs(ctx); err != nil {
		return failf("startup: %w", err)
	}
	slog.Info("startup: prerequisites verified")

	// Step 6: Launch mpv and library scan concurrently.
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

	// Step 7: Restore player to the last saved position in a paused state.
	// LaunchMPV has returned by this point so the player Manager's command
	// queue is initialised; RestorePlayer may enqueue commands for replay
	// once the IPC socket connects.
	if restored.CurrentHash != "" && deps.RestorePlayer != nil {
		for _, pt := range restored.PlayerTracks {
			if pt.Hash == restored.CurrentHash {
				_ = deps.RestorePlayer(pt.Path, restored.PositionMs)
				break
			}
		}
	}

	return &RunResult{
		Lockfile:     lf,
		SocketPath:   socketPath,
		DB:           database,
		Writer:       w,
		CancelWriter: cancelWriter,
		Restored:     restored,
	}, nil
}

// RestoreSession loads the persisted queue and session state from the
// database, validates each queue track against disk, and marks any entry
// whose file cannot be stat-ed as missing. When shuffle is enabled a fresh
// random shuffle ordering is regenerated so that each session starts with a
// new permutation. Returns a non-nil *RestoredSession even when no prior
// session exists (all fields at zero values).
func RestoreSession(database *db.DB, writer *db.Writer) (*RestoredSession, error) {
	rows, err := database.ReadQueue()
	if err != nil {
		return nil, fmt.Errorf("restore session: read queue: %w", err)
	}

	var tuiEntries []tui.QueueEntry
	var playerTracks []player.QueueTrack
	for _, r := range rows {
		missing := r.Missing
		if !missing {
			if _, statErr := os.Stat(r.Path); os.IsNotExist(statErr) {
				missing = true
				_ = writer.MarkQueueEntryMissing(r.ID)
				_ = writer.MarkTrackMissing(r.Hash)
			}
		}
		tuiEntries = append(tuiEntries, tui.QueueEntry{
			ID:             r.ID,
			Path:           r.Path,
			Hash:           r.Hash,
			Display:        buildDisplay(r.Artist, r.Title, r.Path),
			Missing:        missing,
			Artist:         r.Artist,
			Title:          r.Title,
			Album:          r.Album,
			DurationMs:     r.DurationMs,
			Position:       r.Position,
			SourcePlaylist: r.SourcePlaylist,
		})
		playerTracks = append(playerTracks, player.QueueTrack{
			Path:    r.Path,
			Hash:    r.Hash,
			QueueID: r.ID,
			Artist:  r.Artist,
			Title:   r.Title,
		})
	}

	sess, err := database.ReadSession()
	if err != nil {
		return nil, fmt.Errorf("restore session: read session: %w", err)
	}

	var repeatMode tui.RepeatMode
	var shuffle bool
	var currentHash string
	var positionMs int64
	if sess != nil {
		repeatMode = parseRepeatMode(sess.RepeatMode)
		shuffle = sess.Shuffle
		currentHash = sess.CurrentHash
		positionMs = sess.PositionMs
	}

	return &RestoredSession{
		QueueEntries: tuiEntries,
		PlayerTracks: playerTracks,
		RepeatMode:   repeatMode,
		Shuffle:      shuffle,
		CurrentHash:  currentHash,
		PositionMs:   positionMs,
	}, nil
}

// buildDisplay returns a human-readable label for a queue entry, falling back
// to the file path when both artist and title are empty.
func buildDisplay(artist, title, path string) string {
	if artist == "" && title == "" {
		return path
	}
	if artist == "" {
		return title
	}
	if title == "" {
		return artist
	}
	return artist + " – " + title
}

// parseRepeatMode converts a DB repeat_mode string to a tui.RepeatMode.
// Unrecognised values are treated as RepeatOff.
func parseRepeatMode(s string) tui.RepeatMode {
	switch s {
	case "all":
		return tui.RepeatAll
	case "one":
		return tui.RepeatOne
	default:
		return tui.RepeatOff
	}
}

// removeStaleSocket removes the mpv socket file if it exists. A missing file
// is not an error.
func removeStaleSocket(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale mpv socket %q: %w", path, err)
	}
	return nil
}
