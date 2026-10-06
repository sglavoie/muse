package db_test

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"testing"
)

func requireAlbumFavorites(t *testing.T, d *db.DB, tracks ...library.Track) {
	t.Helper()
	rows, err := d.ReadFavoritesByType("album")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, track := range tracks {
		want[library.AlbumKey(track)] = true
	}
	if len(rows) != len(want) {
		t.Fatalf("favorites = %+v; want %v", rows, want)
	}
	for _, row := range rows {
		if !want[row.Value] {
			t.Fatalf("unexpected favorite %s", row.Value)
		}
	}
	_, count, _, err := d.ReadFavoriteCounts()
	if err != nil || count != len(want) {
		t.Fatalf("album count=%d, err=%v", count, err)
	}
}

func TestLegacyAlbumFavoriteSplitsWithoutLosingOtherReleases(t *testing.T) {
	d := openMigratedTestDB(t)
	w := newWriter(t, d)
	a := library.Track{Hash: "a", Path: "/a/song.flac", Album: "Hits",
		AlbumArtist: "A"}
	b := library.Track{Hash: "b", Path: "/b/song.flac", Album: "Hits",
		AlbumArtist: "B"}
	if err := w.UpsertBatch([]library.Track{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFavorite("album", "Hits"); err != nil {
		t.Fatal(err)
	}
	requireAlbumFavorites(t, d, a, b)
	if yes, err := d.IsFavorite("album", library.AlbumKey(a)); err != nil || !yes {
		t.Fatalf("legacy favorite not resolved: %v", err)
	}
	if err := w.RemoveFavorite("album", library.AlbumKey(a)); err != nil {
		t.Fatal(err)
	}
	requireAlbumFavorites(t, d, b)
	if err := w.AddFavorite("album", library.AlbumKey(a)); err != nil {
		t.Fatal(err)
	}
	requireAlbumFavorites(t, d, a, b)
}

func TestAlbumFavoritesFollowRenameSplitMergeAndRescan(t *testing.T) {
	d := openMigratedTestDB(t)
	w := newWriter(t, d)
	a := library.Track{Hash: "a", Path: "/mix/a.flac", Artist: "A", Album: "Mix"}
	b := library.Track{Hash: "b", Path: "/mix/b.flac", Artist: "B", Album: "Mix"}
	if err := w.UpsertBatch([]library.Track{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFavorite("album", "Mix"); err != nil {
		t.Fatal(err)
	}
	if err := w.UpdateTrackMetadata(a.Hash, "", a.Artist, "Renamed", ""); err != nil {
		t.Fatal(err)
	}
	a.Album = "Renamed"
	requireAlbumFavorites(t, d, a, b)
	if err := w.UpdateTrackMetadata(b.Hash, "", b.Artist, "Renamed", ""); err != nil {
		t.Fatal(err)
	}
	b.Album = "Renamed"
	requireAlbumFavorites(t, d, a)
	// Rescanning supplies the release tag and moves files; user edits survive.
	a.AlbumArtist = "Various Artists"
	b.AlbumArtist = a.AlbumArtist
	a.Path = "/moved/a.flac"
	b.Path = "/moved/b.flac"
	if err := w.UpsertBatch([]library.Track{a, b}); err != nil {
		t.Fatal(err)
	}
	requireAlbumFavorites(t, d, a)
	// Undo the album rename through the same metadata writer.
	for _, track := range []library.Track{a, b} {
		if err := w.UpdateTrackMetadata(track.Hash, "", track.Artist, "Mix", ""); err != nil {
			t.Fatal(err)
		}
	}
	a.Album = "Mix"
	requireAlbumFavorites(t, d, a)
}

func TestLegacyUnknownAlbumFavoriteCanBeRemoved(t *testing.T) {
	d := openMigratedTestDB(t)
	w := newWriter(t, d)
	track := library.Track{Hash: "unknown", Path: "/unknown/a.flac"}
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatal(err)
	}
	if err := w.AddFavorite("album", "(unknown album)"); err != nil {
		t.Fatal(err)
	}
	requireAlbumFavorites(t, d, track)
	if err := w.RemoveFavorite("album", library.AlbumKey(track)); err != nil {
		t.Fatal(err)
	}
	requireAlbumFavorites(t, d)
}

func TestAlbumArtistInQueuePlaylistTagAndCoverageReads(t *testing.T) {
	d := openMigratedTestDB(t)
	w := newWriter(t, d)
	track := library.Track{Hash: "a", Path: "/mix/a.flac", Album: "Mix", Artist: "Guest", AlbumArtist: "Various Artists", Genre: "Jazz"}
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.AppendTrack(track, ""); err != nil {
		t.Fatal(err)
	}
	queue, err := d.ReadQueue()
	if err != nil || len(queue) != 1 || queue[0].AlbumArtist != track.AlbumArtist {
		t.Fatalf("queue: %+v, %v", queue, err)
	}
	playlist, err := w.CreatePlaylist("Mix")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddTracksToPlaylist(playlist, []string{track.Hash}); err != nil {
		t.Fatal(err)
	}
	rows, err := d.ReadPlaylistTracks(playlist)
	if err != nil || len(rows) != 1 || (rows[0].AlbumArtist != track.AlbumArtist || rows[0].Genre != track.Genre) {
		t.Fatalf("playlist: %+v, %v", rows, err)
	}
	tag, err := w.CreateTag("Tag", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddTrackTag(track.Hash, tag); err != nil {
		t.Fatal(err)
	}
	rows, err = d.ReadTracksByTag(tag)
	if err != nil || len(rows) != 1 || (rows[0].AlbumArtist != track.AlbumArtist || rows[0].Genre != track.Genre) {
		t.Fatalf("tag: %+v, %v", rows, err)
	}
	coverage, err := d.ReadInsightCoverage()
	if err != nil || len(coverage) != 1 || coverage[0].AlbumArtist != track.AlbumArtist || coverage[0].Path != track.Path {
		t.Fatalf("coverage: %+v, %v", coverage, err)
	}
}
