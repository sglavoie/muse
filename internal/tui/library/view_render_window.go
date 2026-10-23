package library

import (
	"strings"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// Materialize only the visible rows of ordinary tables. Large libraries should
// cost the same to paint as small ones; cached match indexes carry highlights.
func (v Panel) renderFlatTracks(tracks []musiclib.Track) string {
	var matches []musiclib.Match
	filtered := false
	if v.Filter.Text != "" {
		matches, filtered = v.filteredMatches(tracks)
	}
	count := len(tracks)
	if filtered {
		count = len(matches)
	}
	start, end := layout.VisibleWindowForCursor(v.Cursor, v.ScrollOffset, count, v.visibleRows(), nil)
	window := make([]displayTrack, end-start)
	for i := range window {
		index := start + i
		if filtered {
			match := matches[index]
			window[i] = displayTrack{track: tracks[match.TrackIndex], matchedIndexes: match.MatchedIndexes,
				artistPositions: match.ArtistPositions, titlePositions: match.TitlePositions, albumPositions: match.AlbumPositions}
		} else {
			window[i] = displayTrack{track: tracks[index]}
		}
	}
	cols := v.resolveColumns()
	date := "Added"
	if v.ShowPlayedAt {
		date = "Played"
	}
	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(renderRow("#", "Artist", "Title", "Album", "Time", date, cols))
	out := v.renderStateStrip(len(tracks), count, start, end) + "\n" + header
	if count == 0 {
		return out + "\n" + v.renderNoMatches()
	}
	run := rowRun{showDisc: anyMultiDisc(window), dimRunsDisabled: v.Filter.Active || v.Filter.Text != ""}
	for i, track := range window {
		out += "\n" + v.renderTrackRow(track, cols, start+i, run)
		run.prevArtist, run.prevAlbum = track.track.Artist, track.track.Album
	}
	return out
}
