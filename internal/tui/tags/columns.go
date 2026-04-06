package tags

const (
	trackDurationFixed   = 10 // "mm:ss.xxx"
	trackSepCount        = 3  // spaces between four columns
	selectionMarkerFixed = 2  // '●' + space
	minTitleWidth        = 10
	minArtistWidth       = 10
	minAlbumWidth        = 8
)

// TrackColumns holds computed character widths for the tag track-level view
// columns: selection marker, title, artist, album, and duration.
type TrackColumns struct {
	SelectionMarker int
	Title           int
	Artist          int
	Album           int
	Duration        int
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Duration and SelectionMarker receive fixed allocations;
// the remaining space is distributed between Title, Artist, and Album (clamped
// to minimum widths so narrow terminals remain usable).
func ComputeTrack(totalWidth int) TrackColumns {
	fixed := selectionMarkerFixed + trackDurationFixed + trackSepCount
	remaining := totalWidth - fixed
	if remaining < minTitleWidth+minArtistWidth+minAlbumWidth {
		return TrackColumns{
			SelectionMarker: selectionMarkerFixed,
			Title:           minTitleWidth,
			Artist:          minArtistWidth,
			Album:           minAlbumWidth,
			Duration:        trackDurationFixed,
		}
	}
	// Distribute remaining space in ratio 3:2:2 (title gets a bit more).
	title := remaining * 3 / 7
	artist := remaining * 2 / 7
	album := remaining - title - artist
	return TrackColumns{
		SelectionMarker: selectionMarkerFixed,
		Title:           max(title, minTitleWidth),
		Artist:          max(artist, minArtistWidth),
		Album:           max(album, minAlbumWidth),
		Duration:        trackDurationFixed,
	}
}
