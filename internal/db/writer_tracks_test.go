package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestUpsertBatch verifies that UpsertBatch persists tracks to the tracks table
// and that AppendTrack does not return a FK constraint error after the batch is
// committed.
func TestUpsertBatch(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	tracks := []library.Track{
		{Hash: "hash-a1", Path: "/music/a.mp3", Title: "Track A", Artist: "Artist A", Album: "Album A", Duration: 180.5},
		{Hash: "hash-b2", Path: "/music/b.flac", Title: "Track B", Artist: "Artist B", Album: "Album B", Duration: 240.0},
	}

	if err := w.UpsertBatch(tracks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tracks WHERE hash IN ('hash-a1', 'hash-b2')`).Scan(&count); err != nil {
		t.Fatalf("count tracks: %v", err)
	}
	if count != 2 {
		t.Errorf("tracks count = %d; want 2", count)
	}

	var path, title, artist, album string
	var durationMs int64
	if err := d.QueryRow(
		`SELECT path, title, artist, album, duration_ms FROM tracks WHERE hash='hash-a1'`,
	).Scan(&path, &title, &artist, &album, &durationMs); err != nil {
		t.Fatalf("select track: %v", err)
	}
	if path != "/music/a.mp3" {
		t.Errorf("path = %q; want /music/a.mp3", path)
	}
	if title != "Track A" {
		t.Errorf("title = %q; want Track A", title)
	}
	if durationMs != 180500 {
		t.Errorf("duration_ms = %d; want 180500", durationMs)
	}

	if _, err := w.AppendTrack(tracks[0], ""); err != nil {
		t.Errorf("AppendTrack after UpsertBatch: %v", err)
	}
}

// TestUpsertBatch_Idempotent verifies that calling UpsertBatch twice with the
// same hash updates the row in place without cascading a delete on any FK
// dependants.
func TestUpsertBatch_Idempotent(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	original := library.Track{Hash: "hash-idem", Path: "/old.mp3", Title: "Old Title", Artist: "Old Artist", Album: "Old Album", Duration: 60.0}
	if err := w.UpsertBatch([]library.Track{original}); err != nil {
		t.Fatalf("first UpsertBatch: %v", err)
	}

	queueID, err := w.AppendTrack(original, "")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	updated := library.Track{Hash: "hash-idem", Path: "/new.mp3", Title: "New Title", Artist: "New Artist", Album: "New Album", Duration: 90.0}
	if err := w.UpsertBatch([]library.Track{updated}); err != nil {
		t.Fatalf("second UpsertBatch: %v", err)
	}

	var path, title string
	if err := d.QueryRow(`SELECT path, title FROM tracks WHERE hash='hash-idem'`).Scan(&path, &title); err != nil {
		t.Fatalf("select: %v", err)
	}
	if path != "/new.mp3" {
		t.Errorf("path = %q; want /new.mp3", path)
	}
	if title != "New Title" {
		t.Errorf("title = %q; want New Title", title)
	}

	var queueCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM queue WHERE id=?`, queueID).Scan(&queueCount); err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queueCount != 1 {
		t.Errorf("queue row count = %d after second upsert; want 1 (no cascade delete)", queueCount)
	}
}

// TestUpsertBatch_Empty verifies that UpsertBatch returns nil without error
// when given an empty slice.
func TestUpsertBatch_Empty(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	if err := w.UpsertBatch(nil); err != nil {
		t.Errorf("UpsertBatch(nil): %v", err)
	}
	if err := w.UpsertBatch([]library.Track{}); err != nil {
		t.Errorf("UpsertBatch([]): %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("tracks count = %d; want 0", count)
	}
}

// TestUpdateTrackMetadata verifies that UpdateTrackMetadata sets the title,
// artist, album, and user_edited flag on the tracks row.
func TestUpdateTrackMetadata(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	track := library.Track{Hash: "hash-meta", Path: "/a.mp3", Title: "Old", Artist: "Old A", Album: "Old Al", Duration: 60.0}
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	if err := w.UpdateTrackMetadata("hash-meta", "New Title", "New Artist", "New Album", ""); err != nil {
		t.Fatalf("UpdateTrackMetadata: %v", err)
	}

	var title, artist, album string
	var userEdited int
	if err := d.QueryRow(
		`SELECT title, artist, album, user_edited FROM tracks WHERE hash='hash-meta'`,
	).Scan(&title, &artist, &album, &userEdited); err != nil {
		t.Fatalf("select: %v", err)
	}
	if title != "New Title" {
		t.Errorf("title = %q; want New Title", title)
	}
	if artist != "New Artist" {
		t.Errorf("artist = %q; want New Artist", artist)
	}
	if album != "New Album" {
		t.Errorf("album = %q; want New Album", album)
	}
	if userEdited != 1 {
		t.Errorf("user_edited = %d; want 1", userEdited)
	}
}

// TestUpsertBatch_PreservesUserEdited verifies that UpsertBatch does not
// overwrite title/artist/album when user_edited=1.
func TestUpsertBatch_PreservesUserEdited(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	original := library.Track{Hash: "hash-ue", Path: "/a.mp3", Title: "Original", Artist: "Original A", Album: "Original Al", Duration: 60.0}
	if err := w.UpsertBatch([]library.Track{original}); err != nil {
		t.Fatalf("first UpsertBatch: %v", err)
	}

	if err := w.UpdateTrackMetadata("hash-ue", "Edited Title", "Edited Artist", "Edited Album", ""); err != nil {
		t.Fatalf("UpdateTrackMetadata: %v", err)
	}

	rescanned := library.Track{Hash: "hash-ue", Path: "/a-moved.mp3", Title: "Scanned", Artist: "Scanned A", Album: "Scanned Al", Duration: 90.0}
	if err := w.UpsertBatch([]library.Track{rescanned}); err != nil {
		t.Fatalf("second UpsertBatch: %v", err)
	}

	var path, title, artist, album string
	var durationMs int64
	if err := d.QueryRow(
		`SELECT path, title, artist, album, duration_ms FROM tracks WHERE hash='hash-ue'`,
	).Scan(&path, &title, &artist, &album, &durationMs); err != nil {
		t.Fatalf("select: %v", err)
	}

	if path != "/a-moved.mp3" {
		t.Errorf("path = %q; want /a-moved.mp3 (physical field should update)", path)
	}
	if durationMs != 90000 {
		t.Errorf("duration_ms = %d; want 90000 (physical field should update)", durationMs)
	}
	if title != "Edited Title" {
		t.Errorf("title = %q; want Edited Title (user-edited metadata should be preserved)", title)
	}
	if artist != "Edited Artist" {
		t.Errorf("artist = %q; want Edited Artist (user-edited metadata should be preserved)", artist)
	}
	if album != "Edited Album" {
		t.Errorf("album = %q; want Edited Album (user-edited metadata should be preserved)", album)
	}
}

// TestUpsertBatch_PreservesNonZeroDuration verifies that a rescan with
// duration=0 does not overwrite an existing non-zero duration in the database.
func TestUpsertBatch_PreservesNonZeroDuration(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	initial := library.Track{Hash: "hash-dur", Path: "/a.mp3", Title: "A", Duration: 120.5}
	if err := w.UpsertBatch([]library.Track{initial}); err != nil {
		t.Fatalf("first UpsertBatch: %v", err)
	}

	rescanned := library.Track{Hash: "hash-dur", Path: "/a.mp3", Title: "A", Duration: 0}
	if err := w.UpsertBatch([]library.Track{rescanned}); err != nil {
		t.Fatalf("second UpsertBatch: %v", err)
	}

	var durationMs int64
	if err := d.QueryRow(
		`SELECT duration_ms FROM tracks WHERE hash='hash-dur'`,
	).Scan(&durationMs); err != nil {
		t.Fatalf("select: %v", err)
	}
	if durationMs != 120500 {
		t.Errorf("duration_ms = %d; want 120500 (non-zero duration should be preserved when scanner returns 0)", durationMs)
	}
}

// TestUpsertBatch_UpdatesNonZeroDuration verifies that a rescan with a new
// non-zero duration does overwrite the existing duration.
func TestUpsertBatch_UpdatesNonZeroDuration(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	initial := library.Track{Hash: "hash-dur2", Path: "/b.mp3", Title: "B", Duration: 60.0}
	if err := w.UpsertBatch([]library.Track{initial}); err != nil {
		t.Fatalf("first UpsertBatch: %v", err)
	}

	rescanned := library.Track{Hash: "hash-dur2", Path: "/b.mp3", Title: "B", Duration: 75.0}
	if err := w.UpsertBatch([]library.Track{rescanned}); err != nil {
		t.Fatalf("second UpsertBatch: %v", err)
	}

	var durationMs int64
	if err := d.QueryRow(
		`SELECT duration_ms FROM tracks WHERE hash='hash-dur2'`,
	).Scan(&durationMs); err != nil {
		t.Fatalf("select: %v", err)
	}
	if durationMs != 75000 {
		t.Errorf("duration_ms = %d; want 75000 (non-zero duration should update)", durationMs)
	}
}

// TestReadUserEditedTracks verifies that ReadUserEditedTracks returns only
// non-missing tracks whose metadata was manually edited.
func TestReadUserEditedTracks(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	tracks := []library.Track{
		{Hash: "hash-a", Path: "/a.mp3", Title: "A", Artist: "Art A", Album: "Alb A", Genre: "Rock"},
		{Hash: "hash-b", Path: "/b.mp3", Title: "B", Artist: "Art B", Album: "Alb B", Genre: "Pop"},
		{Hash: "hash-c", Path: "/c.mp3", Title: "C", Artist: "Art C", Album: "Alb C", Genre: "Jazz"},
	}
	if err := w.UpsertBatch(tracks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	if err := w.UpdateTrackMetadata("hash-a", "Edited A", "Edited Art A", "Edited Alb A", "Edited Rock"); err != nil {
		t.Fatalf("UpdateTrackMetadata(a): %v", err)
	}
	if err := w.UpdateTrackMetadata("hash-c", "Edited C", "Edited Art C", "Edited Alb C", "Edited Jazz"); err != nil {
		t.Fatalf("UpdateTrackMetadata(c): %v", err)
	}
	if err := w.MarkTrackMissing("hash-c"); err != nil {
		t.Fatalf("MarkTrackMissing(c): %v", err)
	}

	rows, err := d.ReadUserEditedTracks()
	if err != nil {
		t.Fatalf("ReadUserEditedTracks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows; want 1", len(rows))
	}
	r := rows[0]
	if r.Hash != "hash-a" {
		t.Errorf("Hash = %q; want hash-a", r.Hash)
	}
	if r.Title != "Edited A" {
		t.Errorf("Title = %q; want Edited A", r.Title)
	}
	if r.Artist != "Edited Art A" {
		t.Errorf("Artist = %q; want Edited Art A", r.Artist)
	}
	if r.Album != "Edited Alb A" {
		t.Errorf("Album = %q; want Edited Alb A", r.Album)
	}
	if r.Genre != "Edited Rock" {
		t.Errorf("Genre = %q; want Edited Rock", r.Genre)
	}
}
