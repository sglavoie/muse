package db

import (
	"fmt"

	"github.com/sglavoie/muse/internal/insights"
)

// The Insights readers are deliberately wide and dumb: they hand whole result
// sets to internal/insights, which does every aggregation in Go. At this
// library's size the full history is a few hundred kilobytes, and computing in
// Go buys testability and timezone correctness that SQL date functions —
// which would silently use the host's local zone — cannot give.
//
// Every play timestamp these readers return is start-corrected:
// play_history.played_at is written when playback *ends*, so the play started
// at played_at - listened_secs. The Since filter is applied to the corrected
// start too, so a play is in the window when it *began* in the window.

// ReadInsightPlays returns every play that started at or after sinceUnix,
// oldest first. Pass 0 for all time.
func (d *DB) ReadInsightPlays(sinceUnix int64) ([]insights.PlayRow, error) {
	rows, err := d.Query(`
		SELECT ph.track_hash,
		       ph.played_at - ph.listened_secs AS start_at,
		       ph.listened_secs,
		       ph.completion_rate,
		       COALESCE(ph.session_id, ''),
		       t.artist, t.album, t.title
		FROM play_history ph
		JOIN tracks t ON t.hash = ph.track_hash
		WHERE ph.played_at - ph.listened_secs >= ?
		ORDER BY start_at ASC, ph.id ASC`, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("db: read insight plays: %w", err)
	}
	defer rows.Close()

	var out []insights.PlayRow
	for rows.Next() {
		var p insights.PlayRow
		if err := rows.Scan(&p.Hash, &p.StartAt, &p.ListenedSecs, &p.CompletionRate,
			&p.SessionID, &p.Artist, &p.Album, &p.Title); err != nil {
			return nil, fmt.Errorf("db: scan insight play: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReadInsightGenres returns per-genre play aggregates for the window. Genres
// come from the track_genres junction, never from the comma-separated
// tracks.genre column: a track tagged "rock,pop" must count toward both rock
// and pop instead of forming a composite bucket that hides either.
func (d *DB) ReadInsightGenres(sinceUnix int64) ([]insights.GenreRow, error) {
	rows, err := d.Query(`
		SELECT tg.genre,
		       COUNT(*),
		       COALESCE(SUM(ph.listened_secs), 0),
		       COUNT(DISTINCT ph.track_hash)
		FROM play_history ph
		JOIN track_genres tg ON tg.track_hash = ph.track_hash
		WHERE ph.played_at - ph.listened_secs >= ?
		GROUP BY tg.genre
		ORDER BY COUNT(*) DESC, tg.genre ASC`, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("db: read insight genres: %w", err)
	}
	defer rows.Close()

	var out []insights.GenreRow
	for rows.Next() {
		var g insights.GenreRow
		if err := rows.Scan(&g.Genre, &g.Plays, &g.ListenedSecs, &g.Tracks); err != nil {
			return nil, fmt.Errorf("db: scan insight genre: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ReadInsightSessions returns one row per listening session that started at or
// after sinceUnix, oldest first, with each session's opening and closing
// track.
//
// Sessions are rebuilt from play_history.session_id rather than read from the
// listening_sessions table: that table's started_at is written from the same
// end-of-play timestamp, so it is late by the length of the first track.
func (d *DB) ReadInsightSessions(sinceUnix int64) ([]insights.SessionRow, error) {
	rows, err := d.Query(`
		WITH ordered AS (
		    SELECT ph.session_id AS sid,
		           ph.played_at - ph.listened_secs AS start_at,
		           ph.played_at AS end_at,
		           ph.listened_secs AS secs,
		           t.artist AS artist,
		           t.title AS title,
		           ROW_NUMBER() OVER (PARTITION BY ph.session_id ORDER BY ph.played_at ASC, ph.id ASC) AS rn_open,
		           ROW_NUMBER() OVER (PARTITION BY ph.session_id ORDER BY ph.played_at DESC, ph.id DESC) AS rn_close
		    FROM play_history ph
		    JOIN tracks t ON t.hash = ph.track_hash
		    WHERE ph.session_id IS NOT NULL AND ph.session_id != ''
		)
		SELECT sid,
		       MIN(start_at),
		       MAX(end_at),
		       COUNT(*),
		       COALESCE(SUM(secs), 0),
		       COUNT(DISTINCT artist),
		       COALESCE(MAX(CASE WHEN rn_open = 1 THEN artist END), ''),
		       COALESCE(MAX(CASE WHEN rn_open = 1 THEN title END), ''),
		       COALESCE(MAX(CASE WHEN rn_close = 1 THEN artist END), ''),
		       COALESCE(MAX(CASE WHEN rn_close = 1 THEN title END), '')
		FROM ordered
		GROUP BY sid
		HAVING MIN(start_at) >= ?
		ORDER BY MIN(start_at) ASC`, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("db: read insight sessions: %w", err)
	}
	defer rows.Close()

	var out []insights.SessionRow
	for rows.Next() {
		var s insights.SessionRow
		if err := rows.Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.Plays, &s.ListenedSecs,
			&s.DistinctArtists, &s.OpenerArtist, &s.OpenerTitle,
			&s.CloserArtist, &s.CloserTitle); err != nil {
			return nil, fmt.Errorf("db: scan insight session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReadInsightCoverage returns every non-missing library track. Coverage is
// always all-time and takes no window: a track being unplayed inside the last
// 7 days says nothing about whether it has ever been heard.
func (d *DB) ReadInsightCoverage() ([]insights.CoverageRow, error) {
	rows, err := d.Query(`
		SELECT hash, artist, album, duration_ms / 1000.0, play_count,
		       created_at, COALESCE(last_played_at, 0)
		FROM tracks
		WHERE missing = 0`)
	if err != nil {
		return nil, fmt.Errorf("db: read insight coverage: %w", err)
	}
	defer rows.Close()

	var out []insights.CoverageRow
	for rows.Next() {
		var c insights.CoverageRow
		if err := rows.Scan(&c.Hash, &c.Artist, &c.Album, &c.DurationSecs,
			&c.PlayCount, &c.AddedAt, &c.LastPlayedAt); err != nil {
			return nil, fmt.Errorf("db: scan insight coverage: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ReadInsightTimeline returns every play ever recorded, start-corrected and
// flagged with whether it was that track's first. The Calendar, Momentum and
// Discovery panels are all-time by design — the active window only sets their
// resolution — so they read this instead of the windowed play rows.
func (d *DB) ReadInsightTimeline() ([]insights.TimelineRow, error) {
	rows, err := d.Query(`
		SELECT track_hash, start_at, listened_secs, first_ever FROM (
		    SELECT ph.track_hash AS track_hash,
		           ph.played_at - ph.listened_secs AS start_at,
		           ph.listened_secs AS listened_secs,
		           CASE WHEN ROW_NUMBER() OVER (
		               PARTITION BY ph.track_hash
		               ORDER BY ph.played_at - ph.listened_secs ASC, ph.id ASC
		           ) = 1 THEN 1 ELSE 0 END AS first_ever
		    FROM play_history ph
		    JOIN tracks t ON t.hash = ph.track_hash
		)
		ORDER BY start_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("db: read insight timeline: %w", err)
	}
	defer rows.Close()

	var out []insights.TimelineRow
	for rows.Next() {
		var t insights.TimelineRow
		if err := rows.Scan(&t.Hash, &t.StartAt, &t.ListenedSecs, &t.FirstEver); err != nil {
			return nil, fmt.Errorf("db: scan insight timeline: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReadInsightQuality returns the all-time library-health counts behind the
// Data Health panel. deepCutsMinPlays, deepCutsMaxPlays and
// deepCutsMinCompletion mirror the Deep Cuts mix thresholds so the panel can
// show how little that mix's completion filter actually excludes.
//
// The two deep-cuts scalars average over play_events for the same reason
// ReadDeepCutsMix does: measured against the counted-only play_history view
// they would describe a population the mix no longer uses, and the share would
// sit near 1.0 forever. The skips-recorded scalar reports whether that share
// is measurable at all yet.
func (d *DB) ReadInsightQuality(deepCutsMinPlays, deepCutsMaxPlays int, deepCutsMinCompletion float64) (insights.QualityCounts, error) {
	var q insights.QualityCounts

	scalars := []struct {
		what  string
		query string
		args  []any
		into  *int
	}{
		{"tracks total", `SELECT COUNT(*) FROM tracks WHERE missing = 0`, nil, &q.TracksTotal},
		{"tracks missing", `SELECT COUNT(*) FROM tracks WHERE missing = 1`, nil, &q.TracksMissing},
		{"tracks without genre", `
			SELECT COUNT(*) FROM tracks t
			WHERE t.missing = 0
			AND NOT EXISTS (SELECT 1 FROM track_genres tg WHERE tg.track_hash = t.hash)`, nil, &q.TracksWithoutGenre},
		{"plays total", `SELECT COUNT(*) FROM play_history`, nil, &q.PlaysTotal},
		{"plays without genre", `
			SELECT COUNT(*) FROM play_history ph
			WHERE NOT EXISTS (SELECT 1 FROM track_genres tg WHERE tg.track_hash = ph.track_hash)`, nil, &q.PlaysWithoutGenre},
		{"skips recorded", `SELECT COUNT(*) FROM play_events WHERE counted = 0`, nil, &q.SkipsRecorded},
		{"deep cuts candidates", `
			SELECT COUNT(*) FROM (
			    SELECT t.hash FROM tracks t
			    JOIN play_events pe ON pe.track_hash = t.hash
			    WHERE t.missing = 0 AND t.play_count BETWEEN ? AND ?
			    GROUP BY t.hash
			) sub`, []any{deepCutsMinPlays, deepCutsMaxPlays}, &q.DeepCutsCandidates},
		{"deep cuts qualifying", `
			SELECT COUNT(*) FROM (
			    SELECT t.hash FROM tracks t
			    JOIN play_events pe ON pe.track_hash = t.hash
			    WHERE t.missing = 0 AND t.play_count BETWEEN ? AND ?
			    GROUP BY t.hash HAVING AVG(pe.completion_rate) >= ?
			) sub`, []any{deepCutsMinPlays, deepCutsMaxPlays, deepCutsMinCompletion}, &q.DeepCutsQualifying},
	}

	for _, s := range scalars {
		if err := d.QueryRow(s.query, s.args...).Scan(s.into); err != nil {
			return q, fmt.Errorf("db: read insight quality %s: %w", s.what, err)
		}
	}

	return q, nil
}
