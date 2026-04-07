package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestIncrementPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hashPC", "/pc.flac")

	w := newWriter(t, d)

	if err := w.IncrementPlayCount("hashPC"); err != nil {
		t.Fatalf("IncrementPlayCount: %v", err)
	}
	if err := w.IncrementPlayCount("hashPC"); err != nil {
		t.Fatalf("IncrementPlayCount (2nd): %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT play_count FROM tracks WHERE hash = 'hashPC'`).Scan(&count); err != nil {
		t.Fatalf("query play_count: %v", err)
	}
	if count != 2 {
		t.Errorf("play_count = %d, want 2", count)
	}
}

func TestIncrementPlayCount_NoopForUnknownHash(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	if err := w.IncrementPlayCount("nonexistent"); err != nil {
		t.Fatalf("IncrementPlayCount for unknown hash: %v", err)
	}
}

func TestCreateSmartPlaylist(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "test"},
	}
	raw, err := db.MarshalSmartPlaylistQuery(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	id, err := w.CreateSmartPlaylist("My Smart PL", raw)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("got %d playlists, want 1", len(playlists))
	}
	pl := playlists[0]
	if !pl.IsSmart {
		t.Error("IsSmart = false, want true")
	}
	if pl.TrackCount != -1 {
		t.Errorf("TrackCount = %d, want -1", pl.TrackCount)
	}
	if pl.Name != "My Smart PL" {
		t.Errorf("Name = %q, want %q", pl.Name, "My Smart PL")
	}
}

func TestUpdateSmartPlaylistQuery(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	q1 := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpEquals, Value: "A"},
	}
	raw1, _ := db.MarshalSmartPlaylistQuery(q1)
	id, err := w.CreateSmartPlaylist("PL", raw1)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}

	q2 := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldGenre, Operator: db.OpEquals, Value: "jazz"},
	}
	raw2, _ := db.MarshalSmartPlaylistQuery(q2)
	if err := w.UpdateSmartPlaylistQuery(id, "PL-Renamed", raw2); err != nil {
		t.Fatalf("UpdateSmartPlaylistQuery: %v", err)
	}

	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("got %d playlists, want 1", len(playlists))
	}
	if playlists[0].Query != raw2 {
		t.Errorf("Query = %q, want %q", playlists[0].Query, raw2)
	}
	if playlists[0].Name != "PL-Renamed" {
		t.Errorf("Name = %q, want %q", playlists[0].Name, "PL-Renamed")
	}
}

func TestReadPlaylists_IsSmart(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	_, err := w.CreatePlaylist("Regular")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "x"},
	}
	raw, _ := db.MarshalSmartPlaylistQuery(q)
	_, err = w.CreateSmartPlaylist("Smart", raw)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}

	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(playlists) != 2 {
		t.Fatalf("got %d playlists, want 2", len(playlists))
	}

	reg := playlists[0]
	smart := playlists[1]
	if reg.IsSmart {
		t.Errorf("Regular playlist: IsSmart = true, want false")
	}
	if reg.TrackCount != 0 {
		t.Errorf("Regular playlist: TrackCount = %d, want 0", reg.TrackCount)
	}
	if !smart.IsSmart {
		t.Errorf("Smart playlist: IsSmart = false, want true")
	}
	if smart.TrackCount != -1 {
		t.Errorf("Smart playlist: TrackCount = %d, want -1", smart.TrackCount)
	}
}
