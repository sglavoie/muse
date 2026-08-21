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

// clearPendingConfirmations resets the confirmation bar. The insights merge
// plan is cleared with it: the bar is the only thing telling the user a merge
// is armed, so a plan outliving it would fire on an unrelated keypress.
func (m *Model) clearPendingConfirmations() {
	m.confirmationBar = ""
	m.insights.pendingMerge = nil
}

// switchToView transitions to the given view, clearing pending confirmations,
// resetting all sort states, and lazily deriving artist/album lists when needed.
// viewLazyLoadCmd returns the command that populates a view's data on first
// entry, for the views that load asynchronously. Returns nil for the rest.
// The pointer receiver matters: the insights loader stamps a generation onto
// the model that the reply is matched against.
func (m *Model) viewLazyLoadCmd(id viewID) tea.Cmd {
	switch id {
	case viewMixes:
		return m.mixesMaybeLoadCmd()
	case viewInsights:
		return m.insightsMaybeLoadCmd()
	}
	return nil
}

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
	if id == viewQueue && m.followNowPlaying {
		m.queue = m.queue.ScrollToNowPlaying(m.playback.currentIndex)
	}
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
	m.library = m.library.refreshView()
	return m
}

// handleLibraryModeNext cycles the Library view through its three modes:
// All → Recent → History → All, loading mode-specific data as needed.
func (m Model) handleLibraryModeNext() (Model, tea.Cmd) { return m.shiftLibraryMode(1) }

// handleLibraryModePrev cycles the Library view backward: All → History → Recent → All.
func (m Model) handleLibraryModePrev() (Model, tea.Cmd) { return m.shiftLibraryMode(2) }

// shiftLibraryMode rotates the Library mode by delta steps, loads mode-specific
// data, and updates the status bar.
func (m Model) shiftLibraryMode(delta LibraryMode) (Model, tea.Cmd) {
	// Exit visual mode before switching the underlying tracks list. Leaving
	// visualState.active=true with a snapshot of the previous mode's hashes
	// would cause updateVisualSelection to restore those old selections on
	// the next cursor move, contaminating the new mode's selection state.
	m = exitVisualMode(m)
	next := (m.library.mode + delta) % 3
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

// handleBrowseModeNext cycles the Browse view forward: Artists → Albums → Genres → Artists.
func (m Model) handleBrowseModeNext() (Model, tea.Cmd) { return m.shiftBrowseMode(1) }

// handleBrowseModePrev cycles the Browse view backward: Artists → Genres → Albums → Artists.
func (m Model) handleBrowseModePrev() (Model, tea.Cmd) { return m.shiftBrowseMode(2) }

// shiftBrowseMode rotates the Browse sub-mode by delta steps and updates the status bar.
func (m Model) shiftBrowseMode(delta BrowseMode) (Model, tea.Cmd) {
	// Exit visual mode before switching browse sub-modes. visual.browseMode
	// is snapshotted at enter time, so leaving it active after a sub-mode
	// shift would let updateVisualSelection iterate the previous sub-mode's
	// list while the cursor reads from the new sub-mode — mismatching
	// indices and silently corrupting selection state.
	m = exitVisualMode(m)
	m.browse.mode = (m.browse.mode + delta) % 3
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
			tracks := m.library.index.Tracks()
			m.browse.artists.artists = arttui.DeriveArtists(tracks)
			m.browse.artists.artistCounts = arttui.DeriveArtistCounts(tracks)
		}
	case BrowseModeAlbums:
		if len(m.browse.albums.albums) == 0 {
			tracks := m.library.index.Tracks()
			m.browse.albums.albums = albtui.DeriveAllAlbums(tracks)
			m.browse.albums.albumCounts = albtui.DeriveAllAlbumCounts(tracks)
		}
	case BrowseModeGenres:
		if len(m.browse.genres.genres) == 0 {
			tracks := m.library.index.Tracks()
			m.browse.genres.genres = gentui.DeriveGenres(tracks)
			m.browse.genres.genreCounts = gentui.DeriveGenreCounts(tracks)
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
		m.helpViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-2, 0)))
		m.refreshHelpViewport()
	case actionNextView:
		next := nextView(m.activeView)
		loadCmd := m.viewLazyLoadCmd(next)
		m = m.switchToView(next)
		if loadCmd != nil {
			return m, loadCmd
		}
	case actionPrevView:
		prev := prevView(m.activeView)
		loadCmd := m.viewLazyLoadCmd(prev)
		m = m.switchToView(prev)
		if loadCmd != nil {
			return m, loadCmd
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
	case actionSwitchInsights:
		cmd := m.insightsMaybeLoadCmd()
		m = m.switchToView(viewInsights)
		return m, cmd
	case actionInsightsPanelNext:
		return m.handleInsightsPanelCycle(1), nil
	case actionInsightsPanelPrev:
		return m.handleInsightsPanelCycle(-1), nil
	case actionInsightsWindowNext:
		return m.handleInsightsWindowCycle(1)
	case actionInsightsWindowPrev:
		return m.handleInsightsWindowCycle(-1)
	case actionInsightsOpen:
		return m.handleInsightsOpen(), nil
	case actionInsightsBack:
		return m.handleInsightsBack(), nil
	case actionInsightsEnqueueAll:
		return m.handleInsightsEnqueueAll()
	case actionInsightsAntiRut:
		return m.handleInsightsAntiRut()
	case actionInsightsRadioSeed:
		return m.handleInsightsRadioSeed()
	case actionInsightsJumpBrowse:
		return m.handleInsightsJumpBrowse(), nil
	case actionInsightsSavePlaylist:
		return m.handleInsightsSavePlaylist()
	case actionInsightsMergeGenre:
		return m.handleInsightsMergeGenre(), nil
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
			} else {
				// Track was focused but no jump targets apply (e.g. orphan
				// track missing artist/album/genre, or current view has no
				// applicable targets). Silently swallowing the keypress
				// leaves the user wondering whether the key was received.
				m.statusBar.notify("No jump targets available")
			}
		}
	case actionCommandPalette:
		commands := m.buildPaletteCommands()
		if len(commands) > 0 {
			palette, cmd := NewCommandPalette(commands, m.paletteUsage)
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
