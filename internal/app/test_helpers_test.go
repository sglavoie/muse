package app_test

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
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
		CheckPrereqs: func(context.Context) ([]string, error) { return nil, nil },
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

func createSocketFile(t *testing.T, path string) net.Listener {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("Listen(unix, %q): %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		_ = ln.Close()
		t.Fatalf("socket path %q missing after listen: %v", path, err)
	}
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.Remove(path)
	})
	return ln
}

// createStaleSocketFile leaves behind exactly what a crashed muse leaves behind
// when its mpv child is already gone: a socket file with nothing listening on
// it. Go unlinks the socket on listener Close by default, so unlinking is
// disabled first. Use this (rather than createSocketFile) for anything that
// reaches app.Run's stale-socket step — a still-live listener there is an
// *orphaned mpv*, and Run now spends its full reap timeout trying to quit it.
func createStaleSocketFile(t *testing.T, path string) {
	t.Helper()
	ln := createSocketFile(t, path)
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener for %q: %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale socket file %q missing after close: %v", path, err)
	}
}

func shortTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".apptest-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}
