package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TagsView wraps the tags sub-panel state for the root model.
// It holds the navigation level, cursor position, tag list, tag counts, and
// (at TrackLevel) the tracks of the selected tag. Rendering is delegated to
// the tags sub-package.
type TagsView struct {
	level        tagtui.Level
	cursor       int
	scrollOffset int

	tags        []db.TagRow
	tagCounts   map[int64]int         // tag ID → track count; loaded on switch or mutation
	tracks      []db.PlaylistTrackRow // populated when level == TrackLevel
	selectedTag db.TagRow

	// Saved cursors/scroll for tag level (restored on back).
	tagCursor       int
	tagScrollOffset int

	width  int
	height int

	filter           filter.State
	sortState        sorting.SortState
	sortedTrackOrder []int // permutation for sorted track display; nil = natural order
	selectedHashes   map[string]struct{}

	// Inline create/rename input state.
	creating    bool
	createInput textinput.Model
	renaming    bool
	renameInput textinput.Model
}

func (v *TagsView) setSize(w, h int) { v.width = w; v.height = h }

func (v TagsView) render(s styles.Styles, favHashes map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	var sortInd string
	if v.sortState.Field != sorting.SortDefault {
		sortInd = fmt.Sprintf("Sort: %s %s", sorting.SortFieldLabel(v.sortState.Field), sorting.SortDirIndicator(v.sortState.Dir))
	}

	var createInputStr, renameInputStr string
	if v.creating {
		createInputStr = v.createInput.Value()
	}
	if v.renaming {
		renameInputStr = v.renameInput.Value()
	}

	tracks := v.tracks
	if v.sortedTrackOrder != nil && len(v.sortedTrackOrder) == len(v.tracks) {
		// Guard against stale sortedTrackOrder; see view_playlists.go for rationale.
		reordered := make([]db.PlaylistTrackRow, len(v.tracks))
		safe := true
		for i, idx := range v.sortedTrackOrder {
			if idx < 0 || idx >= len(v.tracks) {
				safe = false
				break
			}
			reordered[i] = v.tracks[idx]
		}
		if safe {
			tracks = reordered
		}
	}

	tv := tagtui.View{
		Level:           v.level,
		Cursor:          v.cursor,
		ScrollOffset:    v.scrollOffset,
		Tags:            v.tags,
		TagCounts:       v.tagCounts,
		Tracks:          tracks,
		SelectedTag:     v.selectedTag,
		TagCursor:       v.tagCursor,
		TagScrollOffset: v.tagScrollOffset,
		Width:           v.width,
		Height:          v.height,
		Styles:          s,
		FilterActive:    v.filter.Active,
		FilterText:      v.filter.Text,
		FilterView:      v.filter.View(),
		SortIndicator:   sortInd,
		FavoriteHashes:  favHashes,
		SelectedHashes:  v.selectedHashes,
		NowPlayingHash:  nowPlayingHash,
		NowPlayingIcon:  nowPlayingIcon,
		Creating:        v.creating,
		CreateInput:     createInputStr,
		Renaming:        v.renaming,
		RenameInput:     renameInputStr,
	}
	return tv.Render()
}

// HandleAction executes a pre-resolved tags action dispatched by root.Update.
func (v TagsView) HandleAction(a action) TagsView {
	var sortInd string
	if v.sortState.Field != sorting.SortDefault {
		sortInd = "active"
	}
	view := tagtui.View{
		Level:         v.level,
		Cursor:        v.cursor,
		ScrollOffset:  v.scrollOffset,
		Tags:          v.tags,
		TagCounts:     v.tagCounts,
		Tracks:        v.tracks,
		Height:        v.height,
		FilterActive:  v.filter.Active,
		FilterText:    v.filter.Text,
		SortIndicator: sortInd,
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
// The current tag cursor and scroll offset are saved for restoration by back.
func (v TagsView) openTracks(tag db.TagRow, tracks []db.PlaylistTrackRow) TagsView {
	view := tagtui.View{
		Level:        v.level,
		Cursor:       v.cursor,
		ScrollOffset: v.scrollOffset,
		Tags:         v.tags,
		TagCounts:    v.tagCounts,
		FilterText:   v.filter.Text,
	}
	view = view.OpenTracks(tag, tracks)
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.tagCursor = view.TagCursor
	v.tagScrollOffset = view.TagScrollOffset
	v.selectedTag = view.SelectedTag
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = sorting.SortState{}
	v.sortedTrackOrder = nil
	v.selectedHashes = nil
	return v
}

// back navigates one level up to TagLevel.
func (v TagsView) back() TagsView {
	view := tagtui.View{
		Level:           v.level,
		Cursor:          v.cursor,
		ScrollOffset:    v.scrollOffset,
		Tags:            v.tags,
		TagCounts:       v.tagCounts,
		TagCursor:       v.tagCursor,
		TagScrollOffset: v.tagScrollOffset,
		Tracks:          v.tracks,
		Height:          v.height,
	}
	view = view.Back()
	v.level = view.Level
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	v.tracks = view.Tracks
	v.filter = filter.State{}
	v.sortState = sorting.SortState{}
	v.sortedTrackOrder = nil
	v.selectedHashes = nil
	return v
}

// clampCursor ensures the cursor is within the bounds of the effective list.
func (v TagsView) clampCursor() TagsView {
	var n int
	if v.level == tagtui.TrackLevel {
		n = len(v.displayTracks())
	} else {
		filtered := tagtui.View{Tags: v.tags, TagCounts: v.tagCounts, FilterText: v.filter.Text}.EffectiveTags()
		n = len(filtered)
	}
	v.cursor, v.scrollOffset = sorting.ClampCursorToCount(v.cursor, v.scrollOffset, n)
	return v
}

// recomputeTagsSort rebuilds the sorted track order cache.
func (v TagsView) recomputeTagsSort() TagsView {
	if v.level == tagtui.TrackLevel {
		v.sortedTrackOrder = sorting.SortPlaylistTrackIndices(v.tracks, v.sortState.Field, v.sortState.Dir)
	} else {
		v.sortedTrackOrder = nil
	}
	return v
}

// focusedTag returns the tag at the current cursor position and true.
// Returns a zero value and false when not at TagLevel or list is empty.
func (v TagsView) focusedTag() (db.TagRow, bool) {
	if v.level != tagtui.TagLevel {
		return db.TagRow{}, false
	}
	items := tagtui.View{Tags: v.tags, TagCounts: v.tagCounts, FilterText: v.filter.Text}.EffectiveTags()
	if v.cursor < 0 || v.cursor >= len(items) {
		return db.TagRow{}, false
	}
	return items[v.cursor], true
}

// displayTracks returns tracks in display order (sorted then filtered).
func (v TagsView) displayTracks() []db.PlaylistTrackRow {
	tracks := v.tracks
	if v.sortedTrackOrder != nil && len(v.sortedTrackOrder) == len(v.tracks) {
		// Guard against stale sortedTrackOrder; see view_playlists.go for rationale.
		reordered := make([]db.PlaylistTrackRow, len(v.tracks))
		safe := true
		for i, idx := range v.sortedTrackOrder {
			if idx < 0 || idx >= len(v.tracks) {
				safe = false
				break
			}
			reordered[i] = v.tracks[idx]
		}
		if safe {
			tracks = reordered
		}
	}
	return tagtui.View{Tracks: tracks, FilterText: v.filter.Text}.EffectiveTracks()
}

// focusedTrack returns the track at the current cursor position and true.
// Returns false when not at TrackLevel or list is empty.
func (v TagsView) focusedTrack() (db.PlaylistTrackRow, bool) {
	if v.level != tagtui.TrackLevel {
		return db.PlaylistTrackRow{}, false
	}
	items := v.displayTracks()
	if v.cursor < 0 || v.cursor >= len(items) {
		return db.PlaylistTrackRow{}, false
	}
	return items[v.cursor], true
}

// focusedTrackAsLibTrack returns the focused track converted to musiclib.Track.
func (v TagsView) focusedTrackAsLibTrack() (musiclib.Track, bool) {
	t, ok := v.focusedTrack()
	if !ok {
		return musiclib.Track{}, false
	}
	return musiclib.Track{
		Hash:     t.Hash,
		Path:     t.Path,
		Artist:   t.Artist,
		Title:    t.Title,
		Album:    t.Album,
		Duration: float64(t.DurationMs) / 1000,
	}, true
}

func (v TagsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrackAsLibTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

func (v TagsView) playNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.focusedTrackAsLibTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}

// isCreating reports whether the inline create input is active.
func (v TagsView) isCreating() bool { return v.creating }

// startCreate activates the inline create input with an empty buffer.
func (v TagsView) startCreate() TagsView {
	ti := textinput.New()
	ti.Focus()
	v.creating = true
	v.createInput = ti
	return v
}

// cancelCreate dismisses the inline create input without persisting.
func (v TagsView) cancelCreate() TagsView {
	v.creating = false
	v.createInput = textinput.Model{}
	return v
}

// createInputValue returns the trimmed create input buffer.
func (v TagsView) createInputValue() string {
	return strings.TrimSpace(v.createInput.Value())
}

// updateCreateInput forwards a key message to the Bubbles textinput model.
func (v TagsView) updateCreateInput(msg tea.KeyPressMsg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// updateCreateInputMsg forwards a generic tea.Msg to the textinput.
func (v TagsView) updateCreateInputMsg(msg tea.Msg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// isRenaming reports whether the inline rename input is active.
func (v TagsView) isRenaming() bool { return v.renaming }

// startRename activates the inline rename input pre-populated with currentName.
func (v TagsView) startRename(currentName string) TagsView {
	ti := textinput.New()
	ti.SetValue(currentName)
	ti.CursorEnd()
	ti.Focus()
	v.renaming = true
	v.renameInput = ti
	return v
}

// cancelRename dismisses the inline rename input without persisting.
func (v TagsView) cancelRename() TagsView {
	v.renaming = false
	v.renameInput = textinput.Model{}
	return v
}

// renameInputValue returns the trimmed rename input buffer.
func (v TagsView) renameInputValue() string {
	return strings.TrimSpace(v.renameInput.Value())
}

// updateRenameInput forwards a key message to the Bubbles textinput model.
func (v TagsView) updateRenameInput(msg tea.KeyPressMsg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.renameInput, cmd = v.renameInput.Update(msg)
	return v, cmd
}

// updateRenameInputMsg forwards a generic tea.Msg to the textinput.
func (v TagsView) updateRenameInputMsg(msg tea.Msg) (TagsView, tea.Cmd) {
	var cmd tea.Cmd
	v.renameInput, cmd = v.renameInput.Update(msg)
	return v, cmd
}
