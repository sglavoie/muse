package browse

import (
	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// GenresView wraps the genres sub-panel state for the root model.
// It holds the navigation level, cursor position, genre/artist/track lists,
// and saved cursors for each drill-down level. Rendering is delegated to the
// genres sub-package.
type GenresView struct {
	Level        gentui.Level
	Cursor       int
	ScrollOffset int

	Genres  []string
	Artists []string
	Tracks  []musiclib.Track

	// GenreCounts is populated alongside genres in deriveCurrentBrowseData
	// and rebuild paths; used to render the right-aligned count column at the
	// genre list level. Counts reflect the full catalog regardless of sort/filter.
	GenreCounts map[string]int

	SelectedGenre  string
	SelectedArtist string

	// Saved cursors/scroll for each level (restored on back).
	genreCursor        int
	genreScrollOffset  int
	artistCursor       int
	artistScrollOffset int

	Width  int
	Height int

	Filter         filter.State
	SortState      sorting.SortState
	SelectedHashes map[string]struct{}
}

func (v *GenresView) setSize(w, h int) { v.Width = w; v.Height = h }

func (v GenresView) RecomputeGenresSort() GenresView {
	if v.SortState.Field == sorting.SortDefault {
		return v
	}
	switch v.Level {
	case gentui.GenreLevel:
		if len(v.Genres) > 0 {
			v.Genres = sorting.SortStrings(v.Genres, v.SortState.Dir)
		}
	case gentui.ArtistLevel:
		if len(v.Artists) > 0 {
			v.Artists = sorting.SortStrings(v.Artists, v.SortState.Dir)
		}
	case gentui.TrackLevel:
		if sorted := sorting.SortTracks(v.Tracks, v.SortState.Field, v.SortState.Dir); sorted != nil {
			v.Tracks = sorted
		}
	}
	return v
}

func (v GenresView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	gv := gentui.View{
		Level:              v.Level,
		Cursor:             v.Cursor,
		ScrollOffset:       v.ScrollOffset,
		Genres:             v.Genres,
		Artists:            v.Artists,
		Tracks:             v.Tracks,
		GenreCounts:        v.GenreCounts,
		SelectedGenre:      v.SelectedGenre,
		SelectedArtist:     v.SelectedArtist,
		GenreCursor:        v.genreCursor,
		GenreScrollOffset:  v.genreScrollOffset,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		Width:              v.Width,
		Height:             v.Height,
		Styles:             s,
		FilterActive:       v.Filter.Active,
		FilterText:         v.Filter.Text,
		FilterView:         v.Filter.View(),
		FilterHint: func() string {
			if v.Level == gentui.TrackLevel {
				return "a: artist  t: title  l: album  g: genre  aa: album artist"
			}
			return ""
		}(),
		StripViewLabel:  "Genres",
		FavoriteHashes:  favHashes,
		FavoriteArtists: favArtists,
		FavoriteAlbums:  favAlbums,
		SelectedHashes:  v.SelectedHashes,
		NowPlayingHash:  nowPlayingHash,
		NowPlayingIcon:  nowPlayingIcon,
	}
	if v.SortState.Field != sorting.SortDefault {
		gv.StripSortLabel = sorting.SortFieldLabel(v.SortState.Field)
		gv.StripSortIndicator = sorting.SortDirIndicator(v.SortState.Dir)
	}
	return gv.Render()
}

// HandleAction executes a pre-resolved genres action dispatched by root.Update.
func (v GenresView) HandleAction(a keys.Action) GenresView {
	view := gentui.View{
		Level:              v.Level,
		Cursor:             v.Cursor,
		ScrollOffset:       v.ScrollOffset,
		Genres:             v.Genres,
		Artists:            v.Artists,
		Tracks:             v.Tracks,
		GenreCursor:        v.genreCursor,
		GenreScrollOffset:  v.genreScrollOffset,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
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

// OpenArtists transitions the view to ArtistLevel with the given artist list.
// The current genre cursor and scroll offset are saved for restoration by back.
func (v GenresView) OpenArtists(artists []string) GenresView {
	view := gentui.View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
	}
	view = view.OpenArtists(artists)
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.genreCursor = view.GenreCursor
	v.genreScrollOffset = view.GenreScrollOffset
	v.Artists = view.Artists
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// OpenTracks transitions the view to TrackLevel with the given track list.
// The current artist cursor and scroll offset are saved for restoration by back.
func (v GenresView) OpenTracks(tracks []musiclib.Track) GenresView {
	view := gentui.View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.artistCursor = view.ArtistCursor
	v.artistScrollOffset = view.ArtistScrollOffset
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// Back navigates one level up. At ArtistLevel it returns to GenreLevel;
// at TrackLevel it returns to ArtistLevel (or GenreLevel if artists were
// skipped). At GenreLevel it is a no-op.
func (v GenresView) Back() GenresView {
	view := gentui.View{
		Level:              v.Level,
		Cursor:             v.Cursor,
		ScrollOffset:       v.ScrollOffset,
		Genres:             v.Genres,
		Artists:            v.Artists,
		Tracks:             v.Tracks,
		GenreCursor:        v.genreCursor,
		GenreScrollOffset:  v.genreScrollOffset,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
	}
	view = view.Back()
	// When the artist level was skipped (no artists), go straight back to genres.
	if view.Level == gentui.ArtistLevel && len(view.Artists) == 0 {
		view = view.Back()
	}
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.Artists = view.Artists
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedHashes = nil
	return v
}

// ClampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list.
func (v GenresView) ClampCursor() GenresView {
	var n int
	switch v.Level {
	case gentui.ArtistLevel:
		n = len(arttui.FilterStrings(v.Artists, v.Filter.Text))
	case gentui.TrackLevel:
		n = len(arttui.FilterTracks(v.Tracks, v.Filter.Text))
	default:
		n = len(arttui.FilterStrings(v.Genres, v.Filter.Text))
	}
	v.Cursor, v.ScrollOffset = sorting.ClampCursorToCount(v.Cursor, v.ScrollOffset, n)
	return v
}

// FocusedGenre returns the genre name at the current cursor position and true.
func (v GenresView) FocusedGenre() (string, bool) {
	if v.Level != gentui.GenreLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.Genres, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return "", false
	}
	return items[v.Cursor], true
}

// FocusedArtist returns the artist name at the current cursor position and true.
func (v GenresView) FocusedArtist() (string, bool) {
	if v.Level != gentui.ArtistLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.Artists, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return "", false
	}
	return items[v.Cursor], true
}

// FocusedTrack returns the track at the current cursor position and true.
func (v GenresView) FocusedTrack() (musiclib.Track, bool) {
	if v.Level != gentui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.Tracks, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.Cursor], true
}

func (v GenresView) EnqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

func (v GenresView) PlayNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}
