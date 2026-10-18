package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// Collection Enter still navigates and Queue Enter still plays. Only track
// lists use the shared selection-aware append action, including palette calls.
func (m Model) atEnqueueTrackLevel() bool {
	return m.ActiveView == viewLibrary || m.atCollectionTrackLevel() ||
		(m.ActiveView == viewInsights && m.insights.AtTracks()) ||
		(m.ActiveView == viewMixes && m.mixes.Level == 1)
}

func (m Model) handleSelectedEnqueueAction(action keys.Action) (Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionPlayNext, keys.ActionLibraryEnqueue, keys.ActionPlaylistOpen, keys.ActionArtistOpen,
		keys.ActionAlbumOpen, keys.ActionGenreOpen, keys.ActionFavoritesOpen,
		keys.ActionTagOpen, keys.ActionInsightsOpen:
	default:
		return m, nil, false
	}
	if !m.atEnqueueTrackLevel() || m.activeViewSelectionCount() == 0 {
		return m, nil, false
	}
	if m.batchEnqueuer == nil {
		m.statusBar.Notify("Enqueue not available")
		return m, nil, true
	}
	tracks := m.selectedEnqueueTracks()
	if len(tracks) == 0 {
		m.statusBar.Notify("No selected tracks available")
		return m, nil, true
	}
	source := ""
	if m.ActiveView == viewInsights {
		source = m.insights.DrillLabel
	}
	if m.ActiveView == viewPlaylists {
		_, source, _, _, _ = m.resolvePlaylistForEnqueue()
	}
	cmd := m.batchEnqueueTracksCmd(tracks, source, queueSourceName(source))
	if action == keys.ActionPlayNext {
		appendCmd := cmd
		cmd = func() tea.Msg {
			result := appendCmd().(BatchEnqueuedMsg)
			result.PlayNext = true
			return result
		}
	}
	m = exitVisualMode(m).clearActiveViewSelection()
	return m, cmd, true
}

// Read the unfiltered display order: hidden selections remain in scope and
// playlist row IDs preserve separate occurrences of the same song.
func (m Model) selectedEnqueueTracks() []library.Track {
	if m.ActiveView == viewPlaylists {
		view := m.playlists
		view.Filter = filter.State{}
		var rows []db.PlaylistTrackRow
		for _, row := range view.DisplayTracks() {
			if _, selected := view.SelectedTrackIDs[row.ID]; selected {
				rows = append(rows, row)
			}
		}
		return playlistRowsToLibTracks(rows)
	}
	if m.ActiveView == viewTags {
		view := m.tagsView
		view.Filter = filter.State{}
		return collectSelectedTracks(playlistRowsToLibTracks(view.DisplayTracks()), view.SelectedHashes)
	}
	return m.bulkActionTracks()
}
