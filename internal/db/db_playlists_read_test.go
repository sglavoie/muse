package db_test

import (
	"testing"
	"time"
)

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
	insertPlaylist(t, d, "mellow")

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
