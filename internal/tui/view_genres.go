package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// GenresView wraps the genres sub-panel state for the root model.
// It holds the navigation level, cursor position, genre/artist/track lists,
// and saved cursors for each drill-down level. Rendering is delegated to the
// genres sub-package.
type GenresView struct {
	level        gentui.Level
	cursor       int
	scrollOffset int

	genres  []string
	artists []string
	tracks  []musiclib.Track

	selectedGenre  string
	selectedArtist string

	// Saved cursors/scroll for each level (restored on back).
	genreCursor        int
	genreScrollOffset  int
	artistCursor       int
	artistScrollOffset int

	width  int
	height int

	filter         filter.State
	sortState      SortState
	selectedHashes map[string]struct{}
}

func (v *GenresView) setSize(w, h int) { v.width = w; v.height = h }

func (v GenresView) recomputeGenresSort() GenresView {
	if v.sortState.Field == SortDefault {
		return v
	}
	switch v.level {
	case gentui.GenreLevel:
		if len(v.genres) > 0 {
			v.genres = SortStrings(v.genres, v.sortState.Dir)
		}
	case gentui.ArtistLevel:
		if len(v.artists) > 0 {
			v.artists = SortStrings(v.artists, v.sortState.Dir)
		}
	case gentui.TrackLevel:
		if sorted := SortTracks(v.tracks, v.sortState.Field, v.sortState.Dir); sorted != nil {
			v.tracks = sorted
		}
	}
	return v
}

func (v GenresView) render(s styles.Styles, favHashes map[string]struct{}) string {
	var sortInd string
	if v.sortState.Field != SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", SortFieldLabel(v.sortState.Field), SortDirIndicator(v.sortState.Dir))
	}
	gv := gentui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Genres:             v.genres,
		Artists:            v.artists,
		Tracks:             v.tracks,
		SelectedGenre:      v.selectedGenre,
		SelectedArtist:     v.selectedArtist,
		GenreCursor:        v.genreCursor,
		GenreScrollOffset:  v.genreScrollOffset,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		Width:              v.width,
		Height:             v.height,
		Styles:             s,
		FilterActive:       v.filter.Active,
		FilterText:         v.filter.Text,
		SortIndicator:      sortInd,
		FavoriteHashes:     favHashes,
		SelectedHashes:     v.selectedHashes,
	}
	return gv.Render()
}

// HandleAction executes a pre-resolved genres action dispatched by root.Update.
func (v GenresView) HandleAction(a action) GenresView {
	var sortInd string
	if v.sortState.Field != SortDefault {
		sortInd = "active" // non-empty triggers reserve in visibleRows
	}
	view := gentui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Genres:             v.genres,
		Artists:            v.artists,
		Tracks:             v.tracks,
		GenreCursor:        v.genreCursor,
		GenreScrollOffset:  v.genreScrollOffset,
		ArtistCursor:       v.artistCursor,
		ArtistScrollOffset: v.artistScrollOffset,
		Height:             v.height,
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

// openArtists transitions the view to ArtistLevel with the given artist list.
// The current genre cursor and scroll offset are saved for restoration by back.
func (v GenresView) openArtists(artists []string) GenresView {
	view := gentui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenArtists(artists)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.genreCursor = view.GenreCursor
	v.genreScrollOffset = view.GenreScrollOffset
	v.artists = view.Artists
	v.filter = filter.State{}
	v.sortState = SortState{}
	v.selectedHashes = nil
	return v
}

// openTracks transitions the view to TrackLevel with the given track list.
// The current artist cursor and scroll offset are saved for restoration by back.
func (v GenresView) openTracks(tracks []musiclib.Track) GenresView {
	view := gentui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.artistCursor = view.ArtistCursor
	v.artistScrollOffset = view.ArtistScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = SortState{}
	v.selectedHashes = nil
	return v
}

// back navigates one level up. At ArtistLevel it returns to GenreLevel;
// at TrackLevel it returns to ArtistLevel (or GenreLevel if artists were
// skipped). At GenreLevel it is a no-op.
func (v GenresView) back() GenresView {
	view := gentui.View{
		Level:              v.level,
		Cursor:             v.cursor,
		ScrollOffset:       v.scrollOffset,
		Genres:             v.genres,
		Artists:            v.artists,
		Tracks:             v.tracks,
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
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.artists = view.Artists
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = SortState{}
	v.selectedHashes = nil
	return v
}

// clampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list.
func (v GenresView) clampCursor() GenresView {
	var n int
	switch v.level {
	case gentui.ArtistLevel:
		n = len(arttui.FilterStrings(v.artists, v.filter.Text))
	case gentui.TrackLevel:
		n = len(arttui.FilterTracks(v.tracks, v.filter.Text))
	default:
		n = len(arttui.FilterStrings(v.genres, v.filter.Text))
	}
	if n == 0 {
		v.cursor = 0
		v.scrollOffset = 0
	} else if v.cursor >= n {
		v.cursor = n - 1
	}
	if v.scrollOffset > v.cursor {
		v.scrollOffset = v.cursor
	}
	return v
}

// focusedGenre returns the genre name at the current cursor position and true.
func (v GenresView) focusedGenre() (string, bool) {
	if v.level != gentui.GenreLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.genres, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedArtist returns the artist name at the current cursor position and true.
func (v GenresView) focusedArtist() (string, bool) {
	if v.level != gentui.ArtistLevel {
		return "", false
	}
	items := arttui.FilterStrings(v.artists, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return "", false
	}
	return items[v.cursor], true
}

// focusedTrack returns the track at the current cursor position and true.
func (v GenresView) focusedTrack() (musiclib.Track, bool) {
	if v.level != gentui.TrackLevel {
		return musiclib.Track{}, false
	}
	items := arttui.FilterTracks(v.tracks, v.filter.Text)
	if v.cursor < 0 || v.cursor >= len(items) {
		return musiclib.Track{}, false
	}
	return items[v.cursor], true
}

// enqueueFocusedTrack returns a tea.Cmd that appends the focused track to the
// play queue via qa. Returns nil when the view is not at TrackLevel or no
// track is focused.
func (v GenresView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
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
