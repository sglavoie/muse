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
// the remaining space is distributed between Title, Artist, and Album.
// When enough space exists, minimum flex widths are preserved and extra room is
// split in ratio 3:2:2. On narrower widths, flex columns are still distributed
// in ratio 3:2:2 without forcing minimums to avoid over-allocating past
// totalWidth.
func ComputeTrack(totalWidth int) TrackColumns {
	fixed := selectionMarkerFixed + trackDurationFixed + trackSepCount
	remaining := totalWidth - fixed
	cols := TrackColumns{
		SelectionMarker: selectionMarkerFixed,
		Title:           0,
		Artist:          0,
		Album:           0,
		Duration:        trackDurationFixed,
	}
	if remaining <= 0 {
		cols.Duration = max(totalWidth-selectionMarkerFixed-trackSepCount, 0)
		return cols
	}

	minFlex := minTitleWidth + minArtistWidth + minAlbumWidth
	if remaining < minFlex {
		// Not enough room for minimums: split the available flex without forcing
		// minimums so rendered rows do not exceed totalWidth.
		title := remaining * 3 / 7
		artist := remaining * 2 / 7
		album := remaining - title - artist
		cols.Title = title
		cols.Artist = artist
		cols.Album = album
		return cols
	}

	// Enough room for minimums: allocate minimums first, then split extra flex.
	extra := remaining - minFlex
	titleExtra := extra * 3 / 7
	artistExtra := extra * 2 / 7
	albumExtra := extra - titleExtra - artistExtra
	cols.Title = minTitleWidth + titleExtra
	cols.Artist = minArtistWidth + artistExtra
	cols.Album = minAlbumWidth + albumExtra
	return cols
}
