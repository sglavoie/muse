package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestAlbumArtistSurvivesStorageAndHistory(t *testing.T) {
	d := openMigratedTestDB(t)
	w := newWriter(t, d)
	track := library.Track{Hash: "album-artist", Path: "/compilation/song.flac", Title: "Original", Artist: "Guest", Album: "Compilation",
		AlbumArtist: "Various Artists", Duration: 180}
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatal(err)
	}
	if err := w.UpdateTrackMetadata(track.Hash, "Edited", track.Artist, track.Album, "Jazz"); err != nil {
		t.Fatal(err)
	}
	track.AlbumArtist = "Corrected Release Artist"
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatal(err)
	}
	rows, err := d.ReadTracks()
	if err != nil || len(rows) != 1 || rows[0].AlbumArtist != track.AlbumArtist || rows[0].Title != "Edited" || rows[0].Genre != "Jazz" {
		t.Fatalf("rescan failed to refresh album artist while preserving edits: %+v, %v", rows, err)
	}
	row, found, err := d.ReadTrackByPath(track.Path)
	if err != nil || !found || row.AlbumArtist != track.AlbumArtist {
		t.Fatalf("path lookup lost album artist: %+v, %v", row, err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: track.Hash, PlayedAt: time.Now().Unix(), ListenedSecs: 100, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatal(err)
	}
	history, err := d.ReadPlayHistory(10)
	if err != nil || len(history) != 1 || history[0].AlbumArtist != track.AlbumArtist {
		t.Fatalf("history lost album artist: %+v, %v", history, err)
	}
}

func TestAlbumArtistMigrationRefreshesTagsWithoutLosingEdits(t *testing.T) {
	d := openMigratedTestDB(t)
	migrateDownTo(t, d, 26)
	if _, err := d.Exec(`INSERT INTO tracks (hash, path, title, album, mod_time, user_edited, created_at, updated_at) VALUES ('old', '/old.flac', 'Edited', 'Album', 123, 1, 100, 100)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(d.DB); err != nil {
		t.Fatal(err)
	}
	rows, err := d.ReadTracks()
	if err != nil || len(rows) != 1 || rows[0].ModTime != 0 || rows[0].AlbumArtist != "" || rows[0].Title != "Edited" {
		t.Fatalf("migration did not preserve existing metadata and request a tag refresh: %+v, %v", rows, err)
	}
}

func TestAlbumArtistEditsSurviveRescanAndRestart(t *testing.T) {
	d := openMigratedTestDB(t)
	w := newWriter(t, d)
	track := library.Track{Hash: "album-edit", Path: "/a.flac", Title: "Song", Album: "Compilation", AlbumArtist: "File Artist"}
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"Various Artists", ""} {
		if err := w.UpdateTrackMetadata(track.Hash, track.Title, track.Artist, track.Album, track.Genre, value); err != nil {
			t.Fatal(err)
		}
		if err := w.UpsertBatch([]library.Track{track}); err != nil {
			t.Fatal(err)
		}
		rows, err := d.ReadTracks()
		if err != nil || len(rows) != 1 || rows[0].AlbumArtist != value {
			t.Fatalf("rescan overwrote %q: %+v %v", value, rows, err)
		}
		edited, err := d.ReadUserEditedTracks()
		if err != nil || len(edited) != 1 || edited[0].AlbumArtist != value || !edited[0].AlbumArtistEdited {
			t.Fatalf("restart overlay lost %q: %+v %v", value, edited, err)
		}
	}
}
