package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/browse"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/palette"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

type selectSortFieldMsg struct{ field sorting.SortField }

func (m Model) activeSortFields() ([]sorting.SortField, sorting.SortState) {
	switch m.ActiveView {
	case viewLibrary:
		return librarySortFields(m.library.Mode), m.library.SortState
	case viewQueue:
		return availableSortFields(!m.queue.SavingAsPlaylist, sorting.QueueSortFields, m.queue.SortState)
	case viewPlaylists:
		return m.playlistSortFields(), m.playlists.SortState
	case viewBrowse:
		return m.browseSortFields()
	case viewFavorites:
		return favoritesSortFieldsForLevel(m.favorites), m.favorites.SortState
	case viewTags:
		return tagsSortFieldsForLevel(m.tagsView.Level), m.tagsView.SortState
	case viewMixes:
		return availableSortFields(m.mixes.Level == 1, sorting.LibrarySortFields, m.mixes.TrackView.SortState)
	case viewInsights:
		return availableSortFields(m.insights.AtTracks(), sorting.LibrarySortFields, m.insights.TrackView.SortState)
	}
	return nil, sorting.SortState{}
}

func availableSortFields(available bool, fields []sorting.SortField, state sorting.SortState) ([]sorting.SortField, sorting.SortState) {
	if !available {
		return nil, state
	}
	return fields, state
}

func librarySortFields(mode libtui.LibraryMode) []sorting.SortField {
	switch mode {
	case libtui.LibraryModeRecent:
		return sorting.RecentlyAddedSortFields
	case libtui.LibraryModeHistory:
		return sorting.HistorySortFields
	default:
		return sorting.LibrarySortFields
	}
}

func (m Model) playlistSortFields() []sorting.SortField {
	if m.playlists.Creating || m.playlists.Renaming {
		return nil
	}
	if m.playlists.Level == pltui.TrackLevel {
		return sorting.PlaylistTrackSortFields
	}
	return sorting.PlaylistListSortFields
}

func (m Model) browseSortFields() ([]sorting.SortField, sorting.SortState) {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		return artistsSortFieldsForLevel(m.browse.Artists.Level), m.browse.Artists.SortState
	case browse.BrowseModeAlbums:
		return albumsSortFieldsForLevel(m.browse.Albums.Level), m.browse.Albums.SortState
	case browse.BrowseModeGenres:
		return genresSortFieldsForLevel(m.browse.Genres.Level), m.browse.Genres.SortState
	}
	return nil, sorting.SortState{}
}

func (m Model) sortPaletteCommands() []palette.PaletteCommand {
	fields, state := m.activeSortFields()
	commands := make([]palette.PaletteCommand, 0, len(fields))
	for _, field := range fields {
		name := "Sort by " + sorting.SortFieldLabel(field)
		if field == sorting.SortDefault {
			name = "Sort by default order"
		}
		command := paletteMessageCmd(fmt.Sprintf("sort.field.%d", field), "sort", name, "", selectSortFieldMsg{field})
		if field == state.Field {
			command.CurrentState = "Selected"
			if field != sorting.SortDefault {
				command.CurrentState += " " + sorting.SortDirIndicator(state.Dir)
			}
		}
		commands = append(commands, command)
	}
	return commands
}

func (m Model) selectSortField(field sorting.SortField) (tea.Model, tea.Cmd) {
	fields, state := m.activeSortFields()
	if !slices.Contains(fields, field) || state.Field == field {
		return m, nil
	}
	m = exitVisualMode(m)
	next, cmd, _ := m.applySortField(field)
	return next, cmd
}
