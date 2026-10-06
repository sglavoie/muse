package favorites

import (
	"fmt"
	"maps"

	tea "charm.land/bubbletea/v2"

	musiclib "github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// FavoritesView wraps the favorites sub-panel state for the root model.
// It holds the navigation level, cursor position, category counts, and
// favorite item lists. Rendering is delegated to View.
type FavoritesView struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Category Category

	// Category counts (shown at CategoryLevel).
	ArtistCount int
	AlbumCount  int
	TrackCount  int

	// List items (populated at ListLevel).
	Artists []string
	Albums  []string
	Tracks  []musiclib.Track

	// Drill-down tracks (populated at TrackLevel from a favorite artist/album).
	DrilldownTracks []musiclib.Track
	DrilldownName   string // heading text for TrackLevel (artist or album name)

	// Saved cursors/scroll for each level (restored on Back).
	CategoryCursor   int
	listCursor       int
	listScrollOffset int

	Width  int
	Height int

	Filter        filter.State
	SortState     sorting.SortState
	SelectedItems map[string]struct{}
}

func (v *FavoritesView) SetSize(w, h int) { v.Width = w; v.Height = h }

func (v FavoritesView) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	var sortInd string
	if v.SortState.Field != sorting.SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", sorting.SortFieldLabel(v.SortState.Field), sorting.SortDirIndicator(v.SortState.Dir))
	}
	fv := View{
		Level:            v.Level,
		Cursor:           v.Cursor,
		ScrollOffset:     v.ScrollOffset,
		Category:         v.Category,
		ArtistCount:      v.ArtistCount,
		AlbumCount:       v.AlbumCount,
		TrackCount:       v.TrackCount,
		Artists:          v.Artists,
		Albums:           v.Albums,
		Tracks:           v.Tracks,
		DrilldownTracks:  v.DrilldownTracks,
		DrilldownName:    v.DrilldownName,
		CategoryCursor:   v.CategoryCursor,
		ListCursor:       v.listCursor,
		ListScrollOffset: v.listScrollOffset,
		Width:            v.Width,
		Height:           v.Height,
		Styles:           s,
		FilterActive:     v.Filter.Active,
		FilterText:       v.Filter.Text,
		FilterView:       v.Filter.View(),
		FilterHint: func() string {
			if v.Level == TrackLevel || (v.Level == ListLevel && v.Category == CategoryTracks) {
				return "a: artist  t: title  l: album  g: genre  aa: album artist"
			}
			return ""
		}(),
		SortIndicator:   sortInd,
		SelectedItems:   v.SelectedItems,
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
	if v.SortState.Field != sorting.SortDefault {
		sortInd = "active" // non-empty triggers reserve in visibleRows
	}
	view := View{
		Level:           v.Level,
		Cursor:          v.Cursor,
		ScrollOffset:    v.ScrollOffset,
		Category:        v.Category,
		Artists:         v.Artists,
		Albums:          v.Albums,
		Tracks:          v.Tracks,
		DrilldownTracks: v.DrilldownTracks,
		DrilldownName:   v.DrilldownName,
		Height:          v.Height,
		FilterActive:    v.Filter.Active,
		FilterText:      v.Filter.Text,
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
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	return v
}

// Back navigates one level up.
func (v FavoritesView) Back() FavoritesView {
	view := View{
		Level:            v.Level,
		Cursor:           v.Cursor,
		ScrollOffset:     v.ScrollOffset,
		Category:         v.Category,
		Artists:          v.Artists,
		Albums:           v.Albums,
		Tracks:           v.Tracks,
		DrilldownTracks:  v.DrilldownTracks,
		CategoryCursor:   v.CategoryCursor,
		ListCursor:       v.listCursor,
		ListScrollOffset: v.listScrollOffset,
	}
	view = view.Back()
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.Artists = view.Artists
	v.Albums = view.Albums
	v.Tracks = view.Tracks
	v.DrilldownTracks = view.DrilldownTracks
	v.DrilldownName = ""
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedItems = nil
	return v
}

// OpenTracks transitions from ListLevel to TrackLevel with the given tracks.
// name is the resolved display name for the heading (e.g. the artist or album
// name that was selected, correctly resolved from the possibly-filtered list).
func (v FavoritesView) OpenTracks(tracks []musiclib.Track, name string) FavoritesView {
	view := View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
	}
	view = view.OpenTracks(tracks)
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.listCursor = view.ListCursor
	v.listScrollOffset = view.ListScrollOffset
	v.DrilldownTracks = view.DrilldownTracks
	v.DrilldownName = name
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SelectedItems = nil
	return v
}

// FocusedArtist returns the artist name at the current cursor position and true.
// When a filter is active it indexes into the filtered list.
func (v FavoritesView) FocusedArtist() (string, bool) {
	if v.Level != ListLevel || v.Category != CategoryArtists {
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
func (v FavoritesView) FocusedAlbum() (string, bool) {
	if v.Level != ListLevel || v.Category != CategoryAlbums {
		return "", false
	}
	items := arttui.FilterStrings(v.Albums, v.Filter.Text)
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return "", false
	}
	return items[v.Cursor], true
}

// FocusedTrack returns the track at the current cursor position and true.
func (v FavoritesView) FocusedTrack() (musiclib.Track, bool) {
	var tracks []musiclib.Track
	switch {
	case v.Level == TrackLevel:
		tracks = arttui.FilterTracks(v.DrilldownTracks, v.Filter.Text)
	case v.Level == ListLevel && v.Category == CategoryTracks:
		tracks = arttui.FilterTracks(v.Tracks, v.Filter.Text)
	default:
		return musiclib.Track{}, false
	}
	if v.Cursor < 0 || v.Cursor >= len(tracks) {
		return musiclib.Track{}, false
	}
	return tracks[v.Cursor], true
}

func (v FavoritesView) EnqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

func (v FavoritesView) PlayNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}

// RecomputeFavoritesSort re-sorts the owned slices for the current level.
func (v FavoritesView) RecomputeFavoritesSort() FavoritesView {
	if v.SortState.Field == sorting.SortDefault {
		return v
	}
	switch v.Level {
	case ListLevel:
		return v.recomputeListSort()
	case TrackLevel:
		if sorted := sorting.SortTracks(v.DrilldownTracks, v.SortState.Field, v.SortState.Dir); sorted != nil {
			v.DrilldownTracks = sorted
		}
	}
	return v
}

// recomputeListSort re-sorts whichever slice backs the current list category.
func (v FavoritesView) recomputeListSort() FavoritesView {
	switch v.Category {
	case CategoryArtists:
		if len(v.Artists) > 0 {
			v.Artists = sorting.SortStrings(v.Artists, v.SortState.Dir)
		}
	case CategoryAlbums:
		if len(v.Albums) > 0 {
			v.Albums = sorting.SortStrings(v.Albums, v.SortState.Dir)
		}
	case CategoryTracks:
		if sorted := sorting.SortTracks(v.Tracks, v.SortState.Field, v.SortState.Dir); sorted != nil {
			v.Tracks = sorted
		}
	}
	return v
}

// ClampCursor ensures the cursor is within the bounds of the current level's
// effective (filtered) list. When a filter is active, the cursor indexes into
// the filtered results, so the count must reflect the filtered length rather
// than the raw backing-store length.
func (v FavoritesView) ClampCursor() FavoritesView {
	var n int
	switch v.Level {
	case CategoryLevel:
		n = 3
	case ListLevel:
		switch v.Category {
		case CategoryArtists:
			n = len(arttui.FilterStrings(v.Artists, v.Filter.Text))
		case CategoryAlbums:
			n = len(arttui.FilterStrings(v.Albums, v.Filter.Text))
		case CategoryTracks:
			n = len(arttui.FilterTracks(v.Tracks, v.Filter.Text))
		}
	case TrackLevel:
		n = len(arttui.FilterTracks(v.DrilldownTracks, v.Filter.Text))
	}
	v.Cursor, v.ScrollOffset = sorting.ClampCursorToCount(v.Cursor, v.ScrollOffset, n)
	return v
}

// HasSelection reports whether any items are bulk-selected.
func (v FavoritesView) HasSelection() bool {
	return len(v.SelectedItems) > 0
}

// ClearSelection removes all bulk-selection markers.
func (v FavoritesView) ClearSelection() FavoritesView {
	v.SelectedItems = nil
	return v
}

// ToggleSelection toggles the stable key at the given visible row.
func (v FavoritesView) ToggleSelection(idx int) FavoritesView {
	keys := v.SelectionKeys()
	if idx < 0 || idx >= len(keys) || keys[idx] == "" {
		return v
	}
	v.SelectedItems = maps.Clone(v.SelectedItems)
	if v.SelectedItems == nil {
		v.SelectedItems = make(map[string]struct{})
	}
	key := keys[idx]
	if _, ok := v.SelectedItems[key]; ok {
		delete(v.SelectedItems, key)
	} else {
		v.SelectedItems[key] = struct{}{}
	}
	return v
}

// SelectionKeys returns visible identities in display order. Selections are
// scoped to the current level and cleared when navigating into another list.
func (v FavoritesView) SelectionKeys() []string {
	switch {
	case v.Level == CategoryLevel:
		return nil
	case v.Level == ListLevel && v.Category == CategoryArtists:
		return arttui.FilterStrings(v.Artists, v.Filter.Text)
	case v.Level == ListLevel && v.Category == CategoryAlbums:
		return arttui.FilterStrings(v.Albums, v.Filter.Text)
	}
	tracks := v.Tracks
	if v.Level == TrackLevel {
		tracks = v.DrilldownTracks
	}
	tracks = arttui.FilterTracks(tracks, v.Filter.Text)
	keys := make([]string, len(tracks))
	for i, track := range tracks {
		keys[i] = trackSelectionKey(track)
	}
	return keys
}

func trackSelectionKey(track musiclib.Track) string {
	if track.Hash != "" {
		return track.Hash
	}
	return track.Path
}

// SelectedTracks includes hidden selections, following the unfiltered sort order.
func (v FavoritesView) SelectedTracks() []musiclib.Track {
	var tracks []musiclib.Track
	switch {
	case v.Level == TrackLevel:
		tracks = v.DrilldownTracks
	case v.Level == ListLevel && v.Category == CategoryTracks:
		tracks = v.Tracks
	default:
		return nil
	}
	var selected []musiclib.Track
	for _, track := range tracks {
		if _, ok := v.SelectedItems[trackSelectionKey(track)]; ok {
			selected = append(selected, track)
		}
	}
	return selected
}
