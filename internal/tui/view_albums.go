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

	filter         filter.State
	sortState      SortState
	selectedHashes map[string]struct{}
}

// recomputeAlbumsSort re-sorts the owned slices for the current level.
func (v *AlbumsView) setSize(w, h int) { v.width = w; v.height = h }

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

func (v AlbumsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash string) string {
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
		FilterView:        v.filter.View(),
		FilterHint:        func() string {
			if v.level == albtui.TrackLevel {
				return "a: artist  t: title  l: album  g: genre"
			}
			return ""
		}(),
		SortIndicator:     sortInd,
		FavoriteHashes:    favHashes,
		FavoriteArtists:   favArtists,
		FavoriteAlbums:    favAlbums,
		SelectedHashes:    v.selectedHashes,
		NowPlayingHash:    nowPlayingHash,
	}
	return av.Render()
}

// HandleAction executes a pre-resolved albums action dispatched by root.Update.
func (v AlbumsView) HandleAction(a action) AlbumsView {
	var sortInd string
	if v.sortState.Field != SortDefault {
		sortInd = "active" // non-empty triggers reserve in visibleRows
	}
	view := albtui.View{
		Level:             v.level,
		Cursor:            v.cursor,
		ScrollOffset:      v.scrollOffset,
		Albums:            v.albums,
		Tracks:            v.tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Height:            v.height,
		FilterActive:      v.filter.Active,
		FilterText:        v.filter.Text,
		SortIndicator:     sortInd,
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
	v.selectedHashes = nil
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
	v.selectedHashes = nil
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list. Call after refreshing the album list when the
// library changes. When a filter is active, the cursor indexes into the
// filtered results, so the count must reflect the filtered length.
func (v AlbumsView) clampCursor() AlbumsView {
	var n int
	switch v.level {
	case albtui.TrackLevel:
		n = len(arttui.FilterTracks(v.tracks, v.filter.Text))
	default:
		n = len(arttui.FilterStrings(v.albums, v.filter.Text))
	}
	v.cursor, v.scrollOffset = clampCursorToCount(v.cursor, v.scrollOffset, n)
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

func (v AlbumsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

func (v AlbumsView) playNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}
