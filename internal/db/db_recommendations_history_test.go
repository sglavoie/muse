package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestHasSufficientHistory_Empty(t *testing.T) {
	d := openMigratedDB(t)

	ok, err := d.HasSufficientHistory(20)
	if err != nil {
		t.Fatalf("HasSufficientHistory: %v", err)
	}
	if ok {
		t.Error("HasSufficientHistory = true, want false for empty history")
	}
}

func TestHasSufficientHistory_Below20(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	for i := range 19 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}

	ok, err := d.HasSufficientHistory(20)
	if err != nil {
		t.Fatalf("HasSufficientHistory: %v", err)
	}
	if ok {
		t.Error("HasSufficientHistory = true, want false for 19 plays")
	}
}

func TestHasSufficientHistory_ExactlyAt20(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	for i := range 20 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}

	ok, err := d.HasSufficientHistory(20)
	if err != nil {
		t.Fatalf("HasSufficientHistory: %v", err)
	}
	if !ok {
		t.Error("HasSufficientHistory = false, want true for exactly 20 plays")
	}
}

func TestReadTopArtists_Empty(t *testing.T) {
	d := openMigratedDB(t)

	artists, err := d.ReadTopArtists(5)
	if err != nil {
		t.Fatalf("ReadTopArtists: %v", err)
	}
	if len(artists) != 0 {
		t.Errorf("ReadTopArtists = %v, want empty", artists)
	}
}

func TestReadTopArtists_RankedByPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistB", "AlbumB", "Pop", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 3 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen h1: %v", err)
		}
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 10, ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	artists, err := d.ReadTopArtists(5)
	if err != nil {
		t.Fatalf("ReadTopArtists: %v", err)
	}
	if len(artists) < 2 {
		t.Fatalf("ReadTopArtists returned %d artists, want >= 2", len(artists))
	}
	if artists[0] != "ArtistA" {
		t.Errorf("artists[0] = %q, want ArtistA", artists[0])
	}
	if artists[1] != "ArtistB" {
		t.Errorf("artists[1] = %q, want ArtistB", artists[1])
	}
}

// The "Unknown Artist" placeholder and blank artists are missing tags, not a
// taste: they must never lead the top-artists profile however often untagged
// files are played.
func TestReadTopArtists_ExcludesUnknownAndBlankArtists(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", library.UnknownArtist, "", "", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "", "", "", 180000, 5000000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "Song3", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 5 {
		for _, h := range []string{"h1", "h2"} {
			if err := w.RecordListen(db.PlayRecord{Hash: h, PlayedAt: now + int64(i), ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
				t.Fatalf("RecordListen %s: %v", h, err)
			}
		}
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h3", PlayedAt: now + 10, ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen h3: %v", err)
	}

	artists, err := d.ReadTopArtists(5)
	if err != nil {
		t.Fatalf("ReadTopArtists: %v", err)
	}
	if len(artists) != 1 || artists[0] != "ArtistA" {
		t.Errorf("ReadTopArtists = %q, want [ArtistA]", artists)
	}
}

func TestReadTopGenres_Empty(t *testing.T) {
	d := openMigratedDB(t)

	genres, err := d.ReadTopGenres(5)
	if err != nil {
		t.Fatalf("ReadTopGenres: %v", err)
	}
	if len(genres) != 0 {
		t.Errorf("ReadTopGenres = %v, want empty", genres)
	}
}

func TestReadTopGenres_UsesTrackGenresTable(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistB", "AlbumB", "Pop", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen h1: %v", err)
		}
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now + 10, ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h1", "Rock"); err != nil {
		t.Fatalf("insert track_genres h1: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Pop"); err != nil {
		t.Fatalf("insert track_genres h2: %v", err)
	}

	genres, err := d.ReadTopGenres(5)
	if err != nil {
		t.Fatalf("ReadTopGenres: %v", err)
	}
	if len(genres) < 2 {
		t.Fatalf("ReadTopGenres returned %d genres, want >= 2", len(genres))
	}
	if genres[0] != "Rock" {
		t.Errorf("genres[0] = %q, want Rock", genres[0])
	}
	if genres[1] != "Pop" {
		t.Errorf("genres[1] = %q, want Pop", genres[1])
	}
}

func TestReadCompletionRates_NilInput(t *testing.T) {
	d := openMigratedDB(t)

	rates, err := d.ReadCompletionRates(nil)
	if err != nil {
		t.Fatalf("ReadCompletionRates(nil): %v", err)
	}
	if len(rates) != 0 {
		t.Errorf("ReadCompletionRates(nil) = %v, want empty", rates)
	}
}

func TestReadCompletionRates_Averages(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)
	now := time.Now().Unix()

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen 0.5: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + 1, ListenedSecs: 120, DurationSecs: 120, Counted: true}); err != nil {
		t.Fatalf("RecordListen 1.0: %v", err)
	}

	rates, err := d.ReadCompletionRates([]string{"h1"})
	if err != nil {
		t.Fatalf("ReadCompletionRates: %v", err)
	}
	if rates["h1"] != 0.75 {
		t.Errorf("completion rate for h1 = %f, want 0.75", rates["h1"])
	}
}

func TestReadCompletionRates_MissingHashAbsent(t *testing.T) {
	d := openMigratedDB(t)

	rates, err := d.ReadCompletionRates([]string{"nonexistent"})
	if err != nil {
		t.Fatalf("ReadCompletionRates: %v", err)
	}
	if _, ok := rates["nonexistent"]; ok {
		t.Error("nonexistent hash should be absent from result")
	}
}

func TestReadCompletionRates_ZeroDuration(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)
	now := time.Now().Unix()

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 0, Counted: true}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}

	rates, err := d.ReadCompletionRates([]string{"h1"})
	if err != nil {
		t.Fatalf("ReadCompletionRates: %v", err)
	}
	if rates["h1"] != 0.0 {
		t.Errorf("completion rate for zero-duration track = %f, want 0.0", rates["h1"])
	}
}
