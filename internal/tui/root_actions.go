package tui

import (
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/recommendation"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/filter"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// helpState returns the sections and title to display in the help overlay,
// depending on whether the user has toggled view-specific filtering.
func (m Model) helpState() ([]overlay.HelpSection, string) {
	if !m.helpFiltered {
		return overlay.HelpSections(), "Keybindings  (? to filter by view)"
	}
	ctx := viewToContext(m.prevActiveView)
	name := ViewName(m.prevActiveView)
	return overlay.FilteredHelpSections(ctx), "Keybindings — " + name + "  (? to show all)"
}

// refreshHelpViewport rebuilds the help viewport content from the current
// help state and search filter, then resets the scroll position to the top.
func (m *Model) refreshHelpViewport() {
	sections, title := m.helpState()
	if m.helpSearchFilter.Text != "" {
		sections = overlay.SearchHelpSections(sections, m.helpSearchFilter.Text)
	}
	m.helpViewport.SetContent(overlay.HelpOverlay{}.Render(*m.styles, sections, title, m.termWidth))
	m.helpViewport.SetYOffset(0)
	extraLines := 0
	if m.helpSearchFilter.Active || m.helpSearchFilter.Text != "" {
		extraLines = 1
	}
	m.helpViewport.SetHeight(max(m.termHeight-3-extraLines, 0))
}

// clearPendingConfirmations resets the confirmation bar. The insights merge
// plan is cleared with it: the bar is the only thing telling the user a merge
// is armed, so a plan outliving it would fire on an unrelated keypress.
func (m *Model) clearPendingConfirmations() {
	m.confirmationBar = ""
	m.insights.PendingMerge = nil
}

// viewLazyLoadCmd returns the command that populates a view's data on first
// entry, for the views that load asynchronously. Returns nil for the rest.
// The pointer receiver matters: the insights loader stamps a generation onto
// the model that the reply is matched against.
func (m *Model) viewLazyLoadCmd(id viewID) tea.Cmd {
	switch id {
	case viewMixes:
		return m.mixesMaybeLoadCmd()
	case viewInsights:
		return m.insightsEnterCmd()
	}
	return nil
}

// switchToView preserves each view's sort and cursor while loading its data.
func (m Model) switchToView(id viewID) Model {
	m.clearPendingConfirmations()
	m = exitVisualMode(m)
	if m.queue.SavingAsPlaylist {
		m.queue = m.queue.CancelSaveAsPlaylist()
	}
	if m.playlists.Creating {
		m.playlists = m.playlists.CancelCreate()
	}
	if m.playlists.Renaming {
		m.playlists = m.playlists.CancelRename()
	}
	if m.tagsView.Creating {
		m.tagsView = m.tagsView.CancelCreate()
	}
	if m.tagsView.Renaming {
		m.tagsView = m.tagsView.CancelRename()
	}
	m.ActiveView = id
	if id == viewQueue && m.followNowPlaying {
		m.queue = m.queue.ScrollToNowPlaying(m.playback.currentIndex)
	}
	if id == viewMixes && len(m.mixes.Mixes) == 0 {
		m.mixes.Mixes = append([]recommendation.MixInfo(nil), recommendation.Registry()...)
	}
	if id == viewBrowse {
		m = m.deriveCurrentBrowseData()
	}
	if id == viewFavorites && m.favoriteReader != nil {
		artistCount, albumCount, trackCount, err := m.favoriteReader.ReadFavoriteCounts()
		if err == nil {
			m.favorites.ArtistCount = artistCount
			m.favorites.AlbumCount = albumCount
			m.favorites.TrackCount = trackCount
		}
	}
	if id == viewLibrary {
		switch m.library.Mode {
		case libtui.LibraryModeRecent:
			m = m.syncRecentlyAddedFromLibrary()
		case libtui.LibraryModeHistory:
			m = m.loadLibraryHistoryData()
		}
	}
	if id == viewTags {
		// Sync global tag list into the tags view and load counts if needed.
		m = m.syncTagsViewTags()
		if m.tagsView.TagCounts == nil && m.tagReader != nil {
			counts, _ := m.tagReader.ReadTagCounts()
			m.tagsView.TagCounts = counts
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
	m.library.SourceTracks = tracks
	m.library = m.library.RefreshView()
	return m
}

// handleLibraryModeNext cycles the Library view through its three modes:
// All → Recent → History → All, loading mode-specific data as needed.
func (m Model) handleLibraryModeNext() (Model, tea.Cmd) { return m.shiftLibraryMode(1) }

// handleLibraryModePrev cycles the Library view backward: All → History → Recent → All.
func (m Model) handleLibraryModePrev() (Model, tea.Cmd) { return m.shiftLibraryMode(2) }

// shiftLibraryMode rotates the Library mode by delta steps, loads mode-specific
// data, and updates the status bar.
func (m Model) shiftLibraryMode(delta libtui.LibraryMode) (Model, tea.Cmd) {
	// Exit visual mode before switching the underlying tracks list. Leaving
	// visualState.active=true with a snapshot of the previous mode's hashes
	// would cause updateVisualSelection to restore those old selections on
	// the next cursor move, contaminating the new mode's selection state.
	m = exitVisualMode(m)
	next := (m.library.Mode + delta) % 3
	m.library = m.library.SwitchMode(next)
	switch next {
	case libtui.LibraryModeRecent:
		m = m.syncRecentlyAddedFromLibrary()
		m.statusBar.NotifyInfo("Library: Recently Added")
	case libtui.LibraryModeHistory:
		m = m.loadLibraryHistoryData()
		m.statusBar.NotifyInfo("Library: History")
	default:
		m.statusBar.NotifyInfo("Library: All tracks")
	}
	return m, nil
}

// handleBrowseModeNext cycles the Browse view forward: Artists → Albums → Genres → Artists.
func (m Model) handleBrowseModeNext() (Model, tea.Cmd) { return m.shiftBrowseMode(1) }

// handleBrowseModePrev cycles the Browse view backward: Artists → Genres → Albums → Artists.
func (m Model) handleBrowseModePrev() (Model, tea.Cmd) { return m.shiftBrowseMode(2) }

// shiftBrowseMode rotates the Browse sub-mode by delta steps and updates the status bar.
func (m Model) shiftBrowseMode(delta browse.BrowseMode) (Model, tea.Cmd) {
	// Exit visual mode before switching browse sub-modes. visual.browseMode
	// is snapshotted at enter time, so leaving it active after a sub-mode
	// shift would let updateVisualSelection iterate the previous sub-mode's
	// list while the cursor reads from the new sub-mode — mismatching
	// indices and silently corrupting selection state.
	m = exitVisualMode(m)
	m.browse.Mode = (m.browse.Mode + delta) % 3
	m = m.deriveCurrentBrowseData()
	switch m.browse.Mode {
	case browse.BrowseModeAlbums:
		m.statusBar.NotifyInfo("Browse: Albums")
	case browse.BrowseModeGenres:
		m.statusBar.NotifyInfo("Browse: Genres")
	default:
		m.statusBar.NotifyInfo("Browse: Artists")
	}
	return m, nil
}

// deriveCurrentBrowseData lazily derives the artist/album/genre list for the
// active browse sub-mode when it has not been populated yet.
func (m Model) deriveCurrentBrowseData() Model {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		if len(m.browse.Artists.Artists) == 0 {
			tracks := m.library.Index.Tracks()
			m.browse.Artists.Artists = arttui.DeriveArtists(tracks)
			m.browse.Artists.ArtistCounts = arttui.DeriveArtistCounts(tracks)
		}
	case browse.BrowseModeAlbums:
		if len(m.browse.Albums.Albums) == 0 {
			tracks := m.library.Index.Tracks()
			m.browse.Albums.Albums = albtui.DeriveAllAlbums(tracks)
			m.browse.Albums.AlbumCounts = albtui.DeriveAllAlbumCounts(tracks)
		}
	case browse.BrowseModeGenres:
		if len(m.browse.Genres.Genres) == 0 {
			tracks := m.library.Index.Tracks()
			m.browse.Genres.Genres = gentui.DeriveGenres(tracks)
			m.browse.Genres.GenreCounts = gentui.DeriveGenreCounts(tracks)
		}
	}
	return m
}

// hintForAction returns a contextual hint for actions that are only valid in
// specific views. Returns empty string when no hint applies.
func hintForAction(a keys.Action, view viewID, _ browse.BrowseMode) string {
	switch a {
	case keys.ActionDequeue:
		if view != viewQueue {
			return "d removes tracks in Queue — press 3 to go there"
		}
	case keys.ActionClearQueue:
		if view != viewQueue {
			return "C clears the Queue — press 3 to go there"
		}
	}
	return ""
}

// handleKeyAction dispatches a resolved KeyAction to the appropriate handler.
func (m Model) handleKeyAction(action keys.Action) (tea.Model, tea.Cmd) {
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
	case keys.ActionSettings:
		return m.handleSettingsOpen()
	case keys.ActionDebug:
		m.debugPrevView = m.ActiveView
		m.debugVisible = true
		m.debugViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-2, 0)))
		panel := collectDebugInfo(m)
		m.debugViewport.SetContent(panel.render(*m.styles))
		m.debugViewport.SetYOffset(0)
	case keys.ActionQueueSummary:
		if m.overlay.queueSummary != nil {
			m.overlay.queueSummary = nil
		} else {
			m.overlay.queueSummary = m.buildQueueSummary()
		}
	case keys.ActionModesLegend:
		if m.overlay.modesLegend != nil {
			m.overlay.modesLegend = nil
		} else {
			m.overlay.modesLegend = m.buildModesLegend()
		}
	case keys.ActionHelp:
		m.prevActiveView = m.ActiveView
		m.helpVisible = true
		m.helpFiltered = true
		m.helpSearchFilter = filter.State{}
		m.helpViewport = viewport.New(viewport.WithWidth(m.termWidth), viewport.WithHeight(max(m.termHeight-2, 0)))
		m.refreshHelpViewport()
	case keys.ActionNextView:
		next := nextView(m.ActiveView)
		loadCmd := m.viewLazyLoadCmd(next)
		m = m.switchToView(next)
		if loadCmd != nil {
			return m, loadCmd
		}
	case keys.ActionPrevView:
		prev := prevView(m.ActiveView)
		loadCmd := m.viewLazyLoadCmd(prev)
		m = m.switchToView(prev)
		if loadCmd != nil {
			return m, loadCmd
		}
	case keys.ActionQuit:
		m.overlay.nowPlaying = nil
		m.overlay.queueSummary = nil
		m.overlay.modesLegend = nil
		m.overlay.trackInfo = nil
		m = m.closePreview()
		m.overlay.previewWasPlaying = false
		return m.markQuitting(), tea.Quit
	case keys.ActionFilterBar:
		return m, func() tea.Msg { return FilterBarMsg{} }
	case keys.ActionSwitchLibrary:
		m = m.switchToView(viewLibrary)
	case keys.ActionSwitchPlaylists:
		m = m.switchToView(viewPlaylists)
	case keys.ActionSwitchQueue:
		m = m.switchToView(viewQueue)
		m.queue = m.queue.FocusNowPlaying(m.playback.currentIndex)
	case keys.ActionSwitchBrowse:
		m = m.switchToView(viewBrowse)
	case keys.ActionSwitchFavorites:
		m = m.switchToView(viewFavorites)
	case keys.ActionLibraryModeNext:
		return m.handleLibraryModeNext()
	case keys.ActionLibraryModePrev:
		return m.handleLibraryModePrev()
	case keys.ActionBrowseModeNext:
		return m.handleBrowseModeNext()
	case keys.ActionBrowseModePrev:
		return m.handleBrowseModePrev()
	case keys.ActionSwitchTags:
		m = m.switchToView(viewTags)
	case keys.ActionSwitchMixes:
		cmd := m.mixesMaybeLoadCmd()
		m = m.switchToView(viewMixes)
		return m, cmd
	case keys.ActionSwitchInsights:
		cmd := m.insightsEnterCmd()
		m = m.switchToView(viewInsights)
		return m, cmd
	case keys.ActionInsightsPanelNext:
		return m.handleInsightsPanelCycle(1), nil
	case keys.ActionInsightsPanelPrev:
		return m.handleInsightsPanelCycle(-1), nil
	case keys.ActionInsightsWindowNext:
		return m.handleInsightsWindowCycle(1)
	case keys.ActionInsightsWindowPrev:
		return m.handleInsightsWindowCycle(-1)
	case keys.ActionInsightsOpen:
		return m.handleInsightsOpen()
	case keys.ActionInsightsBack:
		return m.handleInsightsBack(), nil
	case keys.ActionInsightsEnqueueAll:
		return m.handleInsightsEnqueueAll()
	case keys.ActionInsightsAntiRut:
		return m.handleInsightsAntiRut()
	case keys.ActionInsightsRadioSeed:
		return m.handleInsightsRadioSeed()
	case keys.ActionInsightsJumpBrowse:
		return m.handleInsightsJumpBrowse(), nil
	case keys.ActionInsightsSavePlaylist:
		return m.handleInsightsSavePlaylist()
	case keys.ActionInsightsMergeGenre:
		return m.handleInsightsMergeGenre(), nil
	case keys.ActionTagOpen:
		return m.handleTagOpen()
	case keys.ActionTagBack:
		m = m.handleTagBack()
	case keys.ActionTagCreate:
		return m.handleTagCreate()
	case keys.ActionTagRename:
		return m.handleTagRename()
	case keys.ActionTagDelete:
		return m.handleTagDelete()
	case keys.ActionTagEnqueueAll:
		return m.handleTagEnqueueAll()
	case keys.ActionTagMerge:
		return m.handleTagMerge()
	case keys.ActionMixOpen:
		return m.handleMixOpen()
	case keys.ActionMixBack:
		m = m.handleMixBack()
	case keys.ActionMixEnqueueAll:
		return m.handleMixEnqueueAll()
	case keys.ActionSaveMixAsPlaylist:
		return m.handleMixesSaveMixAsPlaylist()
	case keys.ActionMixRadioSeed:
		return m.handleMixesRadioSeed()
	case keys.ActionMixDismiss:
		return m.handleMixDismiss()
	case keys.ActionMixUndismiss:
		return m.handleMixUndismiss()
	case keys.ActionGoTo:
		ctx := m.focusedGoToContext()
		if !ctx.found {
			m.statusBar.Notify("No track focused")
		} else {
			options := m.buildGoToOptions(ctx)
			if len(options) > 0 {
				modal := overlay.NewGoToModal(options)
				m.overlay.goTo = &modal
			} else {
				// Track was focused but no jump targets apply (e.g. orphan
				// track missing artist/album/genre, or current view has no
				// applicable targets). Silently swallowing the keypress
				// leaves the user wondering whether the key was received.
				m.statusBar.Notify("No jump targets available")
			}
		}
	case keys.ActionCommandPalette:
		commands := m.BuildPaletteCommands()
		if len(commands) > 0 {
			cp, cmd := palette.NewCommandPalette(commands, m.paletteUsage)
			m.overlay.commandPalette = &cp
			return m, cmd
		}
	case keys.ActionTagPicker:
		return m.openTagPicker()
	case keys.ActionTagFilter:
		return m.handleTagFilterAction()
	case keys.ActionAutoDJTune:
		return m.openAutoDJTunePanel()
	}
	if hint := hintForAction(action, m.ActiveView, m.browse.Mode); hint != "" {
		m.statusBar.PushMsg(hint, 3*time.Second, statusbar.NotifInfo)
	}
	return m, nil
}

// buildModesLegend snapshots the live mode state into a legend overlay. Radio
// lives on m.radio, not m.playback (root_view_status.go:11-16), so all four
// indicators are read from the same places renderStatusBar reads them.
func (m Model) buildModesLegend() *overlay.ModesLegendOverlay {
	return &overlay.ModesLegendOverlay{
		Repeat:    m.playback.repeatMode,
		Shuffle:   m.playback.shuffle,
		StopAfter: m.playback.stopAfterCurrent,
		Radio:     m.radio.enabled,
	}
}

// handleModesLegendKeyPress handles key events while the playback-mode legend
// overlay is active. Returns (model, cmd, true) when the key was consumed. The
// nil guard keeps ';' free for handlers further down orderedKeyPressHandlers —
// an active filter input still receives it as literal text.
func (m Model) handleModesLegendKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.modesLegend == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.modesLegend = nil
		return m.markQuitting(), tea.Quit, true
	case "esc", ";":
		m.overlay.modesLegend = nil
		return m, nil, true
	default:
		return m, nil, true
	}
}
