package db

import (
	"fmt"
	"time"
)

// ListeningStats holds aggregated listening data for a given time window.
type ListeningStats struct {
	TotalListenedSecs int
	TotalPlays        int
	TopArtists        []StatEntry // top 5 by play count
	TopAlbums         []StatEntry
	TopGenres         []StatEntry
	TopTracks         []StatEntry
	CurrentStreak     int // consecutive days with ≥1 play
}

// StatEntry holds an aggregated name/play-count/listening-time tuple.
type StatEntry struct {
	Name         string
	PlayCount    int
	ListenedSecs int
}

// ReadListeningStats returns aggregated listening statistics for plays recorded
// at or after sinceUnix. Pass sinceUnix = 0 for all-time stats.
func (d *DB) ReadListeningStats(sinceUnix int64) (ListeningStats, error) {
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

	return stats, nil
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
