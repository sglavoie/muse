package library

import (
	"fmt"
	"strings"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// displayTrack pairs a track with its fuzzy match positions for highlight rendering.
type displayTrack struct {
	track           musiclib.Track
	matchedIndexes  []int
	artistPositions []int
	titlePositions  []int
	albumPositions  []int
}

// Render returns the complete library panel string. When Width is zero (before
// the first terminal size message) it uses a simple single-line format.
func (v Panel) Render() string {
	allTracks := v.tracks()
	if len(allTracks) == 0 && v.Filter.Text == "" && !v.TagFilterActive {
		return v.renderEmpty()
	}
	if v.BucketLabel == nil && v.Width > 0 {
		return v.renderFlatTracks(allTracks)
	}

	display := v.resolveDisplayTracks(allTracks)
	maxRows := v.visibleRows()
	start, end := layout.VisibleWindowForCursor(v.Cursor, v.ScrollOffset, len(display), maxRows, v.bucketLabelAt(display))
	start, end = v.rebucketedWindow(display, start, end, maxRows)

	if v.Width == 0 {
		return v.renderZeroWidthFallback(display, start, end)
	}

	cols := v.resolveColumns()
	dateColLabel := "Added"
	if v.ShowPlayedAt {
		dateColLabel = "Played"
	}
	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(renderRow("#", "Artist", "Title", "Album", "Time", dateColLabel, cols))
	stripLine := v.renderStateStrip(len(allTracks), len(display))

	if len(display) == 0 {
		return stripLine + "\n" + header + "\n" + v.renderNoMatches()
	}
	if start >= end {
		return stripLine + "\n" + header
	}

	lines := v.renderRows(display, start, end, cols)
	out, drawn := v.chunkIntoBuckets(lines, display, start, maxRows)
	stripLine = v.renderStateStrip(len(allTracks), len(display), start, start+drawn)
	return stripLine + "\n" + header + "\n" + strings.Join(out, "\n")
}

// resolveDisplayTracks returns the tracks to render, carrying fuzzy match
// positions when an active filter narrowed the list.
func (v Panel) resolveDisplayTracks(allTracks []musiclib.Track) []displayTrack {
	if v.Filter.Text != "" {
		if matches, ok := v.filteredMatches(allTracks); ok {
			display := make([]displayTrack, len(matches))
			for i, m := range matches {
				display[i] = displayTrack{
					track:           allTracks[m.TrackIndex],
					matchedIndexes:  m.MatchedIndexes,
					artistPositions: m.ArtistPositions,
					titlePositions:  m.TitlePositions,
					albumPositions:  m.AlbumPositions,
				}
			}
			return display
		}
	}
	display := make([]displayTrack, len(allTracks))
	for i, t := range allTracks {
		display[i] = displayTrack{track: t}
	}
	return display
}

// bucketLabelAt adapts BucketLabel to the index-based callback the shared
// window helper takes, or returns nil when bucketing is off.
func (v Panel) bucketLabelAt(display []displayTrack) func(int) string {
	if v.BucketLabel == nil {
		return nil
	}
	return func(i int) string { return v.BucketLabel(display[i].track) }
}

// rebucketedWindow re-anchors the window on the cursor when the bucket headers
// and dividers drawn inside it would push the cursor's row off the bottom.
// It returns the window unchanged when the cursor row still fits, or when
// bucketing is off.
func (v Panel) rebucketedWindow(display []displayTrack, start, end, maxRows int) (int, int) {
	if v.BucketLabel == nil || maxRows <= 0 || v.Cursor < 0 || v.Cursor >= len(display) || start >= end {
		return start, end
	}
	labels := make([]string, end-start)
	for i := range labels {
		labels[i] = v.BucketLabel(display[start+i].track)
	}
	if bucketedWindowIncludesRow(labels, v.Cursor-start, maxRows) {
		return start, end
	}
	// Keep the same number of data rows, now anchored at the cursor.
	return v.Cursor, min(len(display), v.Cursor+(end-start))
}

// renderZeroWidthFallback renders the panel before the first terminal size
// message arrives: the legacy "Filter: ..." line so the user can still see
// their query, followed by plain "artist – title" rows.
func (v Panel) renderZeroWidthFallback(display []displayTrack, start, end int) string {
	separator := ""
	switch {
	case v.Filter.Active && v.Filter.Text == "":
		hint := v.Styles.Dim.Render("a: artist  t: title  l: album  g: genre  aa: album artist")
		separator = "Filter: " + hint + "\n"
	case v.Filter.Active:
		separator = "Filter: " + v.Filter.View() + "\n"
	case v.Filter.Text != "":
		separator = "Filter: " + v.Filter.Text + "\n"
	}
	if len(display) == 0 {
		return separator + filter.NoMatchesMessage(v.Filter.Text != "", v.TagFilterActive)
	}
	if start >= end {
		return strings.TrimSuffix(separator, "\n")
	}
	lines := make([]string, end-start)
	for i, dt := range display[start:end] {
		row := fmt.Sprintf("%s – %s", dt.track.Artist, dt.track.Title)
		if start+i == v.Cursor {
			row = v.Styles.Accent.Render(row)
		}
		lines[i] = row
	}
	return separator + strings.Join(lines, "\n")
}

// resolveColumns picks the column layout for the optional trailing date column.
func (v Panel) resolveColumns() Columns {
	switch {
	case v.ShowPlayedAt:
		return ComputeWithPlayedAt(v.Width)
	case v.ShowDateAdded:
		return ComputeWithDateAdded(v.Width)
	default:
		return Compute(v.Width)
	}
}
