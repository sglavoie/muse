package favorites

import "github.com/sglavoie/muse/internal/library"

// OpenList transitions from CategoryLevel to ListLevel for the selected category.
func (v View) OpenList() View {
	v.CategoryCursor = v.Cursor
	v.Category = Category(v.Cursor)
	v.Level = ListLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// OpenTracks transitions from ListLevel to TrackLevel with the given tracks.
func (v View) OpenTracks(tracks []library.Track) View {
	v.ListCursor = v.Cursor
	v.ListScrollOffset = v.ScrollOffset
	v.DrilldownTracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	v.SelectedItems = nil
	return v
}

// Back navigates one level up. The restored cursor/scroll for ListLevel are
// clamped against the (possibly shrunk) list so that a favorite removed during
// the drill-down does not leave the cursor pointing past the new end.
func (v View) Back() View {
	switch v.Level {
	case ListLevel:
		v.Level = CategoryLevel
		v.Cursor = v.CategoryCursor
		v.ScrollOffset = 0
		v.Artists = nil
		v.Albums = nil
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v.SelectedItems = nil
	case TrackLevel:
		v.Level = ListLevel
		v.Cursor = v.ListCursor
		v.ScrollOffset = v.ListScrollOffset
		v.DrilldownTracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v.SelectedItems = nil
		v = v.clampRestoredCursor()
	}
	return v
}

// clampRestoredCursor clamps Cursor to [0, itemCount-1] and adjusts
// ScrollOffset to keep the cursor visible. Used after Back() restores a saved
// cursor that may now point past the (possibly shrunk) parent list.
func (v View) clampRestoredCursor() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.Cursor > n-1 {
		v.Cursor = n - 1
	}
	return v.keepCursorVisible()
}
