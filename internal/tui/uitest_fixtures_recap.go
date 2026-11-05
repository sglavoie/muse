package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
)

// The recap fixture carries a listening history long and varied enough for
// every page of the listening recap: two calendar years of plays, a clear top
// artist, a song first heard in 2025 that took over, one song played nine
// times in a day, a 23-day streak, and evenings that outweigh mornings.
//
// It is generated, but from a fixed seed and a fixed calendar: every play is
// built from local wall-clock fields, so the hours and days it lands on are
// the same in every time zone, and the recap's clock is pinned to
// recapFixtureNow so the default year never moves.

// recapFixtureNow is the instant the recap fixture's recap is computed at.
func recapFixtureNow() time.Time { return time.Date(2025, time.December, 20, 18, 0, 0, 0, time.Local) }

// recapFixtureTrack is one library track with how often the generator picks
// it and the days it can be picked on.
type recapFixtureTrack struct {
	track    library.Track
	weight   int
	from, to string // inclusive YYYY-MM-DD bounds; empty is unbounded
}

func recapFixtureCatalog() []recapFixtureTrack {
	t := func(hash, artist, album, title, genre string, secs float64, n int) library.Track {
		return library.Track{
			Hash: hash, Artist: artist, Album: album, Title: title, Genre: genre, Duration: secs, TrackNumber: n,
			Path: fmt.Sprintf("/music/recap/%s/%s/%02d.flac", artist, album, n),
		}
	}
	return []recapFixtureTrack{
		{track: t("rc01", "Aurora Fields", "Low Tide", "Tidepool", "Ambient", 240, 1), weight: 9},
		{track: t("rc02", "Aurora Fields", "Low Tide", "Saltwater Hymn", "Ambient", 300, 2), weight: 6},
		{track: t("rc03", "Aurora Fields", "Northern Lights", "Polar Night", "Ambient", 280, 1), weight: 3},
		{track: t("rc04", "Night Transit", "Neon Grid", "Midnight Line", "Techno", 210, 1), weight: 7},
		{track: t("rc05", "Night Transit", "Neon Grid", "Signal Fade", "Techno", 195, 2), weight: 4},
		{track: t("rc14", "Night Transit", "Neon Grid", "Overpass", "Techno", 230, 3), weight: 3, to: "2024-12-31"},
		{track: t("rc06", "Mara Quintet", "Blue Rooms", "Room 4", "Jazz", 360, 1), weight: 4},
		{track: t("rc07", "Mara Quintet", "Blue Rooms", "Late Set", "Jazz", 420, 2), weight: 3},
		{track: t("rc08", "Halcyon Drift", "Quiet Machines", "Servo Lullaby", "Electronic", 230, 1), weight: 5},
		{track: t("rc09", "Halcyon Drift", "Quiet Machines", "Clockwork Rain", "Electronic, Ambient", 250, 2), weight: 3},
		{track: t("rc10", "Juniper & Ash", "Hollow Pines", "Woodsmoke", "Folk", 200, 1), weight: 2},
		{track: t("rc11", "Juniper & Ash", "Hollow Pines", "River Stones", "Folk", 215, 2), weight: 2},
		{track: t("rc12", "Lumen", "Afterglow", "First Light", "Ambient", 260, 1), weight: 12, from: "2025-04-02"},
		{track: t("rc13", "Lumen", "Afterglow", "Ember", "Ambient", 245, 2), weight: 3, from: "2025-05-10"},
	}
}

// recapFixtureHours is the hour of day a play starts in, drawn uniformly: the
// repeats are what make the evening the peak.
var recapFixtureHours = []int{21, 21, 21, 22, 22, 20, 19, 8, 12, 17, 23, 7}

// recapFixtureRNG is a tiny linear congruential generator: the fixture needs
// a stable sequence, not good randomness.
type recapFixtureRNG struct{ state uint32 }

func (r *recapFixtureRNG) next(n int) int {
	r.state = r.state*1664525 + 1013904223
	return int(r.state>>8) % n
}

// recapFixtureHistory generates the fixture's plays, oldest first.
func recapFixtureHistory(catalog []recapFixtureTrack) []insights.PlayRow {
	rng := &recapFixtureRNG{state: 2025}
	var plays []insights.PlayRow
	start := time.Date(2024, time.March, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2025, time.December, 20, 0, 0, 0, 0, time.Local)
	for day := start; !day.After(end); day = time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, time.Local) {
		n := recapFixturePlaysOn(day, rng)
		plays = append(plays, recapFixtureDay(day, n, catalog, rng)...)
	}
	// One song on repeat: nine plays of Midnight Line on a Friday in March.
	repeat := time.Date(2025, time.March, 14, 20, 0, 0, 0, time.Local)
	for i := range 9 {
		plays = append(plays, recapFixturePlay(catalog[3].track, repeat.Add(time.Duration(i)*4*time.Minute), 210))
	}
	return plays
}

// recapFixturePlaysOn decides how many plays a day gets: sparse in 2024,
// most days in 2025, every day of the June streak and none either side of it.
func recapFixturePlaysOn(day time.Time, rng *recapFixtureRNG) int {
	roll := rng.next(10)
	extra := rng.next(3)
	streakStart := time.Date(2025, time.June, 1, 0, 0, 0, 0, time.Local)
	streakEnd := time.Date(2025, time.June, 23, 0, 0, 0, 0, time.Local)
	switch {
	case day.Equal(streakStart.AddDate(0, 0, -1)) || day.Equal(streakEnd.AddDate(0, 0, 1)):
		return 0
	case !day.Before(streakStart) && !day.After(streakEnd):
		return 1 + extra
	case day.Year() == 2024:
		if roll < 3 {
			return 1 + extra%2
		}
		return 0
	case roll < 6:
		if day.Weekday() == time.Sunday {
			return 3 + extra
		}
		return 1 + extra
	}
	return 0
}

// recapFixtureDay lays n plays out on day, back to back from a drawn hour.
func recapFixtureDay(day time.Time, n int, catalog []recapFixtureTrack, rng *recapFixtureRNG) []insights.PlayRow {
	if n == 0 {
		return nil
	}
	at := day.Add(time.Duration(recapFixtureHours[rng.next(len(recapFixtureHours))])*time.Hour + time.Duration(rng.next(40))*time.Minute)
	out := make([]insights.PlayRow, 0, n)
	for range n {
		tr := recapFixturePick(day, catalog, rng)
		secs := int(tr.Duration)
		// Some plays stop early but still count: they add what was heard.
		if rng.next(7) == 0 {
			secs = secs * 6 / 10
		}
		out = append(out, recapFixturePlay(tr, at, secs))
		at = at.Add(time.Duration(secs+30) * time.Second)
	}
	return out
}

func recapFixturePlay(t library.Track, at time.Time, secs int) insights.PlayRow {
	return insights.PlayRow{
		Hash: t.Hash, StartAt: at.Unix(), ListenedSecs: secs, CompletionRate: float64(secs) / t.Duration,
		SessionID: at.Format("rc-20060102"), Artist: t.Artist, Album: t.Album, Title: t.Title,
	}
}

// recapFixturePick draws a track available on day, weighted.
func recapFixturePick(day time.Time, catalog []recapFixtureTrack, rng *recapFixtureRNG) library.Track {
	date := day.Format(time.DateOnly)
	total := 0
	for _, c := range catalog {
		if recapFixtureAvailable(c, date) {
			total += c.weight
		}
	}
	pick := rng.next(total)
	for _, c := range catalog {
		if !recapFixtureAvailable(c, date) {
			continue
		}
		if pick < c.weight {
			return c.track
		}
		pick -= c.weight
	}
	return catalog[0].track
}

func recapFixtureAvailable(c recapFixtureTrack, date string) bool {
	return (c.from == "" || date >= c.from) && (c.to == "" || date <= c.to)
}

// recapFixtureLibrary is the catalog with play counts and last-played times
// that agree with the generated history.
func recapFixtureLibrary(catalog []recapFixtureTrack, plays []insights.PlayRow) []library.Track {
	added := time.Date(2024, time.January, 15, 12, 0, 0, 0, time.Local).Unix()
	tracks := make([]library.Track, len(catalog))
	index := map[string]int{}
	for i, c := range catalog {
		tracks[i] = c.track
		tracks[i].CreatedAt = added
		index[c.track.Hash] = i
	}
	for _, p := range plays {
		t := &tracks[index[p.Hash]]
		t.PlayCount++
		t.LastPlayedAt = max(t.LastPlayedAt, p.StartAt+int64(p.ListenedSecs))
	}
	return tracks
}

// newRecapFixture builds a Model whose listening history fills every page of
// the recap. Saving the recap as a playlist goes to an in-memory store.
func newRecapFixture() Model {
	catalog := recapFixtureCatalog()
	plays := recapFixtureHistory(catalog)
	tracks := recapFixtureLibrary(catalog, plays)
	store := &uitestPlaylistStore{}
	opts := append(baseFixtureOptions(),
		WithLibraryTracks(tracks),
		WithInsightsReader(&uitestInsights{tracks: tracks, history: plays}),
		WithPlaylistWriter(store),
		WithPlaylistAdder(store),
		WithRecapClock(recapFixtureNow),
	)
	return New(context.Background(), opts...)
}

// uitestPlaylistStore is an in-memory playlist writer: it remembers the
// playlists created during a run and nothing else.
type uitestPlaylistStore struct {
	rows []db.PlaylistRow
}

func (s *uitestPlaylistStore) CreatePlaylist(name string) (int64, error) {
	id := int64(len(s.rows) + 1)
	s.rows = append(s.rows, db.PlaylistRow{ID: id, Name: name})
	return id, nil
}

func (s *uitestPlaylistStore) ReadPlaylists() ([]db.PlaylistRow, error) {
	return append([]db.PlaylistRow(nil), s.rows...), nil
}

func (s *uitestPlaylistStore) AddTracksToPlaylist(id int64, hashes []string) error {
	for i := range s.rows {
		if s.rows[i].ID == id {
			s.rows[i].TrackCount += len(hashes)
		}
	}
	return nil
}
