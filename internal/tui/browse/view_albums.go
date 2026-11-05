package browse

import (
	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// AlbumsView wraps the albums sub-panel state for the root model.
// It holds the navigation level, cursor position, album/track lists,
// and saved cursors for drill-down. Rendering is delegated to the
// albums sub-package.
type AlbumsView struct {
	Level        albtui.Level
	Cursor       int
	ScrollOffset int

	Albums []string
	Tracks []musiclib.Track

	// AlbumCounts is populated alongside albums in deriveCurrentBrowseData
	// and rebuild paths; used to render the right-aligned count column at the
	// album list level. Counts reflect the full catalog regardless of sort/filter.
	AlbumCounts map[string]int

	SelectedAlbum string

	// Layout is the remembered grid/list choice for the album level; the
	// grid needs Artwork (covers enabled) and room for two tiles across.
	// Releases names each album's cover track and artist for its tile, and
	// Covers draws a cover; the root sets Covers just before rendering.
	Layout   albtui.Layout
	Artwork  bool
	Releases map[string]albtui.Release
	Covers   func(album string, cols, rows int) string

	// Saved cursor/scroll for album level (restored on back).
	albumCursor       int
	albumScrollOffset int

	Width  int
	Height int

	Filter         filter.State
	SortState      sorting.SortState
	SelectedHashes map[string]struct{}
}

// RecomputeAlbumsSort re-sorts the owned slices for the current level.
func (v *AlbumsView) setSize(w, h int) { v.Width = w; v.Height = h }

func (v AlbumsView) RecomputeAlbumsSort() AlbumsView {
	if v.SortState.Field == sorting.SortDefault {
		return v
	}
	switch v.Level {
	case albtui.AlbumLevel:
		if len(v.Albums) > 0 {
			v.Albums = sorting.SortStrings(v.Albums, v.SortState.Dir)
		}
	case albtui.TrackLevel:
		if sorted := sorting.SortTracks(v.Tracks, v.SortState.Field, v.SortState.Dir); sorted != nil {
			v.Tracks = sorted
		}
	}
	return v
}

func (v AlbumsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	av := albtui.View{
		Level:             v.Level,
		Cursor:            v.Cursor,
		ScrollOffset:      v.ScrollOffset,
		Albums:            v.Albums,
		Tracks:            v.Tracks,
		AlbumCounts:       v.AlbumCounts,
		SelectedAlbum:     v.SelectedAlbum,
		Layout:            v.Layout,
		Artwork:           v.Artwork,
		Releases:          v.Releases,
		Covers:            v.Covers,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Width:             v.Width,
		Height:            v.Height,
		Styles:            s,
		FilterActive:      v.Filter.Active,
		FilterText:        v.Filter.Text,
		FilterView:        v.Filter.View(),
		FilterHint: func() string {
			if v.Level == albtui.TrackLevel {
				return "a: artist  t: title  l: album  g: genre  aa: album artist"
			}
			return ""
		}(),
		StripViewLabel:  "Albums",
		FavoriteHashes:  favHashes,
		FavoriteArtists: favArtists,
		FavoriteAlbums:  favAlbums,
		SelectedHashes:  v.SelectedHashes,
		NowPlayingHash:  nowPlayingHash,
		NowPlayingIcon:  nowPlayingIcon,
	}
	if v.SortState.Field != sorting.SortDefault {
		av.StripSortLabel = sorting.SortFieldLabel(v.SortState.Field)
		av.StripSortIndicator = sorting.SortDirIndicator(v.SortState.Dir)
	}
	return av.Render()
}

// HandleAction executes a pre-resolved albums action dispatched by root.Update.
// In the cover grid, up and down move a whole row of tiles and the page keys
// move by rows; ActionCursorLeft and ActionCursorRight only act there.
func (v AlbumsView) HandleAction(a keys.Action) AlbumsView {
	view := v.navView()
	if g, ok := view.Grid(); ok {
		view = gridAction(view, g, a)
	} else {
		view = listAction(view, a)
	}
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	return v
}

// navView is the albums render struct with just what navigation reads.
func (v AlbumsView) navView() albtui.View {
	return albtui.View{
		Level:             v.Level,
		Cursor:            v.Cursor,
		ScrollOffset:      v.ScrollOffset,
		Albums:            v.Albums,
		Tracks:            v.Tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
		Layout:            v.Layout,
		Artwork:           v.Artwork,
		Width:             v.Width,
		Height:            v.Height,
		FilterActive:      v.Filter.Active,
		FilterText:        v.Filter.Text,
	}
}

func listAction(view albtui.View, a keys.Action) albtui.View {
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
	return view
}

func gridAction(view albtui.View, g albtui.Grid, a keys.Action) albtui.View {
	switch a {
	case keys.ActionCursorUp:
		return view.MoveInGrid(-1, 0)
	case keys.ActionCursorDown:
		return view.MoveInGrid(1, 0)
	case keys.ActionCursorLeft:
		return view.MoveInGrid(0, -1)
	case keys.ActionCursorRight:
		return view.MoveInGrid(0, 1)
	case keys.ActionHalfPageDown:
		return view.MoveInGrid(g.PageRows(false), 0)
	case keys.ActionHalfPageUp:
		return view.MoveInGrid(-g.PageRows(false), 0)
	case keys.ActionFullPageDown:
		return view.MoveInGrid(g.PageRows(true), 0)
	case keys.ActionFullPageUp:
		return view.MoveInGrid(-g.PageRows(true), 0)
	case keys.ActionGoToTop, keys.ActionGoToBottom:
		return listAction(view, a).KeepGridVisible()
	}
	return view
}

// GridActive reports whether the album level is drawn as the cover grid.
func (v AlbumsView) GridActive() bool {
	_, ok := v.navView().Grid()
	return ok
}

// KeepGridVisible scrolls the cover grid so the focused tile's row shows;
// it leaves the list untouched.
func (v AlbumsView) KeepGridVisible() AlbumsView {
	view := v.navView().KeepGridVisible()
	v.Cursor, v.ScrollOffset = view.Cursor, view.ScrollOffset
	return v
}

// VisibleTiles returns the albums whose cover tiles are on screen and the
// grid geometry, or false when the album level is drawn as a list.
func (v AlbumsView) VisibleTiles() ([]string, albtui.Grid, bool) {
	return v.navView().VisibleTiles()
}

// ToggleLayout switches the album level between the cover grid and the list.
func (v AlbumsView) ToggleLayout() AlbumsView {
	view := v.navView().ToggleLayout()
	v.Layout, v.Cursor, v.ScrollOffset = view.Layout, view.Cursor, view.ScrollOffset
	return v
}

// OpenTracks transitions the view to TrackLevel with the given track list.
// The current album cursor and scroll offset are saved for restoration by back.
func (v AlbumsView) OpenTracks(tracks []musiclib.Track) AlbumsView {
	view := albtui.View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.albumCursor = view.AlbumCursor
	v.albumScrollOffset = view.AlbumScrollOffset
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// Back navigates one level up. At TrackLevel it returns to AlbumLevel.
// At AlbumLevel it is a no-op.
func (v AlbumsView) Back() AlbumsView {
	view := albtui.View{
		Level:             v.Level,
		Cursor:            v.Cursor,
		ScrollOffset:      v.ScrollOffset,
		Albums:            v.Albums,
		Tracks:            v.Tracks,
		AlbumCursor:       v.albumCursor,
		AlbumScrollOffset: v.albumScrollOffset,
	}
	view = view.Back()
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// ClampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list. Call after refreshing the album list when the
// library changes. When a filter is active, the cursor indexes into the
// filtered results, so the count must reflect the filtered length.
func (v AlbumsView) ClampCursor() AlbumsView {
	var n int
	switch v.Level {
	case albtui.TrackLevel:
		n = len(arttui.FilterTracks(v.Tracks, v.Filter.Text))
	default:
		n = len(arttui.FilterStrings(v.Albums, v.Filter.Text))
	}
	v.Cursor, v.ScrollOffset = sorting.ClampCursorToCount(v.Cursor, v.ScrollOffset, n)
	return v
}

// FocusedAlbum returns the album name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v AlbumsView) FocusedAlbum() (string, bool) {
	if v.Level != albtui.AlbumLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.Albums, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return "", false
	}
	return items[v.Cursor], true
}

// FocusedTrack returns the track at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v AlbumsView) FocusedTrack() (musiclib.Track, bool) {
	if v.Level != albtui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.Tracks, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.Cursor], true
}

func (v AlbumsView) EnqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}
