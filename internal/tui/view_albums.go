package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// AlbumsView wraps the albums sub-panel state for the root model.
// It holds the navigation level, cursor position, album/track lists,
// and saved cursors for drill-down. Rendering is delegated to the
// albums sub-package.
type AlbumsView struct {
	level        albtui.Level
	cursor       int
	scrollOffset int

	albums []string
	tracks []musiclib.Track

	selectedAlbum string

	// Saved cursor/scroll for album level (restored on back).
	albumCursor       int
	albumScrollOffset int

	width  int
	height int

	filter    filter.State
	sortState SortState
}

// recomputeAlbumsSort re-sorts the owned slices for the current level.
func (v AlbumsView) recomputeAlbumsSort() AlbumsView {
	if v.sortState.Field == SortDefault {
		return v
	}
	switch v.level {
	case albtui.AlbumLevel:
		if len(v.albums) > 0 {
			v.albums = SortStrings(v.albums, v.sortState.Dir)
		}
	case albtui.TrackLevel:
		if sorted := SortTracks(v.tracks, v.sortState.Field, v.sortState.Dir); sorted != nil {
			v.tracks = sorted
		}
	}
	return v
}

func (v AlbumsView) render(s styles.Styles, favHashes map[string]struct{}) string {
	var sortInd string
	if v.sortState.Field != SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", SortFieldLabel(v.sortState.Field), SortDirIndicator(v.sortState.Dir))
	}
	av := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		SelectedAlbum:     v.selectedAlbum,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Width:             v.width,
		Height:            v.height,
		Styles:            s,
		FilterActive:      v.filter.Active,
		FilterText:        v.filter.Text,
		SortIndicator:     sortInd,
		FavoriteHashes:    favHashes,
	}
	return av.Render()
}

// HandleAction executes a pre-resolved albums action dispatched by root.Update.
func (v AlbumsView) HandleAction(a action) AlbumsView {
	view := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Height:            v.height,
		FilterText:        v.filter.Text,
	}
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	case actionHalfPageDown:
		view = view.HalfPageDown()
	case actionHalfPageUp:
		view = view.HalfPageUp()
	case actionFullPageDown:
		view = view.FullPageDown()
	case actionFullPageUp:
		view = view.FullPageUp()
	case actionGoToTop:
		view = view.GoToTop()
	case actionGoToBottom:
		view = view.GoToBottom()
	}
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// openTracks transitions the view to TrackLevel with the given track list.
// The current album cursor and scroll offset are saved for restoration by back.
func (v AlbumsView) openTracks(tracks []musiclib.Track) AlbumsView {
	view := albtui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.albumCursor = view.AlbumCursor
	v.albumScrollOffset = view.AlbumScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = SortState{}
	return v
}

// back navigates one level up. At TrackLevel it returns to AlbumLevel.
// At AlbumLevel it is a no-op.
func (v AlbumsView) back() AlbumsView {
	view := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
	}
	view = view.Back()
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = SortState{}
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// list. Call after refreshing the album list when the library changes.
func (v AlbumsView) clampCursor() AlbumsView {
	var n int
	switch v.level {
	case albtui.TrackLevel:
		n = len(v.tracks)
	default:
		n = len(v.albums)
	}
	if n == 0 {
		v.cursor = 0
		v.scrollOffset = 0
	} else if v.cursor >= n {
		v.cursor = n - 1
	}
	return v
}

// focusedAlbum returns the album name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v AlbumsView) focusedAlbum() (string, bool) {
	if v.level != albtui.AlbumLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.albums, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedTrack returns the track at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v AlbumsView) focusedTrack() (musiclib.Track, bool) {
	if v.level != albtui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.tracks, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.cursor], true
}

// enqueueFocusedTrack returns a tea.Cmd that appends the focused track to the
// play queue via qa. Returns nil when no track is focused.
func (v AlbumsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	if !ok {
		return nil
	}
	wasEmpty := prevQueueLen == 0
	return func() tea.Msg {
		id, err := qa.AppendTrack(track, "")
		return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
	}
}
