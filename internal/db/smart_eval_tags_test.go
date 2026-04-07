package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestEvalSmartPlaylist_HasTagEquals(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Tagged", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Untagged", "A", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "workout")
	tagTrack(t, w, "h1", tagID)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpEquals, Value: "workout"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %v, want [h1]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagNotEquals(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Tagged", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Untagged", "A", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "workout")
	tagTrack(t, w, "h1", tagID)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpNotEquals, Value: "workout"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %v, want [h2]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagSoftDeletedExcluded(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Tagged", "A", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "deleted-tag")
	tagTrack(t, w, "h1", tagID)
	if err := w.DeleteTagSoft(tagID); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpEquals, Value: "deleted-tag"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 0 {
		t.Errorf("got %v, want [] (soft-deleted tag should be excluded)", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagInAndGroup(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Both", "Radiohead", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "TagOnly", "Other", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "ArtistOnly", "Radiohead", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "chill")
	tagTrack(t, w, "h1", tagID)
	tagTrack(t, w, "h2", tagID)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjAnd,
			Rules: []db.SmartRule{
				{Field: db.FieldArtist, Operator: db.OpEquals, Value: "Radiohead"},
				{Field: db.FieldTag, Operator: db.OpEquals, Value: "chill"},
			},
		},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %v, want [h1]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagInOrGroup(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "TaggedA", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "TaggedB", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "NoTag", "A", "B", "rock", 200000, 1000, now)

	tagA := insertTag(t, d, w, "workout")
	tagB := insertTag(t, d, w, "driving")
	tagTrack(t, w, "h1", tagA)
	tagTrack(t, w, "h2", tagB)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjOr,
			Rules: []db.SmartRule{
				{Field: db.FieldTag, Operator: db.OpEquals, Value: "workout"},
				{Field: db.FieldTag, Operator: db.OpEquals, Value: "driving"},
			},
		},
	}
	rows := evalQuery(t, d, q)
	got := hashes(rows)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 rows", got)
	}
	want := map[string]bool{"h1": true, "h2": true}
	for _, h := range got {
		if !want[h] {
			t.Errorf("unexpected hash %q in results", h)
		}
	}
}
