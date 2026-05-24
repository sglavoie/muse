package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestOpen_WALMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var mode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpen_BusyTimeout(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var timeout int
	if err := d.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestOpen_PragmasApplyToPooledConnections(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	d.SetMaxOpenConns(2)

	ctx := context.Background()
	conn, err := d.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer conn.Close()

	var pinnedFK int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&pinnedFK); err != nil {
		t.Fatalf("query pinned foreign_keys: %v", err)
	}
	if pinnedFK != 1 {
		t.Fatalf("pinned foreign_keys = %d, want 1", pinnedFK)
	}

	var pooledFK int
	if err := d.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&pooledFK); err != nil {
		t.Fatalf("query pooled foreign_keys: %v", err)
	}
	if pooledFK != 1 {
		t.Errorf("pooled foreign_keys = %d, want 1", pooledFK)
	}

	var pooledTimeout int
	if err := d.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&pooledTimeout); err != nil {
		t.Fatalf("query pooled busy_timeout: %v", err)
	}
	if pooledTimeout != 5000 {
		t.Errorf("pooled busy_timeout = %d, want 5000", pooledTimeout)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	_, err := db.Open("/nonexistent/directory/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestReadQueue_Empty(t *testing.T) {
	d := openMigratedDB(t)

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestReadQueue_PartiallySoftDeleted(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hashA", "/music/a.flac")
	insertTrack(t, d, "hashB", "/music/b.flac")

	// Insert two active entries.
	insertQueueEntry(t, d, "hashA", 1000)
	qidB := insertQueueEntry(t, d, "hashB", 2000)

	// Soft-delete entry B.
	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE queue SET deleted_at = ? WHERE id = ?`, now, qidB); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Hash != "hashA" {
		t.Errorf("got hash %q, want %q", rows[0].Hash, "hashA")
	}
}

func TestReadSession_NoRow(t *testing.T) {
	d := openMigratedDB(t)

	// Remove the seeded session row to simulate a missing row.
	if _, err := d.Exec(`DELETE FROM session WHERE id = 1`); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row != nil {
		t.Errorf("got non-nil row, want nil when no session row exists")
	}
}

func TestReadSession_Default(t *testing.T) {
	d := openMigratedDB(t)

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row == nil {
		t.Fatal("got nil row, want seeded session row")
	}
	if row.RepeatMode != "none" {
		t.Errorf("RepeatMode = %q, want %q", row.RepeatMode, "none")
	}
	if row.Shuffle {
		t.Errorf("Shuffle = true, want false")
	}
	if row.CurrentHash != "" {
		t.Errorf("CurrentHash = %q, want empty", row.CurrentHash)
	}
	if row.PositionMs != 0 {
		t.Errorf("PositionMs = %d, want 0", row.PositionMs)
	}
}

func TestReadPlaylists_Empty(t *testing.T) {
	d := openMigratedDB(t)

	rows, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestReadPlaylists_AlphabeticalOrder(t *testing.T) {
	d := openMigratedDB(t)

	insertPlaylist(t, d, "Zebra")
	insertPlaylist(t, d, "Alpha")
	insertPlaylist(t, d, "mellow") // lowercase to verify NOCASE ordering

	rows, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	want := []string{"Alpha", "mellow", "Zebra"}
	for i, w := range want {
		if rows[i].Name != w {
			t.Errorf("rows[%d].Name = %q, want %q", i, rows[i].Name, w)
		}
		if rows[i].TrackCount != 0 {
			t.Errorf("rows[%d].TrackCount = %d, want 0", i, rows[i].TrackCount)
		}
	}
}

func TestReadPlaylists_TrackCount(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hashA", "/music/a.flac")
	insertTrack(t, d, "hashB", "/music/b.flac")
	pid := insertPlaylist(t, d, "My Playlist")
	insertPlaylistTrack(t, d, pid, "hashA", 1000)
	insertPlaylistTrack(t, d, pid, "hashB", 2000)

	rows, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].TrackCount != 2 {
		t.Errorf("TrackCount = %d, want 2", rows[0].TrackCount)
	}
}

func TestReadPlaylists_TrackCountExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hashA", "/music/a.flac")
	insertTrack(t, d, "hashB", "/music/b.flac")
	pid := insertPlaylist(t, d, "My Playlist")
	insertPlaylistTrack(t, d, pid, "hashA", 1000)
	insertPlaylistTrack(t, d, pid, "hashB", 2000)

	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = ?`, "hashB"); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	rows, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].TrackCount != 1 {
		t.Errorf("TrackCount = %d, want 1 (missing playlist tracks should not count as playable)", rows[0].TrackCount)
	}
}

func TestReadPlaylists_ExcludesSoftDeleted(t *testing.T) {
	d := openMigratedDB(t)

	insertPlaylist(t, d, "Active")
	softDeletedID := insertPlaylist(t, d, "Deleted")

	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE playlists SET deleted_at = ? WHERE id = ?`, now, softDeletedID); err != nil {
		t.Fatalf("soft-delete playlist: %v", err)
	}

	rows, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Name != "Active" {
		t.Errorf("got name %q, want %q", rows[0].Name, "Active")
	}
}

func TestReadPlaylistTracks_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hash1", "/music/one.flac")
	insertTrack(t, d, "hash2", "/music/two.flac")
	pid := insertPlaylist(t, d, "My Playlist")
	insertPlaylistTrack(t, d, pid, "hash1", 1000)
	insertPlaylistTrack(t, d, pid, "hash2", 2000)

	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = ?`, "hash2"); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	rows, err := d.ReadPlaylistTracks(pid)
	if err != nil {
		t.Fatalf("ReadPlaylistTracks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (missing playlist tracks should be hidden)", len(rows))
	}
	if rows[0].Hash != "hash1" {
		t.Errorf("got hash %q, want hash1", rows[0].Hash)
	}
}

func TestReadPlaylistTracks_Empty(t *testing.T) {
	d := openMigratedDB(t)
	pid := insertPlaylist(t, d, "Empty Playlist")

	rows, err := d.ReadPlaylistTracks(pid)
	if err != nil {
		t.Fatalf("ReadPlaylistTracks: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestReadPlaylistTracks_AscendingPosition(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hash1", "/music/one.flac")
	insertTrack(t, d, "hash2", "/music/two.flac")
	insertTrack(t, d, "hash3", "/music/three.flac")
	pid := insertPlaylist(t, d, "My Playlist")
	// Insert out of order to verify sorting.
	insertPlaylistTrack(t, d, pid, "hash2", 2000)
	insertPlaylistTrack(t, d, pid, "hash1", 1000)
	insertPlaylistTrack(t, d, pid, "hash3", 3000)

	rows, err := d.ReadPlaylistTracks(pid)
	if err != nil {
		t.Fatalf("ReadPlaylistTracks: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	wantPositions := []int64{1000, 2000, 3000}
	for i, want := range wantPositions {
		if rows[i].Position != want {
			t.Errorf("rows[%d].Position = %d, want %d", i, rows[i].Position, want)
		}
	}
}

func TestReadPlaylistTracks_Metadata(t *testing.T) {
	d := openMigratedDB(t)

	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, title, artist, duration_ms, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"hashM", "/music/meta.flac", "Song Title", "Artist Name", 180000, now, now,
	); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	pid := insertPlaylist(t, d, "Meta Playlist")
	insertPlaylistTrack(t, d, pid, "hashM", 1000)

	rows, err := d.ReadPlaylistTracks(pid)
	if err != nil {
		t.Fatalf("ReadPlaylistTracks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Position != 1000 {
		t.Errorf("Position = %d, want 1000", r.Position)
	}
	if r.Title != "Song Title" {
		t.Errorf("Title = %q, want %q", r.Title, "Song Title")
	}
	if r.Artist != "Artist Name" {
		t.Errorf("Artist = %q, want %q", r.Artist, "Artist Name")
	}
	if r.DurationMs != 180000 {
		t.Errorf("DurationMs = %d, want 180000", r.DurationMs)
	}
}

func TestWALMode_DurabilityAfterAbruptClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "wal_durability.db")

	// Open and migrate so the tracks table exists.
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Insert rows inside an explicit transaction and commit without checkpointing.
	now := time.Now().Unix()
	type rowData struct{ hash, path string }
	wantRows := []rowData{
		{"wal-hash-1", "/music/wal-1.flac"},
		{"wal-hash-2", "/music/wal-2.flac"},
		{"wal-hash-3", "/music/wal-3.flac"},
	}

	tx, err := d.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, r := range wantRows {
		if _, err := tx.Exec(
			`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			r.hash, r.path, now, now,
		); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert %s: %v", r.hash, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Close without issuing PRAGMA wal_checkpoint — simulates abrupt connection close.
	d.Close()

	// Reopen with a fresh db.Open call.
	d2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()

	// All committed rows must be present — check count.
	var count int
	if err := d2.QueryRow(`SELECT COUNT(*) FROM tracks WHERE hash LIKE 'wal-hash-%'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != len(wantRows) {
		t.Errorf("row count after reopen = %d, want %d", count, len(wantRows))
	}

	// Verify a field value to confirm data integrity, not just row presence.
	var gotPath string
	if err := d2.QueryRow(`SELECT path FROM tracks WHERE hash = ?`, "wal-hash-1").Scan(&gotPath); err != nil {
		t.Fatalf("field query: %v", err)
	}
	if gotPath != "/music/wal-1.flac" {
		t.Errorf("path = %q, want %q", gotPath, "/music/wal-1.flac")
	}
}

func TestReadSession_WithValues(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hashX", "/music/x.flac")

	if _, err := d.Exec(`
		UPDATE session SET repeat_mode='all', shuffle=1, current_hash='hashX', position_ms=42000, updated_at=?
		WHERE id=1`, time.Now().Unix()); err != nil {
		t.Fatalf("update session: %v", err)
	}

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row == nil {
		t.Fatal("got nil row")
	}
	if row.RepeatMode != "all" {
		t.Errorf("RepeatMode = %q, want %q", row.RepeatMode, "all")
	}
	if !row.Shuffle {
		t.Errorf("Shuffle = false, want true")
	}
	if row.CurrentHash != "hashX" {
		t.Errorf("CurrentHash = %q, want %q", row.CurrentHash, "hashX")
	}
	if row.PositionMs != 42000 {
		t.Errorf("PositionMs = %d, want 42000", row.PositionMs)
	}
}
