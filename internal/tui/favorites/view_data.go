package favorites

import (
	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// EffectiveItems returns the list items filtered by FilterText for the current category.
func (v View) EffectiveItems() []string {
	switch v.Category {
	case CategoryArtists:
		return arttui.FilterStrings(v.Artists, v.FilterText)
	case CategoryAlbums:
		return arttui.FilterStrings(v.Albums, v.FilterText)
	default:
		return nil
	}
}

// EffectiveTracks returns the track list filtered by FilterText.
// At TrackLevel with drill-down it uses DrilldownTracks; at ListLevel with
// CategoryTracks it uses Tracks.
func (v View) EffectiveTracks() []library.Track {
	tracks := v.currentTracks()
	return arttui.FilterTracks(tracks, v.FilterText)
}

// currentTracks returns the appropriate track slice for the current level/category.
func (v View) currentTracks() []library.Track {
	if v.Level == TrackLevel {
		return v.DrilldownTracks
	}
	return v.Tracks
}

// visibleRows returns the maximum number of rows that fit on screen.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case CategoryLevel:
		// No chrome needed for the 3-row category view.
	case ListLevel:
		reserve = 2 // heading + separator (blank line or filter line)
		if v.Category == CategoryTracks {
			reserve++ // column header row
		}
	default:
		reserve = 3 // heading + separator + column header row
	}
	if v.SortIndicator != "" {
		reserve++
	}
	return layout.VisibleRows(v.Height, reserve)
}

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	switch v.Level {
	case CategoryLevel:
		return 3
	case ListLevel:
		if v.Category == CategoryTracks {
			return len(v.EffectiveTracks())
		}
		return len(v.EffectiveItems())
	case TrackLevel:
		return len(v.EffectiveTracks())
	default:
		return 0
	}
}

// visibleWindow returns the clamped [start,end) indexes for the visible slice
// window. ScrollOffset is state restored from the parent model, so keep render
// paths resilient to stale negative or overlarge values.
func (v View) visibleWindow(itemCount int) (int, int) {
	rows := v.visibleRows()
	if itemCount <= 0 || rows <= 0 {
		return 0, 0
	}
	maxStart := max(itemCount-rows, 0)
	start := min(max(v.ScrollOffset, 0), maxStart)
	if v.Cursor >= 0 {
		cursor := min(v.Cursor, itemCount-1)
		if cursor < start {
			start = cursor
		} else if cursor >= start+rows {
			start = cursor - rows + 1
		}
	}
	start = min(max(start, 0), maxStart)
	end := min(itemCount, start+rows)
	return start, end
}
