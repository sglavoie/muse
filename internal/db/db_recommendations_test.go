package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// trackHashes returns the Hash field of each track for use in error messages.
func trackHashes(tracks []library.Track) []string {
	out := make([]string, len(tracks))
	for i, t := range tracks {
		out[i] = t.Hash
	}
	return out
}

// --- HasSufficientHistory ---

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
		if err := w.RecordPlay("h1", now+int64(i), 60, 120); err != nil {
			t.Fatalf("RecordPlay: %v", err)
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
		if err := w.RecordPlay("h1", now+int64(i), 60, 120); err != nil {
			t.Fatalf("RecordPlay: %v", err)
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

// --- ReadTopArtists / ReadTopGenres ---

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
		if err := w.RecordPlay("h1", now+int64(i), 60, 180); err != nil {
			t.Fatalf("RecordPlay h1: %v", err)
		}
	}
	if err := w.RecordPlay("h2", now+10, 60, 180); err != nil {
		t.Fatalf("RecordPlay h2: %v", err)
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
		if err := w.RecordPlay("h1", now+int64(i), 60, 180); err != nil {
			t.Fatalf("RecordPlay h1: %v", err)
		}
	}
	if err := w.RecordPlay("h2", now+10, 60, 180); err != nil {
		t.Fatalf("RecordPlay h2: %v", err)
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

// --- ReadCompletionRates ---

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

	// 0.5 and 1.0 → avg = 0.75
	if err := w.RecordPlay("h1", now, 60, 120); err != nil {
		t.Fatalf("RecordPlay 0.5: %v", err)
	}
	if err := w.RecordPlay("h1", now+1, 120, 120); err != nil {
		t.Fatalf("RecordPlay 1.0: %v", err)
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

	if err := w.RecordPlay("h1", now, 60, 0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	rates, err := d.ReadCompletionRates([]string{"h1"})
	if err != nil {
		t.Fatalf("ReadCompletionRates: %v", err)
	}
	if rates["h1"] != 0.0 {
		t.Errorf("completion rate for zero-duration track = %f, want 0.0", rates["h1"])
	}
}

// --- ReadRediscoverMix ---

func TestReadRediscoverMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadRediscoverMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadRediscoverMix_ExcludesRecentPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "Rock", 180000, 5000000, now)

	// h1: 10 plays, last played 91 days ago → should appear
	ninetyOneDaysAgo := now - 91*86400
	if _, err := d.Exec(`UPDATE tracks SET play_count = 10, last_played_at = ? WHERE hash = ?`, ninetyOneDaysAgo, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}
	// h2: 10 plays, last played recently → should NOT appear
	if _, err := d.Exec(`UPDATE tracks SET play_count = 10, last_played_at = ? WHERE hash = ?`, now, "h2"); err != nil {
		t.Fatalf("update h2: %v", err)
	}

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadRediscoverMix returned %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadRediscoverMix_RequiresMinPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)

	// play_count = 9 (below threshold of 10), old enough
	oldEnough := now - 91*86400
	if _, err := d.Exec(`UPDATE tracks SET play_count = 9, last_played_at = ? WHERE hash = ?`, oldEnough, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadRediscoverMix returned %d tracks, want 0 (play_count below threshold)", len(tracks))
	}
}

// --- ReadOnRepeatMix ---

func TestReadOnRepeatMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadOnRepeatMix_RequiresMinRecentPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordPlay("h1", now-int64(i*3600), 60, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix = %d tracks, want 0 (only 2 recent plays)", len(tracks))
	}
}

func TestReadOnRepeatMix_Returns3PlusRecentPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 3 {
		if err := w.RecordPlay("h1", now-int64(i*3600), 60, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadOnRepeatMix returned %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadOnRepeatMix_ExcludesOldPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)

	// 3 plays but all 8 days ago (outside 7-day window)
	eightDaysAgo := now - 8*86400
	for i := range 3 {
		if _, err := d.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			"h1", eightDaysAgo+int64(i), 60,
		); err != nil {
			t.Fatalf("insert play_history: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix = %d tracks, want 0 (all plays outside window)", len(tracks))
	}
}

// --- ReadDeepCutsMix ---

func TestReadDeepCutsMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadDeepCutsMix_HighCompletionRate(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	// 90% completion each
	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 162, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadDeepCutsMix = %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadDeepCutsMix_LowCompletionRateExcluded(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	// 50% completion → avg = 0.5, below 0.8 threshold
	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 90, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0 (low completion rate)", len(tracks))
	}
}

func TestReadDeepCutsMix_ExcludesHighPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 162, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	// play_count = 6 (above max of 5)
	if _, err := d.Exec(`UPDATE tracks SET play_count = 6 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0 (play_count > 5)", len(tracks))
	}
}

// --- ReadNeverPlayedMix ---

func TestReadNeverPlayedMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadNeverPlayedMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadNeverPlayedMix_ExcludesPlayedTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadNeverPlayedMix = %d tracks, want 0 (track has been played)", len(tracks))
	}
}

func TestReadNeverPlayedMix_ArtistAffinityRanking(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	// h1: played track by ArtistA
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	// h2: never-played, same artist (ArtistA) → higher affinity
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	// h3: never-played, different artist → lower affinity
	insertFullTrack(t, d, "h3", "/c.flac", "Song3", "ArtistB", "AlbumB", "Jazz", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) < 2 {
		t.Fatalf("ReadNeverPlayedMix returned %d tracks, want >= 2", len(tracks))
	}
	if tracks[0].Hash != "h2" {
		t.Errorf("tracks[0].Hash = %q, want h2 (higher artist affinity)", tracks[0].Hash)
	}
}

func TestReadNeverPlayedMix_NoGenreTrack(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	// Track with no genre should still appear
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "", 180000, 5000000, now)

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadNeverPlayedMix = %v, want [h1]", trackHashes(tracks))
	}
}

// --- ReadHiddenGemsMix ---

func TestReadHiddenGemsMix_NoHistory(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadHiddenGemsMix = %d tracks, want 0 (no history)", len(tracks))
	}
}

func TestReadHiddenGemsMix_MatchesByArtist(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "Song3", "ArtistZ", "Album", "Jazz", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1 play_count: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	foundH2 := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			foundH2 = true
		}
		if tr.Hash == "h3" {
			t.Errorf("ReadHiddenGemsMix returned h3 (no affinity), want excluded")
		}
	}
	if !foundH2 {
		t.Errorf("ReadHiddenGemsMix did not return h2 (matching artist)")
	}
}

func TestReadHiddenGemsMix_MatchesByGenre(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistZ", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay h1: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1 play_count: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = ?`, "h2"); err != nil {
		t.Fatalf("update h2 play_count: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h1", "Rock"); err != nil {
		t.Fatalf("insert track_genres h1: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Rock"); err != nil {
		t.Fatalf("insert track_genres h2: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	found := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			found = true
		}
	}
	if !found {
		t.Errorf("ReadHiddenGemsMix did not return h2 (genre match via track_genres)")
	}
}

func TestReadHiddenGemsMix_ExcludesHighPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 5 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}
	// h2 has play_count = 3 (above threshold of 2)
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h2"); err != nil {
		t.Fatalf("update h2: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			t.Errorf("ReadHiddenGemsMix returned h2 (play_count=3 > 2), want excluded")
		}
	}
}

func TestReadHiddenGemsMix_MultiGenreTrack(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	// h1: played, genre Rock
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	// h2: play_count=0, has genres Rock and Jazz
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistZ", "Album", "Rock; Jazz", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1 play_count: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h1", "Rock"); err != nil {
		t.Fatalf("insert track_genres h1: %v", err)
	}
	// h2 has two genres
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Rock"); err != nil {
		t.Fatalf("insert track_genres h2 Rock: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Jazz"); err != nil {
		t.Fatalf("insert track_genres h2 Jazz: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	found := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			found = true
		}
	}
	if !found {
		t.Errorf("ReadHiddenGemsMix did not return multi-genre track h2")
	}
}

// --- ReadSessionCoOccurrences ---

func TestReadSessionCoOccurrences_Empty(t *testing.T) {
	d := openMigratedDB(t)

	results, err := d.ReadSessionCoOccurrences("nonexistent", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ReadSessionCoOccurrences = %v, want empty", results)
	}
}

func TestReadSessionCoOccurrences_SelfExclusion(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	for _, r := range results {
		if r.Hash == "seed" {
			t.Error("ReadSessionCoOccurrences returned seed hash, want self-excluded")
		}
	}
}

func TestReadSessionCoOccurrences_SessionBoundary(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "same", "/same.flac", "Same", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "other", "/other.flac", "Other", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	// seed and same share a session (plays 60s apart, within 1800s gap).
	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed: %v", err)
	}
	if err := w.RecordPlay("same", now+60, 60, 180); err != nil {
		t.Fatalf("RecordPlay same: %v", err)
	}
	// other is in a different session (> 1800s gap from the last play).
	if err := w.RecordPlay("other", now+2000, 60, 180); err != nil {
		t.Fatalf("RecordPlay other: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	found := make(map[string]bool)
	for _, r := range results {
		found[r.Hash] = true
	}
	if !found["same"] {
		t.Error("ReadSessionCoOccurrences: expected 'same' in results (same session as seed)")
	}
	if found["other"] {
		t.Error("ReadSessionCoOccurrences: 'other' should not be in results (different session)")
	}
}

func TestReadSessionCoOccurrences_CountOrdering(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "frequent", "/frequent.flac", "Freq", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "rare", "/rare.flac", "Rare", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	// Session 1: seed+frequent+rare (plays within 1800s gap).
	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed 1: %v", err)
	}
	if err := w.RecordPlay("frequent", now+10, 60, 180); err != nil {
		t.Fatalf("RecordPlay frequent 1: %v", err)
	}
	if err := w.RecordPlay("rare", now+20, 60, 180); err != nil {
		t.Fatalf("RecordPlay rare: %v", err)
	}
	// Session 2: seed+frequent only (> 1800s gap forces a new session).
	if err := w.RecordPlay("seed", now+2000, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed 2: %v", err)
	}
	if err := w.RecordPlay("frequent", now+2010, 60, 180); err != nil {
		t.Fatalf("RecordPlay frequent 2: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("ReadSessionCoOccurrences returned empty results")
	}
	if results[0].Hash != "frequent" {
		t.Errorf("results[0].Hash = %q, want frequent (higher co-occurrence count)", results[0].Hash)
	}
}

// --- ReadHourGenreAffinities ---

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
	// Only 2 plays — below the HAVING COUNT(*) >= 3 threshold.
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
	// Rock: 5 plays; Jazz: 3 plays — both above threshold; Rock should rank first.
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
