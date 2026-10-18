package tags

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TagsView wraps the tags sub-panel state for the root model.
// It holds the navigation level, cursor position, tag list, tag counts, and
// (at TrackLevel) the tracks of the selected tag. Rendering is delegated to
// View.
type TagsView struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Tags        []db.TagRow
	TagCounts   map[int64]int         // tag ID → track count; loaded on switch or mutation
	Tracks      []db.PlaylistTrackRow // populated when Level == TrackLevel
	SelectedTag db.TagRow

	// Saved cursors/scroll for tag level (restored on Back).
	TagCursor       int
	TagScrollOffset int

	Width  int
	Height int

	Filter           filter.State
	SortState        sorting.SortState
	SortedTrackOrder []int // permutation for sorted track display; nil = natural order
	SelectedHashes   map[string]struct{}

	// Inline create/rename input state.
	Creating    bool
	CreateInput textinput.Model
	Renaming    bool
	RenameInput textinput.Model
}

func (v *TagsView) SetSize(w, h int) { v.Width = w; v.Height = h }

func (v TagsView) Render(s styles.Styles, favHashes map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	var sortInd string
	if v.SortState.Field != sorting.SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", sorting.SortFieldLabel(v.SortState.Field), sorting.SortDirIndicator(v.SortState.Dir))
	}

	var createInputStr, renameInputStr string
	if v.Creating {
		createInputStr = v.CreateInput.Value()
	}
	if v.Renaming {
		renameInputStr = v.RenameInput.Value()
	}

	tracks := v.Tracks
	if v.SortedTrackOrder != nil && len(v.SortedTrackOrder) == len(v.Tracks) {
		// Guard against stale SortedTrackOrder; see playlists/view_playlists.go for rationale.
		reordered := make([]db.PlaylistTrackRow, len(v.Tracks))
		safe := true
		for i, idx := range v.SortedTrackOrder {
			if idx < 0 || idx >= len(v.Tracks) {
				safe = false
				break
			}
			reordered[i] = v.Tracks[idx]
		}
		if safe {
			tracks = reordered
		}
	}

	tv := View{
		Level:           v.Level,
		Cursor:          v.Cursor,
		ScrollOffset:    v.ScrollOffset,
		Tags:            v.Tags,
		TagCounts:       v.TagCounts,
		Tracks:          tracks,
		SelectedTag:     v.SelectedTag,
		TagCursor:       v.TagCursor,
		TagScrollOffset: v.TagScrollOffset,
		Width:           v.Width,
		Height:          v.Height,
		Styles:          s,
		FilterActive:    v.Filter.Active,
		FilterText:      v.Filter.Text,
		FilterView:      v.Filter.View(),
		SortIndicator:   sortInd,
		FavoriteHashes:  favHashes,
		SelectedHashes:  v.SelectedHashes,
		NowPlayingHash:  nowPlayingHash,
		NowPlayingIcon:  nowPlayingIcon,
		Creating:        v.Creating,
		CreateInput:     createInputStr,
		Renaming:        v.Renaming,
		RenameInput:     renameInputStr,
	}
	return tv.Render()
}

// HandleAction executes a pre-resolved tags action dispatched by root.Update.
func (v TagsView) HandleAction(a keys.Action) TagsView {
	var sortInd string
	if v.SortState.Field != sorting.SortDefault {
		sortInd = "active"
	}
	view := View{
		Level:         v.Level,
		Cursor:        v.Cursor,
		ScrollOffset:  v.ScrollOffset,
		Tags:          v.Tags,
		TagCounts:     v.TagCounts,
		Tracks:        v.Tracks,
		Height:        v.Height,
		FilterActive:  v.Filter.Active,
		FilterText:    v.Filter.Text,
		SortIndicator: sortInd,
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

// OpenTracks transitions the view to TrackLevel with the given track list.
// The current tag cursor and scroll offset are saved for restoration by Back.
func (v TagsView) OpenTracks(tag db.TagRow, tracks []db.PlaylistTrackRow) TagsView {
	view := View{
		Level:        v.Level,
		Cursor:       v.Cursor,
		ScrollOffset: v.ScrollOffset,
		Tags:         v.Tags,
		TagCounts:    v.TagCounts,
		FilterText:   v.Filter.Text,
	}
	view = view.OpenTracks(tag, tracks)
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.TagCursor = view.TagCursor
	v.TagScrollOffset = view.TagScrollOffset
	v.SelectedTag = view.SelectedTag
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SortedTrackOrder = nil
	v.SelectedHashes = nil
	return v
}

// Back navigates one level up to TagLevel.
func (v TagsView) Back() TagsView {
	view := View{
		Level:           v.Level,
		Cursor:          v.Cursor,
		ScrollOffset:    v.ScrollOffset,
		Tags:            v.Tags,
		TagCounts:       v.TagCounts,
		TagCursor:       v.TagCursor,
		TagScrollOffset: v.TagScrollOffset,
		Tracks:          v.Tracks,
		Height:          v.Height,
	}
	view = view.Back()
	v.Level = view.Level
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	v.Tracks = view.Tracks
	v.Filter = filter.State{}
	v.SortState = sorting.SortState{}
	v.SortedTrackOrder = nil
	v.SelectedHashes = nil
	return v
}

// ClampCursor ensures the cursor is within the bounds of the effective list.
func (v TagsView) ClampCursor() TagsView {
	var n int
	if v.Level == TrackLevel {
		n = len(v.DisplayTracks())
	} else {
		filtered := View{Tags: v.Tags, TagCounts: v.TagCounts, FilterText: v.Filter.Text}.EffectiveTags()
		n = len(filtered)
	}
	v.Cursor, v.ScrollOffset = sorting.ClampCursorToCount(v.Cursor, v.ScrollOffset, n)
	return v
}

// RecomputeTagsSort rebuilds the sorted track order cache.
func (v TagsView) RecomputeTagsSort() TagsView {
	if v.Level == TrackLevel {
		v.SortedTrackOrder = sorting.SortPlaylistTrackIndices(v.Tracks, v.SortState.Field, v.SortState.Dir)
	} else {
		v.SortedTrackOrder = nil
	}
	return v
}

// FocusedTag returns the tag at the current cursor position and true.
// Returns a zero value and false when not at TagLevel or list is empty.
func (v TagsView) FocusedTag() (db.TagRow, bool) {
	if v.Level != TagLevel {
		return db.TagRow{}, false
	}
	items := View{Tags: v.Tags, TagCounts: v.TagCounts, FilterText: v.Filter.Text}.EffectiveTags()
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return db.TagRow{}, false
	}
	return items[v.Cursor], true
}

// DisplayTracks returns tracks in display order (sorted then filtered).
func (v TagsView) DisplayTracks() []db.PlaylistTrackRow {
	tracks := v.Tracks
	if v.SortedTrackOrder != nil && len(v.SortedTrackOrder) == len(v.Tracks) {
		// Guard against stale SortedTrackOrder; see playlists/view_playlists.go for rationale.
		reordered := make([]db.PlaylistTrackRow, len(v.Tracks))
		safe := true
		for i, idx := range v.SortedTrackOrder {
			if idx < 0 || idx >= len(v.Tracks) {
				safe = false
				break
			}
			reordered[i] = v.Tracks[idx]
		}
		if safe {
			tracks = reordered
		}
	}
	return View{Tracks: tracks, FilterText: v.Filter.Text}.EffectiveTracks()
}

// FocusedTrack returns the track at the current cursor position and true.
// Returns false when not at TrackLevel or list is empty.
func (v TagsView) FocusedTrack() (db.PlaylistTrackRow, bool) {
	if v.Level != TrackLevel {
		return db.PlaylistTrackRow{}, false
	}
	items := v.DisplayTracks()
	if v.Cursor < 0 || v.Cursor >= len(items) {
		return db.PlaylistTrackRow{}, false
	}
	return items[v.Cursor], true
}

// FocusedTrackAsLibTrack returns the focused track converted to musiclib.Track.
func (v TagsView) FocusedTrackAsLibTrack() (musiclib.Track, bool) {
	t, ok := v.FocusedTrack()
	if !ok {
		return musiclib.Track{}, false
	}
	return musiclib.Track{
		Hash:        t.Hash,
		Path:        t.Path,
		Artist:      t.Artist,
		Title:       t.Title,
		Album:       t.Album,
		AlbumArtist: t.AlbumArtist,
		Duration:    float64(t.DurationMs) / 1000,
	}, true
}

func (v TagsView) EnqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.FocusedTrackAsLibTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

// IsCreating reports whether the inline create input is active.
func (v TagsView) IsCreating() bool { return v.Creating }

// StartCreate activates the inline create input with an empty buffer.
func (v TagsView) StartCreate() TagsView {
	ti := textinput.New()
	ti.Focus()
	v.Creating = true
	v.CreateInput = ti
	return v
}

// CancelCreate dismisses the inline create input without persisting.
func (v TagsView) CancelCreate() TagsView {
	v.Creating = false
	v.CreateInput = textinput.Model{}
	return v
}

// CreateInputValue returns the trimmed create input buffer.
func (v TagsView) CreateInputValue() string {
	return strings.TrimSpace(v.CreateInput.Value())
}

// UpdateCreateInput forwards a key message to the Bubbles textinput model.
func (v TagsView) UpdateCreateInput(msg tea.KeyPressMsg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.CreateInput, cmd = v.CreateInput.Update(msg)
	return v, cmd
}

// UpdateCreateInputMsg forwards a generic tea.Msg to the textinput.
func (v TagsView) UpdateCreateInputMsg(msg tea.Msg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.CreateInput, cmd = v.CreateInput.Update(msg)
	return v, cmd
}

// IsRenaming reports whether the inline rename input is active.
func (v TagsView) IsRenaming() bool { return v.Renaming }

// StartRename activates the inline rename input pre-populated with currentName.
func (v TagsView) StartRename(currentName string) TagsView {
	ti := textinput.New()
	ti.SetValue(currentName)
	ti.CursorEnd()
	ti.Focus()
	v.Renaming = true
	v.RenameInput = ti
	return v
}

// CancelRename dismisses the inline rename input without persisting.
func (v TagsView) CancelRename() TagsView {
	v.Renaming = false
	v.RenameInput = textinput.Model{}
	return v
}

// RenameInputValue returns the trimmed rename input buffer.
func (v TagsView) RenameInputValue() string {
	return strings.TrimSpace(v.RenameInput.Value())
}

// UpdateRenameInput forwards a key message to the Bubbles textinput model.
func (v TagsView) UpdateRenameInput(msg tea.KeyPressMsg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.RenameInput, cmd = v.RenameInput.Update(msg)
	return v, cmd
}

// UpdateRenameInputMsg forwards a generic tea.Msg to the textinput.
func (v TagsView) UpdateRenameInputMsg(msg tea.Msg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.RenameInput, cmd = v.RenameInput.Update(msg)
	return v, cmd
}
