package browse

import (
	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// ArtistsView wraps the artists sub-panel state for the root model.
// It holds the navigation level, cursor position, artist/album/track lists,
// and saved cursors for each drill-down level. Rendering is delegated to the
// artists sub-package.
type ArtistsView struct {
	Level        arttui.Level
	Cursor       int
	ScrollOffset int

	Artists []string
	Albums  []string
	Tracks  []musiclib.Track

	// ArtistCounts is populated alongside artists in deriveCurrentBrowseData
	// and rebuild paths; used to render the right-aligned count column at the
	// artist list level. Independent of sort/filter — counts reflect the full
	// catalog.
	ArtistCounts map[string]int

	SelectedArtist string
	SelectedAlbum  string

	// Saved cursors/scroll for each level (restored on back).
	artistCursor       int
	artistScrollOffset int
	albumCursor        int
	albumScrollOffset  int

	Width  int
	Height int

	Filter         filter.State
	SortState      sorting.SortState
	SelectedHashes map[string]struct{}
}

// RecomputeArtistsSort re-sorts the owned slices for the current level.
func (v *ArtistsView) setSize(w, h int) { v.Width = w; v.Height = h }

func (v ArtistsView) RecomputeArtistsSort() ArtistsView {
	if v.SortState.Field == sorting.SortDefault {
		return v
	}
	switch v.Level {
	case arttui.ArtistLevel:
		if len(v.Artists) > 0 {
			v.Artists = sorting.SortStrings(v.Artists, v.SortState.Dir)
		}
	case arttui.AlbumLevel:
		if len(v.Albums) > 0 {
			v.Albums = sorting.SortStrings(v.Albums, v.SortState.Dir)
		}
	case arttui.TrackLevel:
		if sorted := sorting.SortTracks(v.Tracks, v.SortState.Field, v.SortState.Dir); sorted != nil {
			v.Tracks = sorted
		}
	}
	return v
}

func (v ArtistsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	av := arttui.View{
		Level:              v.Level,
		Cursor:             v.Cursor,
		ScrollOffset:       v.ScrollOffset,
		Artists:            v.Artists,
		Albums:             v.Albums,
		Tracks:             v.Tracks,
		ArtistCounts:       v.ArtistCounts,
		SelectedArtist:     v.SelectedArtist,
		SelectedAlbum:      v.SelectedAlbum,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
		Width:              v.Width,
		Height:             v.Height,
		Styles:             s,
		FilterActive:       v.Filter.Active,
		FilterText:         v.Filter.Text,
		FilterView:         v.Filter.View(),
		FilterHint: func() string {
			if v.Level == arttui.TrackLevel {
				return "a: artist  t: title  l: album  g: genre  aa: album artist"
			}
			return ""
		}(),
		StripViewLabel:  "Artists",
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

// HandleAction executes a pre-resolved artists action dispatched by root.Update.
func (v ArtistsView) HandleAction(a keys.Action) ArtistsView {
	view := arttui.View{
		Level:              v.Level,
		Cursor:             v.Cursor,
		ScrollOffset:       v.ScrollOffset,
		Artists:            v.Artists,
		Albums:             v.Albums,
		Tracks:             v.Tracks,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		AlbumCursor:        v.albumCursor,
		AlbumScrollOffset:  v.albumScrollOffset,
		Height:             v.Height,
		FilterActive:       v.Filter.Active,
		FilterText:         v.Filter.Text,
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
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	return v
}

// OpenAlbums transitions the view to AlbumLevel with the given album list.
// The current artist cursor and scroll offset are saved for restoration by back.
func (v ArtistsView) OpenAlbums(albums []string) ArtistsView {
	view := arttui.View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
	}
	view = view.OpenAlbums(albums)
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.artistCursor = view.ArtistCursor
	v.artistScrollOffset = view.ArtistScrollOffset
	v.Albums = view.Albums
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// OpenTracks transitions the view to TrackLevel with the given track list.
// The current album cursor and scroll offset are saved for restoration by back.
func (v ArtistsView) OpenTracks(tracks []musiclib.Track) ArtistsView {
	view := arttui.View{
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

// Back navigates one level up. At AlbumLevel it returns to ArtistLevel;
// at TrackLevel it returns to AlbumLevel (or ArtistLevel if albums were
// skipped). At ArtistLevel it is a no-op.
func (v ArtistsView) Back() ArtistsView {
	view := arttui.View{
		Level:              v.Level,
		Cursor:             v.Cursor,
		ScrollOffset:       v.ScrollOffset,
		Artists:            v.Artists,
		Albums:             v.Albums,
		Tracks:             v.Tracks,
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
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.Albums = view.Albums
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// ClampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list. Call after refreshing the artist list when the
// library changes. When a filter is active, the cursor indexes into the
// filtered results, so the count must reflect the filtered length.
func (v ArtistsView) ClampCursor() ArtistsView {
	var n int
	switch v.Level {
	case arttui.AlbumLevel:
		n = len(arttui.FilterStrings(v.Albums, v.Filter.Text))
	case arttui.TrackLevel:
		n = len(arttui.FilterTracks(v.Tracks, v.Filter.Text))
	default:
		n = len(arttui.FilterStrings(v.Artists, v.Filter.Text))
	}
	v.Cursor, v.ScrollOffset = sorting.ClampCursorToCount(v.Cursor, v.ScrollOffset, n)
	return v
}

// FocusedArtist returns the artist name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v ArtistsView) FocusedArtist() (string, bool) {
	if v.Level != arttui.ArtistLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.Artists, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return "", false
	}
	return items[v.Cursor], true
}

// FocusedAlbum returns the album name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v ArtistsView) FocusedAlbum() (string, bool) {
	if v.Level != arttui.AlbumLevel {
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
func (v ArtistsView) FocusedTrack() (musiclib.Track, bool) {
	if v.Level != arttui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.Tracks, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.Cursor], true
}

func (v ArtistsView) EnqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}
