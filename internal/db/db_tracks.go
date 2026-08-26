package db

import (
	"database/sql"
	"fmt"

	"github.com/sglavoie/muse/internal/library"
)

// TrackRow holds a single row from the tracks table. Used by ReadTracks to
// return all cached tracks for pre-populating the library on startup.
type TrackRow struct {
	Hash         string
	Path         string
	Title        string
	Artist       string
	Album        string
	Genre        string
	DurationMs   int64
	CreatedAt    int64
	SizeBytes    int64
	ModTime      int64
	PlayCount    int
	LastPlayedAt int64
	TrackNumber  int
	DiscNumber   int
}

// UserEditedRow holds the user-edited metadata for a single track.
type UserEditedRow struct {
	Hash        string
	Title       string
	Artist      string
	Album       string
	Genre       string
	TrackNumber int
	DiscNumber  int
}

// UnverifiedTrack holds the fields needed by the duration audit for a single
// track whose duration has not yet been confirmed by ffprobe.
type UnverifiedTrack struct {
	Hash       string
	Path       string
	DurationMs int64
}

// ReadTracks returns all non-missing tracks from the tracks table. Returns an
// empty slice (not an error) when the table is empty.
func (d *DB) ReadTracks() ([]TrackRow, error) {
	rows, err := d.Query(`
		SELECT hash, path, title, artist, album, genre, duration_ms, created_at, size_bytes, mod_time, play_count, last_played_at, track_number, disc_number
		FROM tracks WHERE missing = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read tracks: %w", err)
	}
	defer rows.Close()

	var result []TrackRow
	for rows.Next() {
		var row TrackRow
		if err := rows.Scan(&row.Hash, &row.Path, &row.Title, &row.Artist, &row.Album, &row.Genre, &row.DurationMs, &row.CreatedAt, &row.SizeBytes, &row.ModTime, &row.PlayCount, &row.LastPlayedAt, &row.TrackNumber, &row.DiscNumber); err != nil {
			return nil, fmt.Errorf("db: read track row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ReadTrackByPath returns the track row stored for the given filesystem path.
// The second return value reports whether a row was found. `missing` is
// deliberately not filtered on: a stale missing marker should not hide the
// metadata for a file that is demonstrably being played. LIMIT 1 because path
// is indexed but not unique (hash is the primary key).
func (d *DB) ReadTrackByPath(path string) (TrackRow, bool, error) {
	var row TrackRow
	err := d.QueryRow(`
		SELECT hash, path, title, artist, album, genre, duration_ms, created_at, size_bytes, mod_time, play_count, last_played_at, track_number, disc_number
		FROM tracks WHERE path = ? LIMIT 1
	`, path).Scan(&row.Hash, &row.Path, &row.Title, &row.Artist, &row.Album, &row.Genre, &row.DurationMs, &row.CreatedAt, &row.SizeBytes, &row.ModTime, &row.PlayCount, &row.LastPlayedAt, &row.TrackNumber, &row.DiscNumber)
	if err == sql.ErrNoRows {
		return TrackRow{}, false, nil
	}
	if err != nil {
		return TrackRow{}, false, fmt.Errorf("db: read track by path: %w", err)
	}
	return row, true, nil
}

// ReadUserEditedTracks returns metadata for all non-missing tracks whose
// metadata was manually edited by the user (user_edited = 1).
func (d *DB) ReadUserEditedTracks() ([]UserEditedRow, error) {
	rows, err := d.Query(`
		SELECT hash, title, artist, album, genre, track_number, disc_number
		FROM tracks WHERE user_edited = 1 AND missing = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read user-edited tracks: %w", err)
	}
	defer rows.Close()

	var result []UserEditedRow
	for rows.Next() {
		var row UserEditedRow
		if err := rows.Scan(&row.Hash, &row.Title, &row.Artist, &row.Album, &row.Genre, &row.TrackNumber, &row.DiscNumber); err != nil {
			return nil, fmt.Errorf("db: read user-edited row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ReadUnverifiedDurations returns all non-missing tracks whose duration_verified
// flag is 0. Returns an empty slice (not an error) when none exist.
func (d *DB) ReadUnverifiedDurations() ([]UnverifiedTrack, error) {
	rows, err := d.Query(`
		SELECT hash, path, duration_ms
		FROM tracks WHERE duration_verified = 0 AND missing = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read unverified durations: %w", err)
	}
	defer rows.Close()

	var result []UnverifiedTrack
	for rows.Next() {
		var row UnverifiedTrack
		if err := rows.Scan(&row.Hash, &row.Path, &row.DurationMs); err != nil {
			return nil, fmt.Errorf("db: read unverified track row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ReadPlayHistory returns the most recent play events joined with track
// metadata, ordered by played_at descending. Each row represents one play
// event; the same track may appear multiple times. LastPlayedAt is set to the
// per-event played_at timestamp. Pass limit ≤ 0 to use the default of 500.
func (d *DB) ReadPlayHistory(limit int) ([]library.Track, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := d.Query(`
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, ph.played_at, t.track_number, t.disc_number
		FROM play_history ph
		JOIN tracks t ON ph.track_hash = t.hash
		ORDER BY ph.played_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read play history: %w", err)
	}
	defer rows.Close()
	return scanTrackRows(rows)
}
