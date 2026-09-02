package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// FavoritesView wraps the favorites sub-panel state for the root model.
// It holds the navigation level, cursor position, category counts, and
// favorite item lists. Rendering is delegated to the favorites sub-package.
type FavoritesView struct {
	level        favtui.Level
	cursor       int
	scrollOffset int

	category favtui.Category

	// Category counts (shown at CategoryLevel).
	artistCount int
	albumCount  int
	trackCount  int

	// List items (populated at ListLevel).
	artists []string
	albums  []string
	tracks  []musiclib.Track

	// Drill-down tracks (populated at TrackLevel from a favorite artist/album).
	drilldownTracks []musiclib.Track
	drilldownName   string // heading text for TrackLevel (artist or album name)

	// Saved cursors/scroll for each level (restored on back).
	categoryCursor   int
	listCursor       int
	listScrollOffset int

	width  int
	height int

	filter        filter.State
	sortState     sorting.SortState
	selectedItems map[int]struct{}
}

func (v *FavoritesView) SetSize(w, h int) { v.width = w; v.height = h }

func (v FavoritesView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	var sortInd string
	if v.sortState.Field != sorting.SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", sorting.SortFieldLabel(v.sortState.Field), sorting.SortDirIndicator(v.sortState.Dir))
	}
	fv := favtui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ScrollOffset:     v.scrollOffset,
		Category:         v.category,
		ArtistCount:      v.artistCount,
		AlbumCount:       v.albumCount,
		TrackCount:       v.trackCount,
		Artists:          v.artists,
		Albums:           v.albums,
		Tracks:           v.tracks,
		DrilldownTracks:  v.drilldownTracks,
		DrilldownName:    v.drilldownName,
		CategoryCursor:   v.categoryCursor,
		ListCursor:       v.listCursor,
		ListScrollOffset: v.listScrollOffset,
		Width:            v.width,
		Height:           v.height,
		Styles:           s,
		FilterActive:     v.filter.Active,
		FilterText:       v.filter.Text,
		FilterView:       v.filter.View(),
		FilterHint: func() string {
			if v.level == favtui.TrackLevel || (v.level == favtui.ListLevel && v.category == favtui.CategoryTracks) {
				return "a: artist  t: title  l: album  g: genre"
			}
			return ""
		}(),
		SortIndicator:   sortInd,
		SelectedItems:   v.selectedItems,
		FavoriteHashes:  favHashes,
		FavoriteArtists: favArtists,
		FavoriteAlbums:  favAlbums,
		NowPlayingHash:  nowPlayingHash,
		NowPlayingIcon:  nowPlayingIcon,
	}
	return fv.Render()
}

// HandleAction executes a pre-resolved favorites action dispatched by root.Update.
func (v FavoritesView) HandleAction(a keys.Action) FavoritesView {
	var sortInd string
	if v.sortState.Field != sorting.SortDefault {
		sortInd = "active" // non-empty triggers reserve in visibleRows
	}
	view := favtui.View{
		Level:           v.level,
		Cursor:          v.cursor,
		ScrollOffset:    v.scrollOffset,
		Category:        v.category,
		Artists:         v.artists,
		Albums:          v.albums,
		Tracks:          v.tracks,
		DrilldownTracks: v.drilldownTracks,
		DrilldownName:   v.drilldownName,
		Height:          v.height,
		FilterActive:    v.filter.Active,
		FilterText:      v.filter.Text,
		SortIndicator:   sortInd,
	}
	switch a {
	case keys.ActionCursorUp:
		view = view.CursorUp()
	case keys.ActionCursorDown:
		view = view.CursorDown()
	case keys.ActionHalfPageDown:
		view = view.HalfPageDown()
	case keys.ActionHalfPageUp:
		view = view.HalfPageUp()
	case keys.ActionFullPageDown:
		view = view.FullPageDown()
	case keys.ActionFullPageUp:
		view = view.FullPageUp()
	case keys.ActionGoToTop:
		view = view.GoToTop()
	case keys.ActionGoToBottom:
		view = view.GoToBottom()
	}
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// back navigates one level up.
func (v FavoritesView) back() FavoritesView {
	view := favtui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ScrollOffset:     v.scrollOffset,
		Category:         v.category,
		Artists:          v.artists,
		Albums:           v.albums,
		Tracks:           v.tracks,
		DrilldownTracks:  v.drilldownTracks,
		CategoryCursor:   v.categoryCursor,
		ListCursor:       v.listCursor,
		ListScrollOffset: v.listScrollOffset,
	}
	view = view.Back()
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.artists = view.Artists
	v.albums = view.Albums
	v.tracks = view.Tracks
	v.drilldownTracks = view.DrilldownTracks
	v.drilldownName = ""
	v.filter = filter.State{}
	v.sortState = sorting.SortState{}
	v.selectedItems = nil
	return v
}

// openTracks transitions from ListLevel to TrackLevel with the given tracks.
// name is the resolved display name for the heading (e.g. the artist or album
// name that was selected, correctly resolved from the possibly-filtered list).
func (v FavoritesView) openTracks(tracks []musiclib.Track, name string) FavoritesView {
	view := favtui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.listCursor = view.ListCursor
	v.listScrollOffset = view.ListScrollOffset
	v.drilldownTracks = view.DrilldownTracks
	v.drilldownName = name
	v.filter = filter.State{}
	v.sortState = sorting.SortState{}
	v.selectedItems = nil
	return v
}

// focusedArtist returns the artist name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v FavoritesView) focusedArtist() (string, bool) {
	if v.level != favtui.ListLevel || v.category != favtui.CategoryArtists {
		return "", false
	}
	items := arttui.FilterStrings(v.artists, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedAlbum returns the album name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v FavoritesView) focusedAlbum() (string, bool) {
	if v.level != favtui.ListLevel || v.category != favtui.CategoryAlbums {
		return "", false
	}
	items := arttui.FilterStrings(v.albums, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedTrack returns the track at the current cursor position and true.
func (v FavoritesView) focusedTrack() (musiclib.Track, bool) {
	var tracks []musiclib.Track
	switch {
	case v.level == favtui.TrackLevel:
		tracks = arttui.FilterTracks(v.drilldownTracks, v.filter.Text)
	case v.level == favtui.ListLevel && v.category == favtui.CategoryTracks:
		tracks = arttui.FilterTracks(v.tracks, v.filter.Text)
	default:
		return musiclib.Track{}, false
	}
	if v.cursor < 0 || v.cursor >= len(tracks) {
		return musiclib.Track{}, false
	}
	return tracks[v.cursor], true
}

func (v FavoritesView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

func (v FavoritesView) playNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}

// recomputeFavoritesSort re-sorts the owned slices for the current level.
func (v FavoritesView) recomputeFavoritesSort() FavoritesView {
	if v.sortState.Field == sorting.SortDefault {
		return v
	}
	switch v.level {
	case favtui.ListLevel:
		switch v.category {
		case favtui.CategoryArtists:
			if len(v.artists) > 0 {
				v.artists = sorting.SortStrings(v.artists, v.sortState.Dir)
			}
		case favtui.CategoryAlbums:
			if len(v.albums) > 0 {
				v.albums = sorting.SortStrings(v.albums, v.sortState.Dir)
			}
		case favtui.CategoryTracks:
			if sorted := sorting.SortTracks(v.tracks, v.sortState.Field, v.sortState.Dir); sorted != nil {
				v.tracks = sorted
			}
		}
	case favtui.TrackLevel:
		if sorted := sorting.SortTracks(v.drilldownTracks, v.sortState.Field, v.sortState.Dir); sorted != nil {
			v.drilldownTracks = sorted
		}
	}
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list. When a filter is active, the cursor indexes into
// the filtered results, so the count must reflect the filtered length rather
// than the raw backing-store length.
func (v FavoritesView) clampCursor() FavoritesView {
	var n int
	switch v.level {
	case favtui.CategoryLevel:
		n = 3
	case favtui.ListLevel:
		switch v.category {
		case favtui.CategoryArtists:
			n = len(arttui.FilterStrings(v.artists, v.filter.Text))
		case favtui.CategoryAlbums:
			n = len(arttui.FilterStrings(v.albums, v.filter.Text))
		case favtui.CategoryTracks:
			n = len(arttui.FilterTracks(v.tracks, v.filter.Text))
		}
	case favtui.TrackLevel:
		n = len(arttui.FilterTracks(v.drilldownTracks, v.filter.Text))
	}
	v.cursor, v.scrollOffset = sorting.ClampCursorToCount(v.cursor, v.scrollOffset, n)
	return v
}

// hasSelection reports whether any items are bulk-selected.
func (v FavoritesView) hasSelection() bool {
	return len(v.selectedItems) > 0
}

// clearSelection removes all bulk-selection markers.
func (v FavoritesView) clearSelection() FavoritesView {
	v.selectedItems = nil
	return v
}

// toggleSelection toggles bulk-selection on the item at the given index.
func (v FavoritesView) toggleSelection(idx int) FavoritesView {
	if v.selectedItems == nil {
		v.selectedItems = make(map[int]struct{})
	}
	if _, ok := v.selectedItems[idx]; ok {
		delete(v.selectedItems, idx)
	} else {
		v.selectedItems[idx] = struct{}{}
	}
	return v
}

// clampAfterRemoval adjusts cursor and scrollOffset after items are removed
// from a list of the given length. Returns the clamped values.
func clampAfterRemoval(cursor, scrollOffset, length int) (int, int) {
	if length == 0 {
		return 0, 0
	}
	if cursor >= length {
		cursor = length - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	maxOffset := length - 1
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}
	// Ensure the cursor is not above the visible window.
	if scrollOffset > cursor {
		scrollOffset = cursor
	}
	return cursor, scrollOffset
}
