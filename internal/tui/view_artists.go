package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// ArtistsView wraps the artists sub-panel state for the root model.
// It holds the navigation level, cursor position, artist/album/track lists,
// and saved cursors for each drill-down level. Rendering is delegated to the
// artists sub-package.
type ArtistsView struct {
	level        arttui.Level
	cursor       int
	scrollOffset int

	artists []string
	albums  []string
	tracks  []musiclib.Track

	selectedArtist string
	selectedAlbum  string

	// Saved cursors/scroll for each level (restored on back).
	artistCursor       int
	artistScrollOffset int
	albumCursor        int
	albumScrollOffset  int

	width  int
	height int

	filter         filter.State
	sortState      SortState
	selectedHashes map[string]struct{}
}

// recomputeArtistsSort re-sorts the owned slices for the current level.
func (v *ArtistsView) setSize(w, h int) { v.width = w; v.height = h }

func (v ArtistsView) recomputeArtistsSort() ArtistsView {
	if v.sortState.Field == SortDefault {
		return v
	}
	switch v.level {
	case arttui.ArtistLevel:
		if len(v.artists) > 0 {
			v.artists = SortStrings(v.artists, v.sortState.Dir)
		}
	case arttui.AlbumLevel:
		if len(v.albums) > 0 {
			v.albums = SortStrings(v.albums, v.sortState.Dir)
		}
	case arttui.TrackLevel:
		if sorted := SortTracks(v.tracks, v.sortState.Field, v.sortState.Dir); sorted != nil {
			v.tracks = sorted
		}
	}
	return v
}

func (v ArtistsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash string) string {
	var sortInd string
	if v.sortState.Field != SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", SortFieldLabel(v.sortState.Field), SortDirIndicator(v.sortState.Dir))
	}
	av := arttui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Artists:            v.artists,
		Albums:             v.albums,
		Tracks:             v.tracks,
		SelectedArtist:     v.selectedArtist,
		SelectedAlbum:      v.selectedAlbum,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
		Width:              v.width,
		Height:             v.height,
		Styles:             s,
		FilterActive:       v.filter.Active,
		FilterText:         v.filter.Text,
		FilterView:         v.filter.View(),
		SortIndicator:      sortInd,
		FavoriteHashes:     favHashes,
		FavoriteArtists:    favArtists,
		FavoriteAlbums:     favAlbums,
		SelectedHashes:     v.selectedHashes,
		NowPlayingHash:     nowPlayingHash,
	}
	return av.Render()
}

// HandleAction executes a pre-resolved artists action dispatched by root.Update.
func (v ArtistsView) HandleAction(a action) ArtistsView {
	var sortInd string
	if v.sortState.Field != SortDefault {
		sortInd = "active" // non-empty triggers reserve in visibleRows
	}
	view := arttui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Artists:            v.artists,
		Albums:             v.albums,
		Tracks:             v.tracks,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
		Height:             v.height,
		FilterActive:       v.filter.Active,
		FilterText:         v.filter.Text,
		SortIndicator:      sortInd,
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

// openAlbums transitions the view to AlbumLevel with the given album list.
// The current artist cursor and scroll offset are saved for restoration by back.
func (v ArtistsView) openAlbums(albums []string) ArtistsView {
	view := arttui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenAlbums(albums)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.artistCursor = view.ArtistCursor
	v.artistScrollOffset = view.ArtistScrollOffset
	v.albums = view.Albums
	v.filter = filter.State{}
	v.sortState = SortState{}
	v.selectedHashes = nil
	return v
}

// openTracks transitions the view to TrackLevel with the given track list.
// The current album cursor and scroll offset are saved for restoration by back.
func (v ArtistsView) openTracks(tracks []musiclib.Track) ArtistsView {
	view := arttui.View{
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

// back navigates one level up. At AlbumLevel it returns to ArtistLevel;
// at TrackLevel it returns to AlbumLevel (or ArtistLevel if albums were
// skipped). At ArtistLevel it is a no-op.
func (v ArtistsView) back() ArtistsView {
	view := arttui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Artists:            v.artists,
		Albums:             v.albums,
		Tracks:             v.tracks,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
	}
	view = view.Back()
	// When the album level was skipped (no albums), go straight back to artists.
	if view.Level == arttui.AlbumLevel && len(view.Albums) == 0 {
		view = view.Back()
	}
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.albums = view.Albums
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = SortState{}
	v.selectedHashes = nil
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list. Call after refreshing the artist list when the
// library changes. When a filter is active, the cursor indexes into the
// filtered results, so the count must reflect the filtered length.
func (v ArtistsView) clampCursor() ArtistsView {
	var n int
	switch v.level {
	case arttui.AlbumLevel:
		n = len(arttui.FilterStrings(v.albums, v.filter.Text))
	case arttui.TrackLevel:
		n = len(arttui.FilterTracks(v.tracks, v.filter.Text))
	default:
		n = len(arttui.FilterStrings(v.artists, v.filter.Text))
	}
	v.cursor, v.scrollOffset = clampCursorToCount(v.cursor, v.scrollOffset, n)
	return v
}

// focusedArtist returns the artist name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v ArtistsView) focusedArtist() (string, bool) {
	if v.level != arttui.ArtistLevel {
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
func (v ArtistsView) focusedAlbum() (string, bool) {
	if v.level != arttui.AlbumLevel {
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
func (v ArtistsView) focusedTrack() (musiclib.Track, bool) {
	if v.level != arttui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.tracks, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.cursor], true
}

func (v ArtistsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

func (v ArtistsView) playNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}
