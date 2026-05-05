package db

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
)

// ReadCachedMix returns up to limit tracks for the given mix from the
// recommendation_cache table, ordered by score descending. Tracks marked
// missing are excluded. Returns an empty (non-nil) slice when the cache
// holds no rows for the mix.
func (d *DB) ReadCachedMix(mixID, limit int) ([]library.Track, error) {
	rows, err := d.Query(`
		SELECT t.path, t.hash, t.artist, t.title, t.album, t.genre,
		       t.duration_ms, t.created_at, t.mod_time, t.size_bytes,
		       t.play_count, t.last_played_at, t.track_number, t.disc_number
		FROM tracks t
		JOIN recommendation_cache rc ON rc.track_hash = t.hash
		WHERE rc.mix_id = ? AND t.missing = 0
		ORDER BY rc.score DESC
		LIMIT ?`, mixID, limit)
	if err != nil {
		return nil, fmt.Errorf("db: read cached mix: %w", err)
	}
	defer rows.Close()
	tracks, err := scanTrackRows(rows)
	if err != nil {
		return nil, err
	}
	if tracks == nil {
		tracks = []library.Track{}
	}
	return tracks, nil
}
