package tui

import (
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/recommendation"
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
	m.helpViewport.SetContent(HelpOverlay{}.render(*m.styles, sections, title, m.termWidth))
	m.helpViewport.SetYOffset(0)
	extraLines := 0
	if m.helpSearchFilter.Active || m.helpSearchFilter.Text != "" {
		extraLines = 1
	}
	m.helpViewport.SetHeight(max(m.termHeight-2-extraLines, 0))
}

// clearPendingConfirmations resets the confirmation bar.
func (m *Model) clearPendingConfirmations() {
	m.confirmationBar = ""
}

// switchToView transitions to the given view, clearing pending confirmations,
// resetting all sort states, and lazily deriving artist/album lists when needed.
func (m Model) switchToView(id viewID) Model {
	m.clearPendingConfirmations()
	m = m.resetAllSorts()
	m = exitVisualMode(m)
	if m.queue.savingAsPlaylist {
		m.queue = m.queue.cancelSaveAsPlaylist()
	}
	if m.playlists.creating {
		m.playlists = m.playlists.cancelCreate()
	}
	if m.playlists.renaming {
		m.playlists = m.playlists.cancelRename()
	}
	if m.tagsView.creating {
		m.tagsView = m.tagsView.cancelCreate()
	}
	if m.tagsView.renaming {
		m.tagsView = m.tagsView.cancelRename()
	}
	m.activeView = id
	if id == viewMixes && len(m.mixes.mixes) == 0 {
		m.mixes.mixes = append([]MixInfo(nil), recommendation.Registry()...)
	}
	if id == viewBrowse {
		m = m.deriveCurrentBrowseData()
	}
	if id == viewFavorites && m.favoriteReader != nil {
		artistCount, albumCount, trackCount, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.artistCount = artistCount
			m.favorites.albumCount = albumCount
			m.favorites.trackCount = trackCount
		}
	}
	if id == viewLibrary {
		switch m.library.mode {
		case LibraryModeRecent:
			m = m.syncRecentlyAddedFromLibrary()
		case LibraryModeHistory:
			m = m.loadLibraryHistoryData()
		}
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

// loadLibraryHistoryData loads play history from historyReader into the Library
// view's source tracks when in History mode.
func (m Model) loadLibraryHistoryData() Model {
	if m.historyReader == nil {
		return m
	}
	tracks, _ := m.historyReader.ReadPlayHistory(500)
	m.library.sourceTracks = tracks
	m.library = m.library.recomputeSortedTracks()
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	return m
}

// handleLibraryModeNext cycles the Library view through its three modes:
// All → Recent → History → All, loading mode-specific data as needed.
func (m Model) handleLibraryModeNext() (Model, tea.Cmd) {
	next := (m.library.mode + 1) % 3
	m.library = m.library.switchMode(next)
	switch next {
	case LibraryModeRecent:
		m = m.syncRecentlyAddedFromLibrary()
		m.statusBar.notifyInfo("Library: Recently Added")
	case LibraryModeHistory:
		m = m.loadLibraryHistoryData()
		m.statusBar.notifyInfo("Library: History")
	default:
		m.statusBar.notifyInfo("Library: All tracks")
	}
	return m, nil
}

// handleLibraryModePrev cycles the Library view backward: All → History → Recent → All.
func (m Model) handleLibraryModePrev() (Model, tea.Cmd) {
	prev := (m.library.mode + 2) % 3
	m.library = m.library.switchMode(prev)
	switch prev {
	case LibraryModeRecent:
		m = m.syncRecentlyAddedFromLibrary()
		m.statusBar.notifyInfo("Library: Recently Added")
	case LibraryModeHistory:
		m = m.loadLibraryHistoryData()
		m.statusBar.notifyInfo("Library: History")
	default:
		m.statusBar.notifyInfo("Library: All tracks")
	}
	return m, nil
}

// handleBrowseModeNext cycles the Browse view forward: Artists → Albums → Genres → Artists.
func (m Model) handleBrowseModeNext() (Model, tea.Cmd) {
	m.browse.mode = (m.browse.mode + 1) % 3
	m = m.deriveCurrentBrowseData()
	switch m.browse.mode {
	case BrowseModeAlbums:
		m.statusBar.notifyInfo("Browse: Albums")
	case BrowseModeGenres:
		m.statusBar.notifyInfo("Browse: Genres")
	default:
		m.statusBar.notifyInfo("Browse: Artists")
	}
	return m, nil
}

// handleBrowseModePrev cycles the Browse view backward: Artists → Genres → Albums → Artists.
func (m Model) handleBrowseModePrev() (Model, tea.Cmd) {
	m.browse.mode = (m.browse.mode + 2) % 3
	m = m.deriveCurrentBrowseData()
	switch m.browse.mode {
	case BrowseModeAlbums:
		m.statusBar.notifyInfo("Browse: Albums")
	case BrowseModeGenres:
		m.statusBar.notifyInfo("Browse: Genres")
	default:
		m.statusBar.notifyInfo("Browse: Artists")
	}
	return m, nil
}

// deriveCurrentBrowseData lazily derives the artist/album/genre list for the
// active browse sub-mode when it has not been populated yet.
func (m Model) deriveCurrentBrowseData() Model {
	switch m.browse.mode {
	case BrowseModeArtists:
		if len(m.browse.artists.artists) == 0 {
			m.browse.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
		}
	case BrowseModeAlbums:
		if len(m.browse.albums.albums) == 0 {
			m.browse.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
		}
	case BrowseModeGenres:
		if len(m.browse.genres.genres) == 0 {
			m.browse.genres.genres = gentui.DeriveGenres(m.library.index.Tracks())
		}
	}
	return m
}

// hintForAction returns a contextual hint for actions that are only valid in
// specific views. Returns empty string when no hint applies.
func hintForAction(a action, view viewID, _ BrowseMode) string {
	switch a {
	case actionDequeue:
		if view != viewQueue {
			return "d removes tracks in Queue — press 3 to go there"
		}
	case actionClearQueue:
		if view != viewQueue {
			return "C clears the Queue — press 3 to go there"
		}
	}
	return ""
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
		m.helpFiltered = true
		m.helpSearchFilter = filter.State{}
		m.helpViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(m.termHeight-2))
		m.refreshHelpViewport()
	case actionNextView:
		next := nextView(m.activeView)
		var mixCmd tea.Cmd
		if next == viewMixes {
			mixCmd = m.mixesMaybeLoadCmd()
		}
		m = m.switchToView(next)
		if mixCmd != nil {
			return m, mixCmd
		}
	case actionPrevView:
		prev := prevView(m.activeView)
		var mixCmd tea.Cmd
		if prev == viewMixes {
			mixCmd = m.mixesMaybeLoadCmd()
		}
		m = m.switchToView(prev)
		if mixCmd != nil {
			return m, mixCmd
		}
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
	case actionSwitchBrowse:
		m = m.switchToView(viewBrowse)
	case actionSwitchFavorites:
		m = m.switchToView(viewFavorites)
	case actionLibraryModeNext:
		return m.handleLibraryModeNext()
	case actionLibraryModePrev:
		return m.handleLibraryModePrev()
	case actionBrowseModeNext:
		return m.handleBrowseModeNext()
	case actionBrowseModePrev:
		return m.handleBrowseModePrev()
	case actionSwitchTags:
		m = m.switchToView(viewTags)
	case actionSwitchMixes:
		cmd := m.mixesMaybeLoadCmd()
		m = m.switchToView(viewMixes)
		return m, cmd
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
	case actionMixOpen:
		return m.handleMixOpen()
	case actionMixBack:
		m = m.handleMixBack()
	case actionMixEnqueueAll:
		return m.handleMixEnqueueAll()
	case actionSaveMixAsPlaylist:
		return m.handleMixesSaveMixAsPlaylist()
	case actionMixRadioSeed:
		return m.handleMixesRadioSeed()
	case actionMixDismiss:
		return m.handleMixDismiss()
	case actionMixUndismiss:
		return m.handleMixUndismiss()
	case actionGoTo:
		ctx := m.focusedGoToContext()
		if !ctx.found {
			m.statusBar.notify("No track focused")
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
	if hint := hintForAction(action, m.activeView, m.browse.mode); hint != "" {
		m.statusBar.pushMsg(hint, 3*time.Second, notifInfo)
	}
	return m, nil
}
