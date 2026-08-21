package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
)

type insightTrack struct {
	hash      string
	artist    string
	album     string
	title     string
	duration  int
	playCount int
	missing   int
	genres    []string
}

func insertInsightTrack(t *testing.T, d *db.DB, tr insightTrack) {
	t.Helper()
	if _, err := d.Exec(`
		INSERT INTO tracks (hash, path, title, artist, album, genre, duration_ms,
		                    play_count, missing, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, 1700000000, 1700000000)`,
		tr.hash, "/"+tr.hash+".flac", tr.title, tr.artist, tr.album,
		tr.duration*1000, tr.playCount, tr.missing,
	); err != nil {
		t.Fatalf("insert track %s: %v", tr.hash, err)
	}
	for _, g := range tr.genres {
		if _, err := d.Exec(
			`INSERT INTO track_genres (track_hash, genre) VALUES (?, ?)`, tr.hash, g,
		); err != nil {
			t.Fatalf("insert genre %q for %s: %v", g, tr.hash, err)
		}
	}
}

// insertInsightPlay records a play the way the app does: playedAt is when the
// play *ended*.
func insertInsightPlay(t *testing.T, d *db.DB, hash string, playedAt int64, listenedSecs int, completion float64, sessionID string) {
	t.Helper()
	if _, err := d.Exec(`
		INSERT INTO play_events (track_hash, played_at, listened_secs, completion_rate, session_id)
		VALUES (?, ?, ?, ?, ?)`,
		hash, playedAt, listenedSecs, completion, sessionID,
	); err != nil {
		t.Fatalf("insert play for %s: %v", hash, err)
	}
}

// insertInsightSkip records an abandoned listen: logged in play_events but
// filtered out of the play_history view, so only readers that ask for the full
// log see it.
func insertInsightSkip(t *testing.T, d *db.DB, hash string, playedAt int64, listenedSecs int, completion float64) {
	t.Helper()
	if _, err := d.Exec(`
		INSERT INTO play_events (track_hash, played_at, listened_secs, completion_rate, session_id, counted)
		VALUES (?, ?, ?, ?, NULL, 0)`,
		hash, playedAt, listenedSecs, completion,
	); err != nil {
		t.Fatalf("insert skip for %s: %v", hash, err)
	}
}

func TestReadInsightPlaysCorrectsPlayedAtToPlayStart(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "Toumani", album: "Kaira", title: "Cantelowes", duration: 600, playCount: 1})

	const endedAt = 1774020300 // the play ENDED here
	insertInsightPlay(t, d, "a", endedAt, 600, 0.9, "s1")

	plays, err := d.ReadInsightPlays(0)
	if err != nil {
		t.Fatalf("ReadInsightPlays: %v", err)
	}
	if len(plays) != 1 {
		t.Fatalf("plays = %d, want 1", len(plays))
	}
	got := plays[0]
	if got.StartAt != endedAt-600 {
		t.Errorf("StartAt = %d, want %d (played_at - listened_secs)", got.StartAt, endedAt-600)
	}
	want := insights.PlayRow{
		Hash: "a", StartAt: endedAt - 600, ListenedSecs: 600, CompletionRate: 0.9,
		SessionID: "s1", Artist: "Toumani", Album: "Kaira", Title: "Cantelowes",
	}
	if got != want {
		t.Errorf("play = %+v, want %+v", got, want)
	}
}

func TestReadInsightPlaysFiltersOnCorrectedStart(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 600, playCount: 2})

	const since = 1774000000
	// Ends 5 minutes after the cutoff but started 5 minutes before it, so the
	// listening happened outside the window.
	insertInsightPlay(t, d, "a", since+300, 600, 1, "s1")
	insertInsightPlay(t, d, "a", since+900, 300, 1, "s1")

	plays, err := d.ReadInsightPlays(since)
	if err != nil {
		t.Fatalf("ReadInsightPlays: %v", err)
	}
	if len(plays) != 1 {
		t.Fatalf("plays = %d, want 1; the window filter must use the corrected start", len(plays))
	}
	if plays[0].StartAt != since+600 {
		t.Errorf("StartAt = %d, want %d", plays[0].StartAt, since+600)
	}
}

// TestReadInsightGenresSplitsMultiGenreTracks is the junction-table invariant
// carried over from ReadListeningStats: genre aggregates must come from
// track_genres, never from the comma-separated tracks.genre column, or a track
// tagged "rock,pop" forms a composite bucket that hides both genres.
func TestReadInsightGenresSplitsMultiGenreTracks(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 300, playCount: 1, genres: []string{"rock", "pop"}})
	insertInsightTrack(t, d, insightTrack{hash: "b", artist: "B", duration: 300, playCount: 1, genres: []string{"rock"}})
	insertInsightPlay(t, d, "a", 1774000300, 300, 1, "s1")
	insertInsightPlay(t, d, "b", 1774000700, 300, 1, "s1")

	genres, err := d.ReadInsightGenres(0)
	if err != nil {
		t.Fatalf("ReadInsightGenres: %v", err)
	}

	seen := map[string]insights.GenreRow{}
	for _, g := range genres {
		seen[g.Genre] = g
	}
	if got := seen["rock"].Plays; got != 2 {
		t.Errorf("rock = %d plays, want 2 (multi-genre tracks count toward each genre)", got)
	}
	if got := seen["pop"].Plays; got != 1 {
		t.Errorf("pop = %d plays, want 1", got)
	}
	if _, ok := seen["rock,pop"]; ok {
		t.Error("found composite genre key \"rock,pop\"; expected per-genre keys only")
	}
	if got := seen["rock"].Tracks; got != 2 {
		t.Errorf("rock = %d distinct tracks, want 2", got)
	}
	if got := seen["rock"].ListenedSecs; got != 600 {
		t.Errorf("rock = %d listened secs, want 600", got)
	}
}

func TestReadInsightGenresPreservesCaseVariants(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 300, playCount: 1, genres: []string{"Ambient"}})
	insertInsightTrack(t, d, insightTrack{hash: "b", artist: "B", duration: 300, playCount: 1, genres: []string{"ambient"}})
	insertInsightPlay(t, d, "a", 1774000300, 300, 1, "s1")
	insertInsightPlay(t, d, "b", 1774000700, 300, 1, "s1")

	genres, err := d.ReadInsightGenres(0)
	if err != nil {
		t.Fatalf("ReadInsightGenres: %v", err)
	}

	// The reader must not fold the collision itself: Data Health needs the raw
	// spellings to offer an exact merge.
	if len(genres) != 2 {
		t.Fatalf("genres = %+v, want both spellings preserved", genres)
	}
	groups := insights.Compute(insights.Input{Genres: genres}).Quality.Collisions
	if len(groups) != 1 || len(groups[0].Variants) != 2 {
		t.Errorf("collisions = %+v, want one group of two variants", groups)
	}
}

func TestReadInsightSessionsDerivesStartAndEnds(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "Toumani", title: "Kaira", duration: 600, playCount: 1})
	insertInsightTrack(t, d, insightTrack{hash: "b", artist: "AWV", title: "Atomos", duration: 300, playCount: 1})

	const firstEnd = 1774000600
	insertInsightPlay(t, d, "a", firstEnd, 600, 1, "s1")     // 1774000000 -> 1774000600
	insertInsightPlay(t, d, "b", firstEnd+300, 300, 1, "s1") // 1774000600 -> 1774000900
	insertInsightPlay(t, d, "b", 1774100300, 300, 1, "s2")

	sessions, err := d.ReadInsightSessions(0)
	if err != nil {
		t.Fatalf("ReadInsightSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}

	want := insights.SessionRow{
		ID: "s1", StartedAt: firstEnd - 600, EndedAt: firstEnd + 300,
		Plays: 2, ListenedSecs: 900, DistinctArtists: 2,
		OpenerArtist: "Toumani", OpenerTitle: "Kaira",
		CloserArtist: "AWV", CloserTitle: "Atomos",
	}
	if sessions[0] != want {
		t.Errorf("session = %+v, want %+v", sessions[0], want)
	}
}

func TestReadInsightSessionsFiltersOnCorrectedStart(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 600, playCount: 1})

	const since = 1774000000
	insertInsightPlay(t, d, "a", since+300, 600, 1, "s1") // started before the cutoff
	insertInsightPlay(t, d, "a", since+900, 300, 1, "s2") // started after it

	sessions, err := d.ReadInsightSessions(since)
	if err != nil {
		t.Fatalf("ReadInsightSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s2" {
		t.Errorf("sessions = %+v, want only s2", sessions)
	}
}

func TestReadInsightTimelineFlagsFirstEverPlays(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 300, playCount: 2})
	insertInsightTrack(t, d, insightTrack{hash: "b", artist: "B", duration: 300, playCount: 1})
	insertInsightPlay(t, d, "a", 1774000300, 300, 1, "s1")
	insertInsightPlay(t, d, "b", 1774000700, 300, 1, "s1")
	insertInsightPlay(t, d, "a", 1774001100, 300, 1, "s1")

	timeline, err := d.ReadInsightTimeline()
	if err != nil {
		t.Fatalf("ReadInsightTimeline: %v", err)
	}
	want := []insights.TimelineRow{
		{Hash: "a", StartAt: 1774000000, ListenedSecs: 300, FirstEver: true},
		{Hash: "b", StartAt: 1774000400, ListenedSecs: 300, FirstEver: true},
		{Hash: "a", StartAt: 1774000800, ListenedSecs: 300, FirstEver: false},
	}
	if len(timeline) != len(want) {
		t.Fatalf("timeline = %+v, want %d rows", timeline, len(want))
	}
	for i := range want {
		if timeline[i] != want[i] {
			t.Errorf("timeline[%d] = %+v, want %+v", i, timeline[i], want[i])
		}
	}
}

func TestReadInsightCoverageSkipsMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", album: "One", duration: 300, playCount: 3})
	insertInsightTrack(t, d, insightTrack{hash: "b", artist: "A", album: "One", duration: 240})
	insertInsightTrack(t, d, insightTrack{hash: "c", artist: "C", album: "Two", duration: 900, missing: 1})

	rows, err := d.ReadInsightCoverage()
	if err != nil {
		t.Fatalf("ReadInsightCoverage: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the two non-missing tracks", rows)
	}
	byHash := map[string]insights.CoverageRow{}
	for _, r := range rows {
		byHash[r.Hash] = r
	}
	if got := byHash["b"]; got.PlayCount != 0 || got.DurationSecs != 240 || got.Artist != "A" || got.Album != "One" {
		t.Errorf("track b = %+v, want an unheard 240 s track by A on One", got)
	}
}

func TestReadInsightQualityCounts(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 300, playCount: 3, genres: []string{"rock"}})
	insertInsightTrack(t, d, insightTrack{hash: "b", artist: "B", duration: 300, playCount: 2})
	insertInsightTrack(t, d, insightTrack{hash: "c", artist: "C", duration: 300, missing: 1})
	insertInsightPlay(t, d, "a", 1774000300, 300, 1.0, "s1")
	insertInsightPlay(t, d, "b", 1774000700, 300, 0.9, "s1")
	insertInsightPlay(t, d, "b", 1774001100, 60, 0.2, "s1")

	q, err := d.ReadInsightQuality(1, 5, 0.8)
	if err != nil {
		t.Fatalf("ReadInsightQuality: %v", err)
	}

	want := insights.QualityCounts{
		TracksTotal:        2,
		TracksMissing:      1,
		TracksWithoutGenre: 1,
		PlaysTotal:         3,
		PlaysWithoutGenre:  2,
		DeepCutsCandidates: 2,
		DeepCutsQualifying: 1, // track b averages 0.55 and drops out
	}
	if q != want {
		t.Errorf("quality = %+v, want %+v", q, want)
	}
}

func TestReadInsightQualityCountsSkippedListens(t *testing.T) {
	d := openMigratedDB(t)
	insertInsightTrack(t, d, insightTrack{hash: "a", artist: "A", duration: 300, playCount: 1, genres: []string{"rock"}})
	insertInsightPlay(t, d, "a", 1774000300, 300, 1.0, "s1")
	insertInsightSkip(t, d, "a", 1774000400, 6, 0.02)
	insertInsightSkip(t, d, "a", 1774000500, 6, 0.02)

	q, err := d.ReadInsightQuality(1, 5, 0.8)
	if err != nil {
		t.Fatalf("ReadInsightQuality: %v", err)
	}

	want := insights.QualityCounts{
		TracksTotal:        1,
		PlaysTotal:         1, // the counted-only view still reports one play
		SkipsRecorded:      2,
		DeepCutsCandidates: 1,
		// Read through play_history the average would be a flat 1.0; over the
		// full log the two skips pull it to 0.35 and the filter bites.
		DeepCutsQualifying: 0,
	}
	if q != want {
		t.Errorf("quality = %+v, want %+v", q, want)
	}
}

func TestInsightReadersOnAnEmptyLibrary(t *testing.T) {
	d := openMigratedDB(t)

	plays, err := d.ReadInsightPlays(0)
	if err != nil || plays != nil {
		t.Errorf("ReadInsightPlays = %v, %v; want nil, nil", plays, err)
	}
	genres, err := d.ReadInsightGenres(0)
	if err != nil || genres != nil {
		t.Errorf("ReadInsightGenres = %v, %v; want nil, nil", genres, err)
	}
	sessions, err := d.ReadInsightSessions(0)
	if err != nil || sessions != nil {
		t.Errorf("ReadInsightSessions = %v, %v; want nil, nil", sessions, err)
	}
	coverage, err := d.ReadInsightCoverage()
	if err != nil || coverage != nil {
		t.Errorf("ReadInsightCoverage = %v, %v; want nil, nil", coverage, err)
	}
	timeline, err := d.ReadInsightTimeline()
	if err != nil || timeline != nil {
		t.Errorf("ReadInsightTimeline = %v, %v; want nil, nil", timeline, err)
	}
	quality, err := d.ReadInsightQuality(1, 5, 0.8)
	if err != nil || quality != (insights.QualityCounts{}) {
		t.Errorf("ReadInsightQuality = %+v, %v; want zero, nil", quality, err)
	}

	if snap := insights.Compute(insights.Input{}); snap.TotalPlays != 0 {
		t.Errorf("Compute of an empty library reports %d plays", snap.TotalPlays)
	}
}
