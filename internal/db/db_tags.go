package db

import "fmt"

// TagRow holds a single row from the tags table.
type TagRow struct {
	ID        int64
	Name      string
	Color     string
	CreatedAt int64
}

// scanTagRows scans the standard 4-column TagRow SELECT result set
// (id, name, color, created_at) into a slice, returning any scan or iteration error.
func scanTagRows(rows rowScanner, errContext string) ([]TagRow, error) {
	result := make([]TagRow, 0)
	for rows.Next() {
		var row TagRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Color, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("%s: %w", errContext, err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ReadTags returns all active (non-deleted) tags ordered by name.
// Returns an empty slice (not an error) when none exist.
func (d *DB) ReadTags() ([]TagRow, error) {
	rows, err := d.Query(`
		SELECT id, name, color, created_at
		FROM tags WHERE deleted_at IS NULL
		ORDER BY name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read tags: %w", err)
	}
	defer rows.Close()

	return scanTagRows(rows, "db: read tag row")
}

// ReadTagsForTrack returns all active tags assigned to the track with the
// given hash, ordered by name. Returns an empty slice when none exist.
func (d *DB) ReadTagsForTrack(hash string) ([]TagRow, error) {
	rows, err := d.Query(`
		SELECT tg.id, tg.name, tg.color, tg.created_at
		FROM track_tags tt
		JOIN tags tg ON tt.tag_id = tg.id
		WHERE tt.track_hash = ? AND tg.deleted_at IS NULL
		ORDER BY tg.name COLLATE NOCASE
	`, hash)
	if err != nil {
		return nil, fmt.Errorf("db: read tags for track: %w", err)
	}
	defer rows.Close()

	return scanTagRows(rows, "db: read tag row for track")
}

// ReadTrackHashesByTag returns all track hashes that carry the given tag ID.
// Returns an empty slice when none exist.
func (d *DB) ReadTrackHashesByTag(tagID int64) ([]string, error) {
	rows, err := d.Query(`
		SELECT tt.track_hash
		FROM track_tags tt
		JOIN tags tg ON tt.tag_id = tg.id
		WHERE tt.tag_id = ? AND tg.deleted_at IS NULL
	`, tagID)
	if err != nil {
		return nil, fmt.Errorf("db: read track hashes by tag: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("db: read track hash row: %w", err)
		}
		result = append(result, hash)
	}
	return result, rows.Err()
}

// ReadTracksByTag returns full track rows for all tracks carrying the given
// tag ID, ordered by artist then title. Returns an empty slice when none exist.
func (d *DB) ReadTracksByTag(tagID int64) ([]PlaylistTrackRow, error) {
	rows, err := d.Query(`
		SELECT 0, 0, t.title, t.artist, t.album, t.duration_ms, t.hash, t.path, t.track_number, t.disc_number
		FROM track_tags tt
		JOIN tracks t ON tt.track_hash = t.hash
		JOIN tags tg ON tt.tag_id = tg.id
		WHERE tt.tag_id = ? AND tg.deleted_at IS NULL AND t.missing = 0
		ORDER BY t.artist COLLATE NOCASE, t.title COLLATE NOCASE
	`, tagID)
	if err != nil {
		return nil, fmt.Errorf("db: read tracks by tag: %w", err)
	}
	defer rows.Close()

	return scanPlaylistTrackRows(rows, "db: read tracks by tag row")
}

// ReadTagCounts returns a map of tag ID to the number of tracks carrying that
// tag (only counting non-missing tracks with active tags).
func (d *DB) ReadTagCounts() (map[int64]int, error) {
	rows, err := d.Query(`
		SELECT tt.tag_id, COUNT(*)
		FROM track_tags tt
		JOIN tags tg ON tt.tag_id = tg.id
		JOIN tracks t ON tt.track_hash = t.hash
		WHERE tg.deleted_at IS NULL AND t.missing = 0
		GROUP BY tt.tag_id
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read tag counts: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var id int64
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("db: read tag count row: %w", err)
		}
		result[id] = count
	}
	return result, rows.Err()
}
