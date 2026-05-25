package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
)

type restoreSessionHarness struct {
	t        *testing.T
	dirs     config.Dirs
	database *db.DB
	writer   *db.Writer
}

func newRestoreSessionHarness(t *testing.T) *restoreSessionHarness {
	t.Helper()

	dirs := setupDirs(t)
	database := openTestDB(t, dirs)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("close test DB: %v", err)
		}
	})

	return &restoreSessionHarness{
		t:        t,
		dirs:     dirs,
		database: database,
		writer:   newTestWriter(t, database),
	}
}

func (h *restoreSessionHarness) path(name string) string {
	h.t.Helper()
	return filepath.Join(h.dirs.Runtime, name)
}

func (h *restoreSessionHarness) writeAudioFile(name string) string {
	h.t.Helper()

	path := h.path(name)
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return path
}

func (h *restoreSessionHarness) insertTrackAndQueue(hash, path, artist, title string, position int64) int64 {
	h.t.Helper()
	insertTrack(h.t, h.database, hash, path, artist, title)
	return insertQueueEntry(h.t, h.database, hash, position)
}

func (h *restoreSessionHarness) setSession(hash string, positionMs int64, shuffle bool, repeatMode string) {
	h.t.Helper()
	setSessionRow(h.t, h.database, hash, positionMs, shuffle, repeatMode)
}

func (h *restoreSessionHarness) restore() *app.RestoredSession {
	h.t.Helper()

	restored, err := app.RestoreSession(h.database, h.writer)
	if err != nil {
		h.t.Fatalf("RestoreSession: %v", err)
	}
	return restored
}

func (h *restoreSessionHarness) markQueueMissing(id int64, missing int) {
	h.t.Helper()
	if _, err := h.database.Exec(`UPDATE queue SET missing=? WHERE id=?`, missing, id); err != nil {
		h.t.Fatalf("mark queue missing: %v", err)
	}
}

func (h *restoreSessionHarness) markTrackMissing(hash string, missing int) {
	h.t.Helper()
	if _, err := h.database.Exec(`UPDATE tracks SET missing=? WHERE hash=?`, missing, hash); err != nil {
		h.t.Fatalf("mark track missing: %v", err)
	}
}

func (h *restoreSessionHarness) queueMissing(id int64) int {
	h.t.Helper()
	return h.readMissing(`SELECT missing FROM queue WHERE id=?`, id)
}

func (h *restoreSessionHarness) trackMissing(hash string) int {
	h.t.Helper()
	return h.readMissing(`SELECT missing FROM tracks WHERE hash=?`, hash)
}

func (h *restoreSessionHarness) readMissing(query string, arg any) int {
	h.t.Helper()

	var missing int
	if err := h.database.QueryRow(query, arg).Scan(&missing); err != nil {
		h.t.Fatalf("query missing flag: %v", err)
	}
	return missing
}
