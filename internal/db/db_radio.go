package db

import "fmt"

// ReadRecentPlayedHashes returns the hashes of the most recently played
// tracks, deduplicated, oldest-first among the newest `limit` distinct tracks.
//
// Radio's recent-history avoidance is an in-memory ring buffer that starts
// empty on every launch, so tracks heard in a previous session carry no
// penalty. Seeding that ring at restore is what this reader exists for; the
// oldest-first ordering matches the ring's append-and-truncate-from-the-front
// semantics, so the newest track sits at the end and survives the longest.
//
// Reads the counted-only play_history view: a skipped track was not really
// "recently heard" and should not be avoided because of the skip.
func (d *DB) ReadRecentPlayedHashes(limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := d.Query(`
		SELECT track_hash FROM (
		    SELECT track_hash, MAX(played_at) AS last_at
		    FROM play_history
		    GROUP BY track_hash
		    ORDER BY last_at DESC
		    LIMIT ?
		)
		ORDER BY last_at ASC`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read recent played hashes: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("db: scan recent played hash: %w", err)
		}
		out = append(out, hash)
	}
	return out, rows.Err()
}
