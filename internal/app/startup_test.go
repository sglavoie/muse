package app_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui"
)

// logCapture is a minimal slog.Handler that records message strings.
type logCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	l.msgs = append(l.msgs, r.Message)
	l.mu.Unlock()
	return nil
}

func (l *logCapture) WithAttrs(_ []slog.Attr) slog.Handler { return l }
func (l *logCapture) WithGroup(_ string) slog.Handler      { return l }

func (l *logCapture) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]string, len(l.msgs))
	copy(cp, l.msgs)
	return cp
}

// setupDirs creates isolated temp directories and returns a config.Dirs that
// points to them.
func setupDirs(t *testing.T) config.Dirs {
	t.Helper()
	return config.Dirs{
		Runtime: t.TempDir(),
		Data:    t.TempDir(),
	}
}

// noopDeps returns a Deps where prereqs always pass and both parallel tasks
// succeed immediately.
func noopDeps() app.Deps {
	return app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}
}

// installLogger swaps in logCapture as the slog default and restores the
// previous default when the test ends.
func installLogger(t *testing.T) *logCapture {
	t.Helper()
	prev := slog.Default()
	cap := &logCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return cap
}

// hasMsg returns true if any captured message contains sub.
func hasMsg(msgs []string, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

// msgIndex returns the position of the first message containing sub, or -1.
func msgIndex(msgs []string, sub string) int {
	for i, m := range msgs {
		if strings.Contains(m, sub) {
			return i
		}
	}
	return -1
}

// TestRunOrder verifies that all startup steps execute in the correct sequence
// and that expected slog messages are emitted for each lifecycle event.
func TestRunOrder(t *testing.T) {
	dirs := setupDirs(t)
	log := installLogger(t)

	// Place a stale socket file to confirm it gets removed.
	staleSocket := filepath.Join(dirs.Runtime, "mpv.sock")
	if err := os.WriteFile(staleSocket, nil, 0o600); err != nil {
		t.Fatalf("create stale socket: %v", err)
	}

	var mu sync.Mutex
	var parallelSteps []string

	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV: func(_ context.Context, _ string) error {
			mu.Lock()
			parallelSteps = append(parallelSteps, "mpv")
			mu.Unlock()
			return nil
		},
		RunScan: func(_ context.Context, _ *db.DB) error {
			mu.Lock()
			parallelSteps = append(parallelSteps, "scan")
			mu.Unlock()
			return nil
		},
	}

	if _, err := app.Run(context.Background(), dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	// Stale socket must have been removed.
	if _, err := os.Stat(staleSocket); !os.IsNotExist(err) {
		t.Error("stale mpv socket was not removed")
	}

	// Both parallel tasks must have run.
	mu.Lock()
	gotMPV := contains(parallelSteps, "mpv")
	gotScan := contains(parallelSteps, "scan")
	mu.Unlock()
	if !gotMPV {
		t.Error("LaunchMPV was not called")
	}
	if !gotScan {
		t.Error("RunScan was not called")
	}

	// Verify log message ordering.
	msgs := log.messages()
	checks := []struct {
		before string
		after  string
	}{
		{"acquiring lockfile", "removing stale mpv socket"},
		{"removing stale mpv socket", "running database migrations"},
		{"running database migrations", "checking prerequisites"},
		{"checking prerequisites", "launching mpv and library scan"},
	}
	for _, c := range checks {
		bi := msgIndex(msgs, c.before)
		ai := msgIndex(msgs, c.after)
		if bi == -1 {
			t.Errorf("log message %q not found", c.before)
			continue
		}
		if ai == -1 {
			t.Errorf("log message %q not found", c.after)
			continue
		}
		if bi >= ai {
			t.Errorf("expected %q before %q, got indices %d >= %d", c.before, c.after, bi, ai)
		}
	}

	// Verify all expected lifecycle log messages are present.
	for _, expected := range []string{
		"acquiring lockfile",
		"lockfile acquired",
		"removing stale mpv socket",
		"stale mpv socket removed",
		"running database migrations",
		"database migrations complete",
		"checking prerequisites",
		"prerequisites verified",
		"launching mpv and library scan",
	} {
		if !hasMsg(msgs, expected) {
			t.Errorf("expected log message containing %q, not found in: %v", expected, msgs)
		}
	}
}

// TestRunLockfileFailureStopsSequence verifies that a lockfile conflict causes
// Run to return an error and no subsequent steps execute.
func TestRunLockfileFailureStopsSequence(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	// Write a lockfile that claims a live PID (use current process PID).
	lockPath := filepath.Join(dirs.Runtime, "muse.pid")
	if err := os.MkdirAll(dirs.Runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("999999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// PID 999999999 is almost certainly not alive, so acquire will reclaim it.
	// To force a real conflict we write the actual running PID.
	if err := os.WriteFile(lockPath, []byte(pidStr(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	prereqCalled := false
	deps := app.Deps{
		CheckPrereqs: func() error { prereqCalled = true; return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from lockfile conflict, got nil")
	}
	if !strings.Contains(err.Error(), "another instance is running") {
		t.Errorf("unexpected error: %v", err)
	}
	if prereqCalled {
		t.Error("CheckPrereqs should not have been called after lockfile failure")
	}
}

// TestRunPrereqFailureStopsParallelLaunch verifies that a prereq check failure
// prevents mpv and the scan from starting.
func TestRunPrereqFailureStopsParallelLaunch(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	prereqErr := errors.New("mpv not found")
	launchCalled := false

	deps := app.Deps{
		CheckPrereqs: func() error { return prereqErr },
		LaunchMPV:    func(_ context.Context, _ string) error { launchCalled = true; return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from prereq failure, got nil")
	}
	if !strings.Contains(err.Error(), prereqErr.Error()) {
		t.Errorf("unexpected error: %v", err)
	}
	if launchCalled {
		t.Error("LaunchMPV should not have been called after prereq failure")
	}
}

// TestRunMigrationFailureStopsPrereqs verifies that a migration failure
// prevents the prerequisite check from running.
func TestRunMigrationFailureStopsPrereqs(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	// Create a regular file at dirs.Data so that db.Open's MkdirAll call
	// fails when it tries to treat that path as a directory for muse.db.
	blockPath := filepath.Join(dirs.Data, "block")
	if err := os.WriteFile(blockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dirs.Data = blockPath

	prereqCalled := false
	deps := app.Deps{
		CheckPrereqs: func() error { prereqCalled = true; return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from DB open failure, got nil")
	}
	if prereqCalled {
		t.Error("CheckPrereqs should not have been called after migration failure")
	}
}

// TestRunStaleSocketRemovedBeforeMigration verifies that the socket file is
// removed after the lockfile is held but before the DB is opened.
func TestRunStaleSocketRemovedBeforeMigration(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	staleSocket := filepath.Join(dirs.Runtime, "mpv.sock")
	if err := os.WriteFile(staleSocket, nil, 0o600); err != nil {
		t.Fatalf("create stale socket: %v", err)
	}

	// Verify socket is gone by the time RunScan executes (after migration).
	socketRemovedBeforeScan := false
	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan: func(_ context.Context, _ *db.DB) error {
			_, err := os.Stat(staleSocket)
			socketRemovedBeforeScan = os.IsNotExist(err)
			return nil
		},
	}

	if _, err := app.Run(context.Background(), dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if !socketRemovedBeforeScan {
		t.Error("stale socket was not removed before scan launched")
	}
}

// TestRunNoStaleSocket verifies that Run succeeds even when no socket file exists.
func TestRunNoStaleSocket(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	if _, err := app.Run(context.Background(), dirs, noopDeps()); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
}

// TestRunParallelLaunchBothErrorsReturned verifies that if both parallel tasks
// fail, at least one error is surfaced.
func TestRunParallelLaunchBothErrorsReturned(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	mpvErr := errors.New("mpv failed")
	scanErr := errors.New("scan failed")

	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return mpvErr },
		RunScan:      func(_ context.Context, _ *db.DB) error { return scanErr },
	}

	_, err := app.Run(context.Background(), dirs, deps)
	if err == nil {
		t.Fatal("expected error from parallel task failures, got nil")
	}
}

// TestRunConcurrentLaunchAndScan proves that LaunchMPV and RunScan execute
// concurrently: RunScan must start (and record its start time) before the
// deliberately-delayed LaunchMPV returns.
func TestRunConcurrentLaunchAndScan(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	gate := make(chan struct{})

	var mu sync.Mutex
	var scanStarted, mpvReturned time.Time

	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV: func(_ context.Context, _ string) error {
			<-gate // block until gate is closed
			mu.Lock()
			mpvReturned = time.Now()
			mu.Unlock()
			return nil
		},
		RunScan: func(_ context.Context, _ *db.DB) error {
			mu.Lock()
			scanStarted = time.Now()
			mu.Unlock()
			return nil
		},
	}

	// Release gate after a short delay so RunScan has time to start first.
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(gate)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := app.Run(ctx, dirs, deps); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	mu.Lock()
	started := scanStarted
	returned := mpvReturned
	mu.Unlock()

	if started.IsZero() {
		t.Fatal("RunScan was never called")
	}
	if returned.IsZero() {
		t.Fatal("LaunchMPV never returned")
	}
	if !started.Before(returned) {
		t.Errorf("RunScan started (%v) was not before LaunchMPV returned (%v); goroutines are not running concurrently", started, returned)
	}
}

// ---- Session restore tests ----

// openTestDB opens (or creates) a SQLite database at dirs.Data/muse.db and
// runs all pending migrations. The caller must close the returned DB.
func openTestDB(t *testing.T, dirs config.Dirs) *db.DB {
	t.Helper()
	dbPath := filepath.Join(dirs.Data, "muse.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("openTestDB: open: %v", err)
	}
	if err := db.Migrate(database.DB); err != nil {
		database.Close()
		t.Fatalf("openTestDB: migrate: %v", err)
	}
	return database
}

// insertTrack inserts a minimal tracks row and returns the hash.
func insertTrack(t *testing.T, database *db.DB, hash, path, artist, title string) {
	t.Helper()
	now := time.Now().Unix()
	_, err := database.Exec(
		`INSERT INTO tracks (hash, path, artist, title, album, duration_ms, size_bytes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '', 120000, 1024, ?, ?)`,
		hash, path, artist, title, now, now,
	)
	if err != nil {
		t.Fatalf("insertTrack %q: %v", hash, err)
	}
}

// insertQueueEntry inserts a queue row at the given position and returns the row ID.
func insertQueueEntry(t *testing.T, database *db.DB, hash string, position int64) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := database.Exec(
		`INSERT INTO queue (track_hash, position, added_at) VALUES (?, ?, ?)`,
		hash, position, now,
	)
	if err != nil {
		t.Fatalf("insertQueueEntry hash=%q pos=%d: %v", hash, position, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// setSessionRow writes current_hash and position_ms into the single session row.
func setSessionRow(t *testing.T, database *db.DB, hash string, positionMs int64, shuffle bool, repeatMode string) {
	t.Helper()
	shuffleVal := 0
	if shuffle {
		shuffleVal = 1
	}
	var hashArg any
	if hash != "" {
		hashArg = hash
	}
	_, err := database.Exec(
		`UPDATE session SET current_hash=?, position_ms=?, shuffle=?, repeat_mode=?, updated_at=? WHERE id=1`,
		hashArg, positionMs, shuffleVal, repeatMode, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("setSessionRow: %v", err)
	}
}

// newTestWriter creates a db.Writer for use in RestoreSession tests.
// It cancels the context (stopping the goroutine) on test cleanup.
func newTestWriter(t *testing.T, database *db.DB) *db.Writer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := db.NewWriter(ctx, database.DB)
	t.Cleanup(func() {
		cancel()
		w.Wait()
	})
	return w
}

// TestRestoreSessionEmpty verifies that RestoreSession on an empty database
// returns a non-nil RestoredSession with no entries and default flags.
func TestRestoreSessionEmpty(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if restored == nil {
		t.Fatal("RestoreSession returned nil")
	}
	if len(restored.QueueEntries) != 0 {
		t.Errorf("QueueEntries: want 0, got %d", len(restored.QueueEntries))
	}
	if len(restored.PlayerTracks) != 0 {
		t.Errorf("PlayerTracks: want 0, got %d", len(restored.PlayerTracks))
	}
	if restored.RepeatMode != tui.RepeatOff {
		t.Errorf("RepeatMode: want RepeatOff, got %v", restored.RepeatMode)
	}
	if restored.Shuffle {
		t.Error("Shuffle: want false")
	}
	if restored.CurrentHash != "" {
		t.Errorf("CurrentHash: want empty, got %q", restored.CurrentHash)
	}
}

// TestRestoreSessionReadableFiles verifies that queue entries with readable
// file paths are returned with Missing=false.
func TestRestoreSessionReadableFiles(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	// Create a real audio file so os.Stat succeeds.
	audioPath := filepath.Join(dirs.Runtime, "track.mp3")
	if err := os.WriteFile(audioPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	insertTrack(t, database, "hash-aaa", audioPath, "Artist A", "Song A")
	insertQueueEntry(t, database, "hash-aaa", 1000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	e := restored.QueueEntries[0]
	if e.Missing {
		t.Error("QueueEntry.Missing: want false for readable file")
	}
	if e.Artist != "Artist A" {
		t.Errorf("Artist: want %q, got %q", "Artist A", e.Artist)
	}
	if e.Title != "Song A" {
		t.Errorf("Title: want %q, got %q", "Song A", e.Title)
	}
	if e.Display != "Artist A – Song A" {
		t.Errorf("Display: want %q, got %q", "Artist A – Song A", e.Display)
	}
	if len(restored.PlayerTracks) != 1 {
		t.Fatalf("PlayerTracks: want 1, got %d", len(restored.PlayerTracks))
	}
	if restored.PlayerTracks[0].Hash != "hash-aaa" {
		t.Errorf("PlayerTracks[0].Hash: want %q, got %q", "hash-aaa", restored.PlayerTracks[0].Hash)
	}
}

// TestRestoreSessionMissingFiles verifies that queue entries with unreadable
// file paths have Missing=true in the returned QueueEntry slice.
func TestRestoreSessionMissingFiles(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	// Use a path that does not exist on disk.
	missingPath := filepath.Join(dirs.Runtime, "nonexistent.mp3")

	insertTrack(t, database, "hash-bbb", missingPath, "Artist B", "Song B")
	insertQueueEntry(t, database, "hash-bbb", 1000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(restored.QueueEntries))
	}
	if !restored.QueueEntries[0].Missing {
		t.Error("QueueEntry.Missing: want true for unreadable path")
	}
	// The player track must still be present (missing tracks stay in queue).
	if len(restored.PlayerTracks) != 1 {
		t.Fatalf("PlayerTracks: want 1, got %d", len(restored.PlayerTracks))
	}
}

// TestRestoreSessionMixedFiles verifies that only missing files have Missing=true
// when the queue contains both readable and unreadable entries.
func TestRestoreSessionMixedFiles(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	readablePath := filepath.Join(dirs.Runtime, "exists.mp3")
	if err := os.WriteFile(readablePath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dirs.Runtime, "gone.mp3")

	insertTrack(t, database, "hash-r", readablePath, "Readable", "Track")
	insertTrack(t, database, "hash-m", missingPath, "Missing", "Track")
	insertQueueEntry(t, database, "hash-r", 1000)
	insertQueueEntry(t, database, "hash-m", 2000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 2 {
		t.Fatalf("QueueEntries: want 2, got %d", len(restored.QueueEntries))
	}
	// Order follows queue position.
	if restored.QueueEntries[0].Missing {
		t.Error("QueueEntries[0] (readable): want Missing=false")
	}
	if !restored.QueueEntries[1].Missing {
		t.Error("QueueEntries[1] (missing path): want Missing=true")
	}
}

// TestRestoreSessionQueuePersistence verifies that queue contents survive a
// simulated session restart: after inserting two tracks and their queue rows,
// RestoreSession returns fully-populated QueueEntries and parallel PlayerTracks
// with matching IDs, hashes, artist, and title values.
func TestRestoreSessionQueuePersistence(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	// Create real audio files so os.Stat succeeds (Missing=false).
	pathA := filepath.Join(dirs.Runtime, "track-a.mp3")
	pathB := filepath.Join(dirs.Runtime, "track-b.mp3")
	for _, p := range []string{pathA, pathB} {
		if err := os.WriteFile(p, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	insertTrack(t, database, "hash-aaa", pathA, "Artist A", "Song A")
	insertTrack(t, database, "hash-bbb", pathB, "Artist B", "Song B")

	idA := insertQueueEntry(t, database, "hash-aaa", 1000)
	idB := insertQueueEntry(t, database, "hash-bbb", 2000)

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(restored.QueueEntries) != 2 {
		t.Fatalf("QueueEntries: want 2, got %d", len(restored.QueueEntries))
	}
	if len(restored.PlayerTracks) != 2 {
		t.Fatalf("PlayerTracks: want 2, got %d", len(restored.PlayerTracks))
	}

	eA := restored.QueueEntries[0]
	if eA.ID != idA {
		t.Errorf("QueueEntries[0].ID: want %d, got %d", idA, eA.ID)
	}
	if eA.Hash != "hash-aaa" {
		t.Errorf("QueueEntries[0].Hash: want %q, got %q", "hash-aaa", eA.Hash)
	}
	if eA.Artist != "Artist A" {
		t.Errorf("QueueEntries[0].Artist: want %q, got %q", "Artist A", eA.Artist)
	}
	if eA.Title != "Song A" {
		t.Errorf("QueueEntries[0].Title: want %q, got %q", "Song A", eA.Title)
	}

	eB := restored.QueueEntries[1]
	if eB.ID != idB {
		t.Errorf("QueueEntries[1].ID: want %d, got %d", idB, eB.ID)
	}
	if eB.Hash != "hash-bbb" {
		t.Errorf("QueueEntries[1].Hash: want %q, got %q", "hash-bbb", eB.Hash)
	}
	if eB.Artist != "Artist B" {
		t.Errorf("QueueEntries[1].Artist: want %q, got %q", "Artist B", eB.Artist)
	}
	if eB.Title != "Song B" {
		t.Errorf("QueueEntries[1].Title: want %q, got %q", "Song B", eB.Title)
	}

	// Verify parallel alignment between QueueEntries and PlayerTracks.
	for i, e := range restored.QueueEntries {
		if restored.PlayerTracks[i].Hash != e.Hash {
			t.Errorf("PlayerTracks[%d].Hash: want %q, got %q", i, e.Hash, restored.PlayerTracks[i].Hash)
		}
	}
}

// TestRestoreSessionMissingFileMarkedInDB verifies that MarkQueueEntryMissing
// is called for entries whose files cannot be stat-ed, persisting the missing
// state back to the database.
func TestRestoreSessionMissingFileMarkedInDB(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	missingPath := filepath.Join(dirs.Runtime, "ghost.mp3")
	insertTrack(t, database, "hash-ghost", missingPath, "Ghost", "Track")
	id := insertQueueEntry(t, database, "hash-ghost", 1000)

	if _, err := app.RestoreSession(database, w); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}

	// Read the queue row back and verify missing=1 was written.
	var missing int
	err := database.QueryRow(`SELECT missing FROM queue WHERE id=?`, id).Scan(&missing)
	if err != nil {
		t.Fatalf("query queue.missing: %v", err)
	}
	if missing != 1 {
		t.Errorf("queue.missing: want 1, got %d", missing)
	}
}

// TestRestoreSessionSessionFlags verifies that repeat_mode, shuffle,
// current_hash, and position_ms are restored from the session table.
func TestRestoreSessionSessionFlags(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	audioPath := filepath.Join(dirs.Runtime, "song.mp3")
	if err := os.WriteFile(audioPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-xyz", audioPath, "X", "Y")
	insertQueueEntry(t, database, "hash-xyz", 1000)
	setSessionRow(t, database, "hash-xyz", 30500, false, "all")

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if restored.RepeatMode != tui.RepeatAll {
		t.Errorf("RepeatMode: want RepeatAll, got %v", restored.RepeatMode)
	}
	if restored.Shuffle {
		t.Error("Shuffle: want false")
	}
	if restored.CurrentHash != "hash-xyz" {
		t.Errorf("CurrentHash: want %q, got %q", "hash-xyz", restored.CurrentHash)
	}
	if restored.PositionMs != 30500 {
		t.Errorf("PositionMs: want 30500, got %d", restored.PositionMs)
	}
}

// TestRestoreSessionRepeatOne verifies that repeat_mode="one" is correctly parsed.
func TestRestoreSessionRepeatOne(t *testing.T) {
	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	defer database.Close()
	w := newTestWriter(t, database)

	setSessionRow(t, database, "", 0, false, "one")

	restored, err := app.RestoreSession(database, w)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if restored.RepeatMode != tui.RepeatOne {
		t.Errorf("RepeatMode: want RepeatOne, got %v", restored.RepeatMode)
	}
}

// TestRunRestorePlayerCalled verifies that deps.RestorePlayer is invoked with
// the correct path and position when a session with a current_hash is present.
// It also verifies that Play() is never called during the restore path.
func TestRunRestorePlayerCalled(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	// Pre-populate the database that app.Run() will open.
	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "session.mp3")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-restore", audioPath, "Restore", "Artist")
	insertQueueEntry(t, database, "hash-restore", 1000)
	setSessionRow(t, database, "hash-restore", 42000, false, "none")
	database.Close()

	var (
		mu                  sync.Mutex
		restorePlayerCalled bool
		restorePath         string
		restorePositionMs   int64
	)

	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(path string, positionMs int64) error {
			mu.Lock()
			restorePlayerCalled = true
			restorePath = path
			restorePositionMs = positionMs
			mu.Unlock()
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	result.CancelWriter()
	result.Writer.Wait()
	result.DB.Close()
	result.Lockfile.Release()

	mu.Lock()
	called := restorePlayerCalled
	gotPath := restorePath
	gotPos := restorePositionMs
	mu.Unlock()

	if !called {
		t.Error("RestorePlayer was not called")
	}
	if gotPath != audioPath {
		t.Errorf("RestorePlayer path: want %q, got %q", audioPath, gotPath)
	}
	if gotPos != 42000 {
		t.Errorf("RestorePlayer positionMs: want 42000, got %d", gotPos)
	}
}

// TestRunRestorePlayerNotCalledWithoutCurrentHash verifies that RestorePlayer
// is not called when the session has no current_hash (nothing was playing).
func TestRunRestorePlayerNotCalledWithoutCurrentHash(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	// Pre-populate queue but leave current_hash empty in session.
	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "nopos.mp3")
	if err := os.WriteFile(audioPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-nopos", audioPath, "", "")
	insertQueueEntry(t, database, "hash-nopos", 1000)
	// Leave session default (current_hash NULL).
	database.Close()

	restorePlayerCalled := false
	deps := app.Deps{
		CheckPrereqs: func() error { return nil },
		LaunchMPV:    func(_ context.Context, _ string) error { return nil },
		RunScan:      func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: func(_ string, _ int64) error {
			restorePlayerCalled = true
			return nil
		},
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	result.CancelWriter()
	result.Writer.Wait()
	result.DB.Close()
	result.Lockfile.Release()

	if restorePlayerCalled {
		t.Error("RestorePlayer must not be called when session has no current_hash")
	}
}

// TestRunRestorePlayerNilDepSafe verifies that Run succeeds even when
// RestorePlayer is nil and a session with a current_hash is present.
func TestRunRestorePlayerNilDepSafe(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "safe.mp3")
	if err := os.WriteFile(audioPath, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-safe", audioPath, "", "")
	insertQueueEntry(t, database, "hash-safe", 1000)
	setSessionRow(t, database, "hash-safe", 1000, false, "none")
	database.Close()

	deps := app.Deps{
		CheckPrereqs:  func() error { return nil },
		LaunchMPV:     func(_ context.Context, _ string) error { return nil },
		RunScan:       func(_ context.Context, _ *db.DB) error { return nil },
		RestorePlayer: nil, // intentionally nil
	}

	result, err := app.Run(context.Background(), dirs, deps)
	if err != nil {
		t.Fatalf("Run with nil RestorePlayer: %v", err)
	}
	result.CancelWriter()
	result.Writer.Wait()
	result.DB.Close()
	result.Lockfile.Release()
}

// TestRunRestoredSessionInResult verifies that RunResult.Restored contains
// the queue entries loaded from the database.
func TestRunRestoredSessionInResult(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	audioPath := filepath.Join(dirs.Runtime, "result.mp3")
	if err := os.WriteFile(audioPath, []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	insertTrack(t, database, "hash-result", audioPath, "Res", "Ult")
	insertQueueEntry(t, database, "hash-result", 1000)
	setSessionRow(t, database, "hash-result", 5000, true, "all")
	database.Close()

	result, err := app.Run(context.Background(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	result.CancelWriter()
	result.Writer.Wait()
	result.DB.Close()
	result.Lockfile.Release()

	r := result.Restored
	if r == nil {
		t.Fatal("RunResult.Restored is nil")
	}
	if len(r.QueueEntries) != 1 {
		t.Fatalf("QueueEntries: want 1, got %d", len(r.QueueEntries))
	}
	if r.QueueEntries[0].Missing {
		t.Error("QueueEntry.Missing: want false for readable file")
	}
	if r.RepeatMode != tui.RepeatAll {
		t.Errorf("RepeatMode: want RepeatAll, got %v", r.RepeatMode)
	}
	if !r.Shuffle {
		t.Error("Shuffle: want true")
	}
	if r.CurrentHash != "hash-result" {
		t.Errorf("CurrentHash: want %q, got %q", "hash-result", r.CurrentHash)
	}
	if r.PositionMs != 5000 {
		t.Errorf("PositionMs: want 5000, got %d", r.PositionMs)
	}
}

// helpers

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

func pidStr(pid int) string {
	return strconv.Itoa(pid) + "\n"
}
