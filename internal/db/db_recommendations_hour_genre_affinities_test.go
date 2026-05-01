package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestReadHourGenreAffinities_Empty(t *testing.T) {
	d := openMigratedDB(t)

	affinities, err := d.ReadHourGenreAffinities()
	if err != nil {
		t.Fatalf("ReadHourGenreAffinities: %v", err)
	}
	if len(affinities) != 0 {
		t.Errorf("ReadHourGenreAffinities = %v, want empty for no play history", affinities)
	}
}

func TestReadHourGenreAffinities_BelowThreshold(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h1", "Jazz"); err != nil {
		t.Fatalf("insert track_genres: %v", err)
	}
	w := newWriter(t, d)
	now := time.Now().Unix()
	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 60, 120); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}

	affinities, err := d.ReadHourGenreAffinities()
	if err != nil {
		t.Fatalf("ReadHourGenreAffinities: %v", err)
	}
	if len(affinities) != 0 {
		t.Errorf("ReadHourGenreAffinities: expected no rows below threshold (got %v)", affinities)
	}
}

func TestReadHourGenreAffinities_OrderedByCount(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	for _, args := range [][2]string{{"h1", "Rock"}, {"h2", "Jazz"}} {
		if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, args[0], args[1]); err != nil {
			t.Fatalf("insert track_genres: %v", err)
		}
	}
	w := newWriter(t, d)
	now := time.Now().Unix()
	for i := range 5 {
		if err := w.RecordPlay("h1", now+int64(i), 60, 120); err != nil {
			t.Fatalf("RecordPlay Rock: %v", err)
		}
	}
	for i := range 3 {
		if err := w.RecordPlay("h2", now+100+int64(i), 60, 120); err != nil {
			t.Fatalf("RecordPlay Jazz: %v", err)
		}
	}

	affinities, err := d.ReadHourGenreAffinities()
	if err != nil {
		t.Fatalf("ReadHourGenreAffinities: %v", err)
	}
	currentHour := time.Now().Hour()
	var hourRows []db.HourGenreAffinity
	for _, a := range affinities {
		if a.Hour == currentHour {
			hourRows = append(hourRows, a)
		}
	}
	if len(hourRows) < 2 {
		t.Fatalf("expected 2 genres for current hour, got %d: %v", len(hourRows), hourRows)
	}
	if hourRows[0].Genre != "Rock" {
		t.Errorf("first genre = %q, want Rock (higher count)", hourRows[0].Genre)
	}
	if hourRows[0].Count < hourRows[1].Count {
		t.Errorf("first count %d < second count %d; want descending order", hourRows[0].Count, hourRows[1].Count)
	}
}
