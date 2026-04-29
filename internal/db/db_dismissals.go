package db

import "fmt"

// ReadDismissedTrackHashes returns the set of track hashes that have been
// dismissed from recommendation mixes. Returns an empty map (not nil) when
// no dismissals exist.
func (d *DB) ReadDismissedTrackHashes() (map[string]struct{}, error) {
	rows, err := d.Query(
		`SELECT value FROM recommendation_dismissals WHERE type = 'track'`)
	if err != nil {
		return nil, fmt.Errorf("db: read dismissed track hashes: %w", err)
	}
	defer rows.Close()

	result := make(map[string]struct{})
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("db: scan dismissed track hash: %w", err)
		}
		result[hash] = struct{}{}
	}
	return result, rows.Err()
}
