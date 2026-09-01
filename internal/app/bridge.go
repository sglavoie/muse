package app

import (
	"time"

	"github.com/sglavoie/muse/internal/library"
)

const (
	scanChannelBuffer  = 256
	watchChannelBuffer = 64
)

// UpsertBatcher batches library track upserts to the database.
type UpsertBatcher interface {
	UpsertBatch([]library.Track) error
}

// MissingMarker marks a single track as missing in the database by hash.
type MissingMarker interface {
	MarkTrackMissing(hash string) error
}

// GenrePopulator populates the track_genres junction table after an upsert
// batch. Implementations that satisfy this interface will have their
// PopulateTrackGenres method called automatically by the bridge after each
// batch flush.
type GenrePopulator interface {
	PopulateTrackGenres([]library.Track) error
}

func ensureCreatedAt(track library.Track) library.Track {
	if track.CreatedAt == 0 {
		track.CreatedAt = time.Now().Unix()
	}
	return track
}
