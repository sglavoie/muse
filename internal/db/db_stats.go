package db

import (
	"fmt"
	"time"
)

// MixCountConfig holds the thresholds used by ReadMixCounts.
// Populate from config.MixesConfig at the call site.
type MixCountConfig struct {
	RediscoverMinPlays    int
	RediscoverAgeDays     int
	HiddenGemsMaxPlays    int
	HiddenGemsTopArtists  int
	HiddenGemsTopGenres   int
	OnRepeatMinPlays      int
	OnRepeatWindowDays    int
	DeepCutsMinPlays      int
	DeepCutsMaxPlays      int
	DeepCutsMinCompletion float64
}

// DefaultMixCountConfig returns thresholds matching the default config values.
func DefaultMixCountConfig() MixCountConfig {
	return MixCountConfig{
		RediscoverMinPlays:    10,
		RediscoverAgeDays:     90,
		HiddenGemsMaxPlays:    2,
		HiddenGemsTopArtists:  20,
		HiddenGemsTopGenres:   10,
		OnRepeatMinPlays:      3,
		OnRepeatWindowDays:    7,
		DeepCutsMinPlays:      1,
		DeepCutsMaxPlays:      5,
		DeepCutsMinCompletion: 0.8,
	}
}

// MixStatEntry records a mix name and how many tracks currently qualify.
type MixStatEntry struct {
	Name        string
	TrackCount  int
	Description string
}

// ListeningStats holds aggregated listening data for a given time window.
type ListeningStats struct {
	TotalListenedSecs int
	TotalPlays        int
	TopArtists        []StatEntry // top 5 by play count
	TopAlbums         []StatEntry
	TopGenres         []StatEntry
	TopTracks         []StatEntry
	CurrentStreak     int            // consecutive days with ≥1 play
	MixStats          []MixStatEntry // always all-time, independent of period
}

// StatEntry holds an aggregated name/play-count/listening-time tuple.
type StatEntry struct {
	Name         string
	PlayCount    int
	ListenedSecs int
}

// ReadListeningStats returns aggregated listening statistics for plays recorded
// at or after sinceUnix. Pass sinceUnix = 0 for all-time stats. Mix counts are
// always all-time and use thresholds from mixCfg.
func (d *DB) ReadListeningStats(sinceUnix int64, mixCfg MixCountConfig) (ListeningStats, error) {
	var stats ListeningStats

	if err := d.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(listened_secs), 0) FROM play_history WHERE played_at >= ?`,
		sinceUnix,
	).Scan(&stats.TotalPlays, &stats.TotalListenedSecs); err != nil {
		return stats, fmt.Errorf("db: read listening stats totals: %w", err)
	}

	var err error
	stats.TopArtists, err = readTopEntries(d,
		`SELECT t.artist, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY t.artist ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top artists: %w", err)
	}

	stats.TopAlbums, err = readTopEntries(d,
		`SELECT t.album, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY t.album ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top albums: %w", err)
	}

	stats.TopGenres, err = readTopEntries(d,
		`SELECT t.genre, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY t.genre ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top genres: %w", err)
	}

	stats.TopTracks, err = readTopEntries(d,
		`SELECT t.artist || ' — ' || t.title, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY ph.track_hash ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top tracks: %w", err)
	}

	stats.CurrentStreak, err = readCurrentStreak(d)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats streak: %w", err)
	}

	stats.MixStats, err = d.ReadMixCounts(mixCfg)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats mix counts: %w", err)
	}

	return stats, nil
}

// ReadMixCounts returns a count of qualifying tracks for each recommendation
// mix, using the thresholds in cfg. Mix counts are always all-time.
func (d *DB) ReadMixCounts(cfg MixCountConfig) ([]MixStatEntry, error) {
	entries := []MixStatEntry{
		{Name: "Rediscover", Description: "Tracks you loved but haven't played in 90+ days"},
		{Name: "Hidden Gems", Description: "Unplayed tracks by your favorite artists"},
		{Name: "On Repeat", Description: "Your most played tracks this week"},
		{Name: "Deep Cuts", Description: "Tracks you always listen to completion"},
		{Name: "Never Played", Description: "Tracks you might like but haven't tried"},
	}

	cutoff := time.Now().Add(-time.Duration(cfg.RediscoverAgeDays) * 24 * time.Hour).Unix()
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM tracks WHERE play_count >= ? AND last_played_at < ? AND last_played_at > 0`,
		cfg.RediscoverMinPlays, cutoff,
	).Scan(&entries[0].TrackCount); err != nil {
		return nil, fmt.Errorf("db: read mix counts rediscover: %w", err)
	}

	if err := d.QueryRow(`
		SELECT COUNT(*) FROM tracks t
		WHERE t.play_count <= ?
		AND (
		    t.artist IN (
		        SELECT t2.artist FROM play_history ph JOIN tracks t2 ON ph.track_hash = t2.hash
		        GROUP BY t2.artist ORDER BY COUNT(*) DESC LIMIT ?
		    )
		    OR EXISTS (
		        SELECT 1 FROM track_genres tg WHERE tg.track_hash = t.hash
		        AND tg.genre IN (
		            SELECT tg2.genre FROM play_history ph
		            JOIN track_genres tg2 ON ph.track_hash = tg2.track_hash
		            GROUP BY tg2.genre ORDER BY COUNT(*) DESC LIMIT ?
		        )
		    )
		)`,
		cfg.HiddenGemsMaxPlays, cfg.HiddenGemsTopArtists, cfg.HiddenGemsTopGenres,
	).Scan(&entries[1].TrackCount); err != nil {
		return nil, fmt.Errorf("db: read mix counts hidden gems: %w", err)
	}

	windowStart := time.Now().Add(-time.Duration(cfg.OnRepeatWindowDays) * 24 * time.Hour).Unix()
	if err := d.QueryRow(`
		SELECT COUNT(*) FROM (
		    SELECT t.hash FROM tracks t
		    JOIN play_history ph ON ph.track_hash = t.hash
		    WHERE ph.played_at >= ?
		    GROUP BY t.hash HAVING COUNT(*) >= ?
		) sub`,
		windowStart, cfg.OnRepeatMinPlays,
	).Scan(&entries[2].TrackCount); err != nil {
		return nil, fmt.Errorf("db: read mix counts on repeat: %w", err)
	}

	if err := d.QueryRow(`
		SELECT COUNT(*) FROM (
		    SELECT t.hash FROM tracks t
		    JOIN play_history ph ON ph.track_hash = t.hash
		    WHERE t.play_count BETWEEN ? AND ?
		    GROUP BY t.hash HAVING AVG(ph.completion_rate) >= ?
		) sub`,
		cfg.DeepCutsMinPlays, cfg.DeepCutsMaxPlays, cfg.DeepCutsMinCompletion,
	).Scan(&entries[3].TrackCount); err != nil {
		return nil, fmt.Errorf("db: read mix counts deep cuts: %w", err)
	}

	if err := d.QueryRow(
		`SELECT COUNT(*) FROM tracks WHERE play_count = 0`,
	).Scan(&entries[4].TrackCount); err != nil {
		return nil, fmt.Errorf("db: read mix counts never played: %w", err)
	}

	return entries, nil
}

func readTopEntries(d *DB, query string, arg int64) ([]StatEntry, error) {
	rows, err := d.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []StatEntry
	for rows.Next() {
		var entry StatEntry
		if err := rows.Scan(&entry.Name, &entry.PlayCount, &entry.ListenedSecs); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func readCurrentStreak(d *DB) (int, error) {
	// LIMIT 366 bounds the result set: streaks longer than a year are
	// extremely unlikely, and without a limit SQLite must materialise every
	// distinct play day before the Go-side loop can break on the first gap.
	rows, err := d.Query(
		`SELECT DISTINCT date(played_at, 'unixepoch', 'localtime') AS day
		 FROM play_history ORDER BY day DESC LIMIT 366`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	streak := 0
	expected := ""
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return 0, err
		}
		// On the first row, accept either today or yesterday as the
		// streak start. This way a streak is not broken just because
		// the user has not played anything yet today.
		if expected == "" {
			if day == today || day == yesterday {
				expected = day
			} else {
				break
			}
		}
		if day != expected {
			break
		}
		streak++
		parsedDay, err := time.ParseInLocation("2006-01-02", day, time.Local)
		if err != nil {
			break
		}
		expected = parsedDay.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return streak, rows.Err()
}
