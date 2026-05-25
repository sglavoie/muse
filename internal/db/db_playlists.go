package db

import "fmt"

// scanPlaylistTrackRows scans the standard 10-column PlaylistTrackRow SELECT
// result set (id, position, title, artist, album, duration_ms, hash, path,
// track_number, disc_number) into a slice, returning any scan or iteration error.
func scanPlaylistTrackRows(rows rowScanner, errContext string) ([]PlaylistTrackRow, error) {
	var result []PlaylistTrackRow
	for rows.Next() {
		var row PlaylistTrackRow
		if err := rows.Scan(&row.ID, &row.Position, &row.Title, &row.Artist, &row.Album, &row.DurationMs, &row.Hash, &row.Path, &row.TrackNumber, &row.DiscNumber); err != nil {
			return nil, fmt.Errorf("%s: %w", errContext, err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// PlaylistRow holds a single playlist entry with its active track count.
// Used by ReadPlaylists to return typed results for the playlist list view.
// IsSmart is true when the playlist is defined by a query (query IS NOT NULL).
// When IsSmart is true, TrackCount is -1 (tracks are computed on demand) and
// Query holds the raw JSON query string.
type PlaylistRow struct {
	ID         int64
	Name       string
	TrackCount int
	IsSmart    bool
	Query      string
}

// PlaylistTrackRow holds a single playlist track entry with its position and
// track metadata. Used by ReadPlaylistTracks to return typed results for the
// playlist track-level view.
type PlaylistTrackRow struct {
	ID          int64 // playlist_tracks row ID; required for soft-delete operations
	Position    int64
	Title       string
	Artist      string
	Album       string
	DurationMs  int64
	Hash        string // BLAKE3 content hash; required for enqueue operations
	Path        string // file path; required for enqueue operations
	TrackNumber int
	DiscNumber  int
}

// ReadPlaylists returns all active (non-deleted) playlists ordered
// alphabetically by name (case-insensitive), each with its active track count.
// For smart playlists (query IS NOT NULL), TrackCount is -1 and the raw JSON
// query string is populated in the Query field.
// Returns an empty slice (not an error) when no playlists exist.
func (d *DB) ReadPlaylists() ([]PlaylistRow, error) {
	rows, err := d.Query(`
		SELECT p.id, p.name,
		       CASE WHEN p.query IS NOT NULL THEN -1 ELSE COUNT(t.hash) END,
		       p.query IS NOT NULL,
		       COALESCE(p.query, '')
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.id AND pt.deleted_at IS NULL
		LEFT JOIN tracks t ON t.hash = pt.track_hash AND t.missing = 0
		WHERE p.deleted_at IS NULL
		GROUP BY p.id
		ORDER BY p.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read playlists: %w", err)
	}
	defer rows.Close()

	var result []PlaylistRow
	for rows.Next() {
		var row PlaylistRow
		var isSmart int
		if err := rows.Scan(&row.ID, &row.Name, &row.TrackCount, &isSmart, &row.Query); err != nil {
			return nil, fmt.Errorf("db: read playlist row: %w", err)
		}
		row.IsSmart = isSmart != 0
		result = append(result, row)
	}
	return result, rows.Err()
}

// ReadPlaylistTracks returns all tracks for the given playlist ID ordered by
// ascending position, joined with track metadata. Returns an empty slice (not
// an error) when the playlist has no tracks.
func (d *DB) ReadPlaylistTracks(playlistID int64) ([]PlaylistTrackRow, error) {
	rows, err := d.Query(`
		SELECT pt.id, pt.position, t.title, t.artist, t.album, t.duration_ms, t.hash, t.path, t.track_number, t.disc_number
		FROM playlist_tracks pt
		JOIN tracks t ON pt.track_hash = t.hash
		WHERE pt.playlist_id = ? AND pt.deleted_at IS NULL AND t.missing = 0
		ORDER BY pt.position
	`, playlistID)
	if err != nil {
		return nil, fmt.Errorf("db: read playlist tracks: %w", err)
	}
	defer rows.Close()

	return scanPlaylistTrackRows(rows, "db: read playlist track row")
}
