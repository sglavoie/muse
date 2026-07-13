package db

import (
	"database/sql"
	"fmt"
)

// QueueRow holds a single queue entry joined with its track metadata.
// Used by ReadQueue to return typed results that callers convert into
// player.QueueTrack and tui.QueueEntry slices.
type QueueRow struct {
	ID             int64
	Hash           string
	Path           string
	Artist         string
	Title          string
	Album          string
	Missing        bool
	DurationMs     int64
	Position       int64
	SourcePlaylist string
	TrackNumber    int
	DiscNumber     int
}

// SessionRow holds the persisted session state from the single session row.
// CurrentHash is empty when no track was current at the time of the last save.
type SessionRow struct {
	RepeatMode  string
	Shuffle     bool
	RadioMode   bool
	CurrentHash string
	PositionMs  int64
	Volume      float64
}

// ReadQueue returns all active (non-deleted) queue entries ordered by
// position, joined with track metadata. Returns an empty slice (not an error)
// when the queue is empty.
//
// Ordering is always by position (original insertion order). The legacy
// shuffle_position column is retained for schema compatibility but is no longer
// read or regenerated; shuffle playback order is Go-owned by the player.
func (d *DB) ReadQueue() ([]QueueRow, error) {
	rows, err := d.Query(`
		SELECT q.id, t.hash, t.path, t.artist, t.title, t.album,
		       CASE WHEN q.missing != 0 OR t.missing != 0 THEN 1 ELSE 0 END,
		       t.duration_ms, q.position, q.source_playlist, t.track_number, t.disc_number
		FROM queue q
		JOIN tracks t ON q.track_hash = t.hash
		WHERE q.deleted_at IS NULL
		ORDER BY q.position
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read queue: %w", err)
	}
	defer rows.Close()

	result := make([]QueueRow, 0)
	for rows.Next() {
		var row QueueRow
		var missing int
		if err := rows.Scan(&row.ID, &row.Hash, &row.Path, &row.Artist, &row.Title, &row.Album, &missing, &row.DurationMs, &row.Position, &row.SourcePlaylist, &row.TrackNumber, &row.DiscNumber); err != nil {
			return nil, fmt.Errorf("db: read queue row: %w", err)
		}
		row.Missing = missing != 0
		result = append(result, row)
	}
	return result, rows.Err()
}

// ReadSession reads the single session row and returns the persisted state.
// Returns nil (no error) when no session row exists.
func (d *DB) ReadSession() (*SessionRow, error) {
	var row SessionRow
	var currentHash sql.NullString
	var shuffle int
	var radioMode int
	err := d.QueryRow(`
		SELECT repeat_mode, shuffle, radio_mode, current_hash, position_ms, volume
		FROM session WHERE id = 1
	`).Scan(&row.RepeatMode, &shuffle, &radioMode, &currentHash, &row.PositionMs, &row.Volume)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: read session: %w", err)
	}
	row.Shuffle = shuffle != 0
	row.RadioMode = radioMode != 0
	if currentHash.Valid {
		row.CurrentHash = currentHash.String
	}
	return &row, nil
}
