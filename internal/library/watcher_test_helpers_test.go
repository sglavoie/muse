package library

import "context"

// collectTracks drains ch and returns all non-deletion Track values, stopping
// once ch is closed or ctx is cancelled.
func collectTracks(ctx context.Context, ch <-chan WatchEvent) []Track {
	var tracks []Track
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return tracks
			}
			if !e.Deleted {
				tracks = append(tracks, e.Track)
			}
		case <-ctx.Done():
			return tracks
		}
	}
}
