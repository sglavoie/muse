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
		reserve = 1 // collection heading
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
	// Favorites pulls an overshooting cursor back to the last item so the
	// window follows it; the shared helper deliberately ignores an
	// out-of-range cursor, so clamp here rather than there.
	cursor := min(v.Cursor, itemCount-1)
	return layout.VisibleWindowForCursor(cursor, v.ScrollOffset, itemCount, v.visibleRows(), nil)
}
