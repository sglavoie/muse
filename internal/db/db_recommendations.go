package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
)

// MixID is a type alias for recommendation.MixID, kept for backward
// compatibility so callers using db.MixID continue to compile unchanged.
type MixID = recommendation.MixID

// Re-export recommendation mix constants so callers using db.MixX still compile.
const (
	MixRediscover  = recommendation.MixRediscover
	MixHiddenGems  = recommendation.MixHiddenGems
	MixOnRepeat    = recommendation.MixOnRepeat
	MixDeepCuts    = recommendation.MixDeepCuts
	MixNeverPlayed = recommendation.MixNeverPlayed
)

// Compile-time check: *DB satisfies recommendation.RecommendationStore.
var _ recommendation.RecommendationStore = (*DB)(nil)

// SessionCoOccurrence records how many times a track co-occurred with a seed
// track within a time window in play history.
type SessionCoOccurrence struct {
	Hash  string
	Count int
}

// ReadRediscoverMix returns tracks played at least minPlays times but not heard
// in the last ageDays days, ordered by play count descending.
func (d *DB) ReadRediscoverMix(minPlays, ageDays, limit int) ([]library.Track, error) {
	cutoff := time.Now().Add(-time.Duration(ageDays) * 24 * time.Hour).Unix()
	rows, err := d.Query(`
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, t.last_played_at
		FROM tracks t
		WHERE t.play_count >= ? AND t.last_played_at < ? AND t.last_played_at > 0
		ORDER BY t.play_count DESC
		LIMIT ?`, minPlays, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read rediscover mix: %w", err)
	}
	defer rows.Close()
	return scanTrackRows(rows)
}

// ReadHiddenGemsMix returns low-played tracks (play_count <= maxPlays) whose
// artist or genre matches the user's top taste profile. topArtists and
// topGenres control how many top artists/genres are considered. Uses
// track_genres for exact genre matching. Returns results in random order.
func (d *DB) ReadHiddenGemsMix(maxPlays, topArtists, topGenres, limit int) ([]library.Track, error) {
	artists, err := d.ReadTopArtists(topArtists)
	if err != nil {
		return nil, fmt.Errorf("db: read hidden gems mix (top artists): %w", err)
	}
	genres, err := d.ReadTopGenres(topGenres)
	if err != nil {
		return nil, fmt.Errorf("db: read hidden gems mix (top genres): %w", err)
	}
	if len(artists) == 0 && len(genres) == 0 {
		return nil, nil
	}

	var conditions []string
	var args []any

	if len(artists) > 0 {
		conditions = append(conditions, "t.artist IN ("+inPlaceholders(len(artists))+")")
		for _, a := range artists {
			args = append(args, a)
		}
	}
	if len(genres) > 0 {
		conditions = append(conditions,
			"EXISTS (SELECT 1 FROM track_genres tg WHERE tg.track_hash = t.hash AND tg.genre IN ("+inPlaceholders(len(genres))+"))")
		for _, g := range genres {
			args = append(args, g)
		}
	}
	args = append(args, limit)

	args = append([]any{maxPlays}, args...)
	query := `
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, t.last_played_at
		FROM tracks t
		WHERE t.play_count <= ? AND (` + strings.Join(conditions, " OR ") + `)
		ORDER BY RANDOM()
		LIMIT ?`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: read hidden gems mix: %w", err)
	}
	defer rows.Close()
	return scanTrackRows(rows)
}

// ReadOnRepeatMix returns tracks with minPlays or more plays in the last
// windowDays days, ordered by recent play count descending.
func (d *DB) ReadOnRepeatMix(minPlays, windowDays, limit int) ([]library.Track, error) {
	windowStart := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).Unix()
	rows, err := d.Query(`
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, t.last_played_at,
		       COUNT(*) AS recent_count
		FROM tracks t
		JOIN play_history ph ON ph.track_hash = t.hash
		WHERE ph.played_at >= ?
		GROUP BY t.hash
		HAVING COUNT(*) >= ?
		ORDER BY recent_count DESC
		LIMIT ?`, windowStart, minPlays, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read on repeat mix: %w", err)
	}
	defer rows.Close()
	return scanTrackRowsWithExtra(rows, 1)
}

// ReadDeepCutsMix returns tracks with minPlays–maxPlays plays and an average
// completion rate of at least minCompletion, ordered by play count ascending.
func (d *DB) ReadDeepCutsMix(minPlays, maxPlays int, minCompletion float64, limit int) ([]library.Track, error) {
	rows, err := d.Query(`
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, t.last_played_at
		FROM tracks t
		JOIN play_history ph ON ph.track_hash = t.hash
		WHERE t.play_count BETWEEN ? AND ?
		GROUP BY t.hash
		HAVING AVG(ph.completion_rate) >= ?
		ORDER BY t.play_count ASC
		LIMIT ?`, minPlays, maxPlays, minCompletion, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read deep cuts mix: %w", err)
	}
	defer rows.Close()
	return scanTrackRows(rows)
}

// ReadNeverPlayedMix returns unplayed tracks ranked by artist and genre
// affinity (how often the user has listened to the same artist or genre).
// Genre affinity uses the track_genres junction table for exact matching.
func (d *DB) ReadNeverPlayedMix(limit int) ([]library.Track, error) {
	rows, err := d.Query(`
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, t.last_played_at,
		       (
		           SELECT COUNT(*)
		           FROM play_history ph2
		           JOIN tracks t2 ON ph2.track_hash = t2.hash
		           WHERE t2.artist = t.artist
		       ) +
		       (
		           SELECT COUNT(*)
		           FROM play_history ph3
		           JOIN track_genres tg2 ON ph3.track_hash = tg2.track_hash
		           JOIN track_genres tg ON tg.genre = tg2.genre
		           WHERE tg.track_hash = t.hash
		       ) AS affinity
		FROM tracks t
		WHERE t.play_count = 0
		ORDER BY affinity DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read never played mix: %w", err)
	}
	defer rows.Close()
	return scanTrackRowsWithExtra(rows, 1)
}

// ReadCompletionRates returns the average stored completion_rate per track hash
// for the given hashes. Hashes with no play history are absent from the result.
func (d *DB) ReadCompletionRates(hashes []string) (map[string]float64, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	args := make([]any, len(hashes))
	for i, h := range hashes {
		args[i] = h
	}
	rows, err := d.Query(`
		SELECT track_hash, AVG(completion_rate)
		FROM play_history
		WHERE track_hash IN (`+inPlaceholders(len(hashes))+`)
		GROUP BY track_hash`, args...)
	if err != nil {
		return nil, fmt.Errorf("db: read completion rates: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64, len(hashes))
	for rows.Next() {
		var hash string
		var rate float64
		if err := rows.Scan(&hash, &rate); err != nil {
			return nil, fmt.Errorf("db: read completion rates row: %w", err)
		}
		result[hash] = rate
	}
	return result, rows.Err()
}

// ReadSessionCoOccurrences returns tracks that shared a listening session with
// any play of seedHash, ordered by co-occurrence count descending. The seed
// track itself is excluded from results. Tracks with no session_id (pre-018
// rows without a backfill) are ignored.
func (d *DB) ReadSessionCoOccurrences(seedHash string, limit int) ([]SessionCoOccurrence, error) {
	rows, err := d.Query(`
		SELECT ph2.track_hash, COUNT(*) AS co_count
		FROM play_history ph1
		JOIN play_history ph2
		  ON ph2.session_id = ph1.session_id
		 AND ph2.track_hash != ?
		WHERE ph1.track_hash = ?
		  AND ph1.session_id IS NOT NULL
		GROUP BY ph2.track_hash
		ORDER BY co_count DESC
		LIMIT ?`, seedHash, seedHash, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read session co-occurrences: %w", err)
	}
	defer rows.Close()

	var result []SessionCoOccurrence
	for rows.Next() {
		var c SessionCoOccurrence
		if err := rows.Scan(&c.Hash, &c.Count); err != nil {
			return nil, fmt.Errorf("db: read session co-occurrences row: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// ReadTopArtists returns the top artists by play count across all play history.
func (d *DB) ReadTopArtists(limit int) ([]string, error) {
	rows, err := d.Query(`
		SELECT t.artist, COUNT(*) AS cnt
		FROM play_history ph
		JOIN tracks t ON ph.track_hash = t.hash
		GROUP BY t.artist
		ORDER BY cnt DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read top artists: %w", err)
	}
	defer rows.Close()
	return scanStringWithCount(rows, "db: read top artists row")
}

// ReadTopGenres returns the top genres by play count, using the track_genres
// junction table for accurate per-genre counts.
func (d *DB) ReadTopGenres(limit int) ([]string, error) {
	rows, err := d.Query(`
		SELECT tg.genre, COUNT(*) AS cnt
		FROM play_history ph
		JOIN track_genres tg ON ph.track_hash = tg.track_hash
		GROUP BY tg.genre
		ORDER BY cnt DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read top genres: %w", err)
	}
	defer rows.Close()
	return scanStringWithCount(rows, "db: read top genres row")
}

// HasSufficientHistory returns true when play_history contains at least
// minCount rows.
func (d *DB) HasSufficientHistory(minCount int) (bool, error) {
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM play_history`).Scan(&count); err != nil {
		return false, fmt.Errorf("db: has sufficient history: %w", err)
	}
	return count >= minCount, nil
}

// HourGenreAffinity records how many times a genre was played in a given hour.
type HourGenreAffinity struct {
	Hour  int // 0-23 local time
	Genre string
	Count int
}

// ReadHourGenreAffinities returns play counts grouped by (hour, genre) for
// hours where a genre was played at least 3 times. Uses the track_genres
// junction table for exact per-genre matching. Results are ordered by hour
// then count descending.
func (d *DB) ReadHourGenreAffinities() ([]HourGenreAffinity, error) {
	rows, err := d.Query(`
		SELECT CAST(strftime('%H', ph.played_at, 'unixepoch', 'localtime') AS INTEGER),
		       tg.genre, COUNT(*)
		FROM play_history ph
		JOIN track_genres tg ON tg.track_hash = ph.track_hash
		GROUP BY 1, 2
		HAVING COUNT(*) >= 3
		ORDER BY 1, 3 DESC`)
	if err != nil {
		return nil, fmt.Errorf("db: read hour genre affinities: %w", err)
	}
	defer rows.Close()

	var result []HourGenreAffinity
	for rows.Next() {
		var a HourGenreAffinity
		if err := rows.Scan(&a.Hour, &a.Genre, &a.Count); err != nil {
			return nil, fmt.Errorf("db: read hour genre affinities row: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// inPlaceholders returns a comma-separated string of n SQL placeholders: "?,?,?".
func inPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// scanTrackRows scans the standard 12-column track SELECT result set.
func scanTrackRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]library.Track, error) {
	var result []library.Track
	for rows.Next() {
		var track library.Track
		var durationMs int64
		if err := rows.Scan(
			&track.Path, &track.Hash, &track.Artist, &track.Title, &track.Album, &track.Genre,
			&durationMs, &track.CreatedAt, &track.ModTime, &track.SizeBytes,
			&track.PlayCount, &track.LastPlayedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan track row: %w", err)
		}
		track.Duration = float64(durationMs) / 1000
		result = append(result, track)
	}
	return result, rows.Err()
}

// scanTrackRowsWithExtra scans the standard 12-column track result set followed
// by extraCols additional columns that are read and discarded.
func scanTrackRowsWithExtra(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}, extraCols int) ([]library.Track, error) {
	extra := make([]any, extraCols)
	for i := range extra {
		var v any
		extra[i] = &v
	}
	var result []library.Track
	for rows.Next() {
		var track library.Track
		var durationMs int64
		dest := []any{
			&track.Path, &track.Hash, &track.Artist, &track.Title, &track.Album, &track.Genre,
			&durationMs, &track.CreatedAt, &track.ModTime, &track.SizeBytes,
			&track.PlayCount, &track.LastPlayedAt,
		}
		dest = append(dest, extra...)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("db: scan track row (extra): %w", err)
		}
		track.Duration = float64(durationMs) / 1000
		result = append(result, track)
	}
	return result, rows.Err()
}

// scanStringWithCount scans the first column of each row as a string,
// discarding the second integer column (typically a COUNT).
func scanStringWithCount(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}, errPrefix string) ([]string, error) {
	var result []string
	for rows.Next() {
		var s string
		var cnt int
		if err := rows.Scan(&s, &cnt); err != nil {
			return nil, fmt.Errorf("%s: %w", errPrefix, err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
