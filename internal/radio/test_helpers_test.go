package radio

import "github.com/sglavoie/muse/internal/library"

func track(hash, artist, album, genre string, dur float64) library.Track {
	return library.Track{Hash: hash, Artist: artist, Album: album, Genre: genre, Duration: dur}
}

func seedFrom(t library.Track, genres []string, tagIDs map[int64]struct{}) SeedTrack {
	return SeedTrack{
		Hash:     t.Hash,
		Artist:   t.Artist,
		Album:    t.Album,
		Genres:   genres,
		Duration: t.Duration,
		TagIDs:   tagIDs,
	}
}

func scoresByHash(results []TrackScore) map[string]float64 {
	scores := make(map[string]float64, len(results))
	for _, result := range results {
		scores[result.Track.Hash] = result.Score
	}
	return scores
}
