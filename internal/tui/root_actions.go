package tui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/filter"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// helpState returns the sections and title to display in the help overlay,
// depending on whether the user has toggled view-specific filtering.
func (m Model) helpState() ([]helpSection, string) {
	if !m.helpFiltered {
		return helpSections(), "Keybindings  (? to filter by view)"
	}
	ctx := viewToContext(m.prevActiveView)
	name := viewName(m.prevActiveView)
	return filteredHelpSections(ctx), "Keybindings — " + name + "  (? to show all)"
}

// refreshHelpViewport rebuilds the help viewport content from the current
// help state and search filter, then resets the scroll position to the top.
func (m *Model) refreshHelpViewport() {
	sections, title := m.helpState()
	if m.helpSearchFilter.Text != "" {
		sections = searchHelpSections(sections, m.helpSearchFilter.Text)
	}
	m.helpViewport.SetContent(HelpOverlay{}.render(*m.styles, sections, title))
	m.helpViewport.SetYOffset(0)
	extraLines := 0
	if m.helpSearchFilter.Active || m.helpSearchFilter.Text != "" {
		extraLines = 1
	}
	m.helpViewport.SetHeight(max(m.termHeight-2-extraLines, 0))
}

// clearPendingConfirmations resets all two-press confirmation states.
func (m *Model) clearPendingConfirmations() {
	m.undo.clearQueuePending = false
	m.undo.bulkRemovePending = false
}

// switchToView transitions to the given view, clearing pending confirmations,
// resetting all sort states, and lazily deriving artist/album lists when needed.
func (m Model) switchToView(id viewID) Model {
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m = exitVisualMode(m)
	m.activeView = id
	if id == viewArtists && len(m.artists.artists) == 0 {
		m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	}
	if id == viewAlbums && len(m.albums.albums) == 0 {
		m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
	}
	if id == viewGenres && len(m.genres.genres) == 0 {
		m.genres.genres = gentui.DeriveGenres(m.library.index.Tracks())
	}
	if id == viewFavorites && m.favoriteReader != nil {
		artistCount, albumCount, trackCount, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = artistCount
			m.favorites.albumCount = albumCount
			m.favorites.trackCount = trackCount
		}
	}
	if id == viewHistory && m.historyReader != nil && len(m.history.baseTracks()) == 0 {
		tracks, _ := m.historyReader.ReadPlayHistory(500)
		m.history.sourceTracks = tracks
		m.history = m.history.recomputeSortedTracks()
	}
	if id == viewTags {
		// Sync global tag list into the tags view and load counts if needed.
		m = m.syncTagsViewTags()
		if m.tagsView.tagCounts == nil && m.tagReader != nil {
			counts, _ := m.tagReader.ReadTagCounts()
			m.tagsView.tagCounts = counts
		}
	}
	return m
}

// handleKeyAction dispatches a resolved KeyAction to the appropriate handler.
func (m Model) handleKeyAction(action action) (tea.Model, tea.Cmd) {
	if model, cmd, ok := m.handleSortAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handleMetadataKeyAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handlePlaybackKeyAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handleFavoriteKeyAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handleLibraryKeyAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handleQueueKeyAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handlePlaylistKeyAction(action); ok {
		return model, cmd
	}
	if model, cmd, ok := m.handleNavigationAction(action); ok {
		return model, cmd
	}

	switch action {
	case actionSettings:
		return m.handleSettingsOpen()
	case actionDebug:
		m.debugPrevView = m.activeView
		m.debugVisible = true
		m.debugViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-2, 0)))
		panel := collectDebugInfo(m)
		m.debugViewport.SetContent(panel.render(*m.styles))
		m.debugViewport.SetYOffset(0)
	case actionStats:
		m.statsPrevView = m.activeView
		m.statsVisible = true
		m.statsPeriod = 0
		m.statsViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-3, 0)))
		m.statsViewport.SetYOffset(0)
		return m, m.loadStatsCmd()
	case actionQueueSummary:
		if m.overlay.queueSummary != nil {
			m.overlay.queueSummary = nil
		} else {
			m.overlay.queueSummary = m.buildQueueSummary()
		}
	case actionHelp:
		m.prevActiveView = m.activeView
		m.helpVisible = true
		m.helpFiltered = false
		m.helpSearchFilter = filter.State{}
		m.helpViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(m.termHeight-2))
		m.refreshHelpViewport()
	case actionNextView:
		m = m.switchToView(nextView(m.activeView))
	case actionPrevView:
		m = m.switchToView(prevView(m.activeView))
	case actionQuit:
		m.overlay.nowPlaying = nil
		m.overlay.queueSummary = nil
		m.overlay.trackInfo = nil
		m = m.dismissPreview()
		return m, tea.Quit
	case actionFilterBar:
		return m, func() tea.Msg { return FilterBarMsg{} }
	case actionSwitchLibrary:
		m = m.switchToView(viewLibrary)
	case actionSwitchPlaylists:
		m = m.switchToView(viewPlaylists)
	case actionSwitchQueue:
		m = m.switchToView(viewQueue)
		m.queue = m.queue.FocusNowPlaying(m.playback.currentIndex)
	case actionSwitchArtists:
		m = m.switchToView(viewArtists)
	case actionSwitchAlbums:
		m = m.switchToView(viewAlbums)
	case actionSwitchFavorites:
		m = m.switchToView(viewFavorites)
	case actionSwitchRecentlyAdded:
		m = m.switchToView(viewRecentlyAdded)
	case actionSwitchHistory:
		m = m.switchToView(viewHistory)
	case actionSwitchGenres:
		m = m.switchToView(viewGenres)
	case actionSwitchTags:
		m = m.switchToView(viewTags)
	case actionTagOpen:
		return m.handleTagOpen()
	case actionTagBack:
		m = m.handleTagBack()
	case actionTagCreate:
		return m.handleTagCreate()
	case actionTagRename:
		return m.handleTagRename()
	case actionTagDelete:
		return m.handleTagDelete()
	case actionTagEnqueueAll:
		return m.handleTagEnqueueAll()
	case actionTagMerge:
		return m.handleTagMerge()
	case actionGoTo:
		ctx := m.focusedGoToContext()
		if !ctx.found {
			m.statusBar.notification = newNotification("No track focused")
		} else {
			options := m.buildGoToOptions(ctx)
			if len(options) > 0 {
				modal := NewGoToModal(options)
				m.overlay.goTo = &modal
			}
		}
	case actionCommandPalette:
		commands := m.buildPaletteCommands()
		if len(commands) > 0 {
			palette, cmd := NewCommandPalette(commands)
			m.overlay.commandPalette = &palette
			return m, cmd
		}
	case actionTagPicker:
		return m.openTagPicker()
	case actionTagFilter:
		return m.handleTagFilterAction()
	}
	return m, nil
}
