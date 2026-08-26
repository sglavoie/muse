package tui

import (
	"fmt"
	"math/rand/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleConfigToggleMsg toggles an audio setting, applies it to the player,
// persists the config, and shows a notification.
func (m Model) handleConfigToggleMsg(msg configToggleMsg) (Model, tea.Cmd) {
	var note string
	switch msg.field {
	case toggleNormalize:
		next := !m.cfg.NormalizeVolume
		if err := applyPlayerStep(m.normalizeApplyStep(m.cfg.NormalizeVolume, next)); err != nil {
			m.statusBar.notify("Volume normalization failed: " + err.Error())
			return m, nil
		}
		m.cfg.NormalizeVolume = next
		note = "Volume normalization: " + onOff(m.cfg.NormalizeVolume)

	case toggleEQ:
		next := !m.cfg.EQEnabled
		if err := applyPlayerStep(m.equalizerApplyStep(m.cfg.EQEnabled, m.cfg.EQBands, next, m.cfg.EQBands)); err != nil {
			m.statusBar.notify("Equalizer failed: " + err.Error())
			return m, nil
		}
		m.cfg.EQEnabled = next
		note = "Equalizer: " + onOff(m.cfg.EQEnabled)

	case toggleStereoWide:
		next := !m.cfg.StereoWideEnabled
		if err := applyPlayerStep(m.stereoWideApplyStep(m.cfg.StereoWideEnabled, m.cfg.StereoWideAmount, next, m.cfg.StereoWideAmount)); err != nil {
			m.statusBar.notify("Stereo widening failed: " + err.Error())
			return m, nil
		}
		m.cfg.StereoWideEnabled = next
		note = "Stereo widening: " + onOff(m.cfg.StereoWideEnabled)
	}

	m.statusBar.notify(note)

	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}

// onOff returns "On" when b is true, "Off" otherwise.
func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

// viewSwitchActions maps each primary view to the action that switches to it.
// The legacy sub-view IDs (viewArtists/viewAlbums/viewGenres) are absent
// because they have no switch action and are not in viewOrder; a message
// naming one runs its action in place.
var viewSwitchActions = map[viewID]action{
	viewLibrary:   actionSwitchLibrary,
	viewPlaylists: actionSwitchPlaylists,
	viewQueue:     actionSwitchQueue,
	viewBrowse:    actionSwitchBrowse,
	viewFavorites: actionSwitchFavorites,
	viewTags:      actionSwitchTags,
	viewMixes:     actionSwitchMixes,
	viewInsights:  actionSwitchInsights,
}

// handleCommandPaletteAction replays a palette action, switching to target
// first when one is named and not already active.
//
// The switch is dispatched through handleKeyAction rather than switchToView so
// the switch actions keep their own effects: actionSwitchMixes returns
// mixesMaybeLoadCmd() and actionSwitchQueue focuses the now-playing row.
//
// The already-active guard is load-bearing: switchToView resets every sort,
// exits visual mode and cancels in-flight create/rename inputs, so switching
// to the view the user is already in would silently discard that state.
func (m Model) handleCommandPaletteAction(act action, target viewID) (tea.Model, tea.Cmd) {
	// The "Delete playlist" row names a single target, but the handler behind
	// actionPlaylistDelete removes the focused or selected tracks at playlist
	// track level. Refuse it there instead of deleting what the row never
	// promised.
	if act == actionPlaylistDelete && !atPlaylistListLevel(m) {
		m.statusBar.notify("Open the playlist list to delete a playlist")
		return m, nil
	}

	switchAction, ok := viewSwitchActions[target]
	if !ok || m.activeView == target {
		return m.atCollectionLevelFor(act).handleKeyAction(act)
	}
	switched, switchCmd := m.handleKeyAction(switchAction)
	next, ok := switched.(Model)
	if !ok {
		return switched, switchCmd
	}
	after, actionCmd := next.atCollectionLevelFor(act).handleKeyAction(act)
	return after, tea.Batch(switchCmd, actionCmd)
}

// atCollectionLevelFor drills the destination view back out to the collection
// level the create actions require. switchToView preserves each view's drill
// level, so a palette pick could otherwise land on a track level whose handler
// ignores the action and leave the user moved but with no create input.
//
// The active-view check keeps the drill-out tied to the view the action will
// run in: a create action dispatched without a target view still no-ops in
// place rather than reshaping a view the user is not looking at.
func (m Model) atCollectionLevelFor(act action) Model {
	switch act {
	case actionPlaylistCreate, actionSmartPlaylistCreate:
		if m.activeView == viewPlaylists && !m.playlists.atListLevel() {
			m = exitVisualMode(m)
			m.playlists = m.playlists.backToList()
		}
	case actionTagCreate:
		if m.activeView == viewTags && m.tagsView.level == tagtui.TrackLevel {
			m = exitVisualMode(m)
			m.tagsView = m.tagsView.back()
		}
	}
	return m
}

// handleNavigateLibraryModeMsg switches to an exact Library mode while
// retaining the per-mode cursor, scroll, and filter snapshot maintained by
// LibraryView. Re-selecting the active mode behaves like ordinary view
// navigation and does not discard its current selection.
func (m Model) handleNavigateLibraryModeMsg(msg navigateLibraryModeMsg) (Model, tea.Cmd) {
	if msg.mode > LibraryModeHistory {
		return m, nil
	}
	if m.library.mode != msg.mode {
		m.library = m.library.switchMode(msg.mode)
	}
	m = m.switchToView(viewLibrary)
	return m, nil
}

// handleNavigateBrowseModeMsg switches to an exact Browse mode. Browse's
// sub-views intentionally retain their own cursor, filter, and drill-down
// state, matching navigation via the mode-cycle keys.
func (m Model) handleNavigateBrowseModeMsg(msg navigateBrowseModeMsg) (Model, tea.Cmd) {
	if msg.mode > BrowseModeGenres {
		return m, nil
	}
	m.browse.mode = msg.mode
	m = m.switchToView(viewBrowse)
	return m, nil
}

// handleRandomAlbumMsg switches to Albums browse mode and positions the cursor
// on a randomly-chosen album. Any active filter is cleared first so the random
// index always refers to a visible row.
func (m Model) handleRandomAlbumMsg() (Model, tea.Cmd) {
	m.browse.mode = BrowseModeAlbums
	m = m.switchToView(viewBrowse)
	m.browse.albums.filter = filter.State{}
	if len(m.browse.albums.albums) == 0 {
		m.statusBar.notify("Library is empty")
		return m, nil
	}
	idx := rand.IntN(len(m.browse.albums.albums)) //nolint:gosec // non-security random for UI navigation
	m.browse.albums.cursor = idx
	m.browse.albums.scrollOffset = max(0, idx-m.browse.albums.height/2)
	m.browse.albums = m.browse.albums.clampCursor()
	return m, nil
}

// handleRandomArtistMsg switches to Artists browse mode and positions the cursor
// on a randomly-chosen artist. Any active filter is cleared first so the random
// index always refers to a visible row.
func (m Model) handleRandomArtistMsg() (Model, tea.Cmd) {
	m.browse.mode = BrowseModeArtists
	m = m.switchToView(viewBrowse)
	m.browse.artists.filter = filter.State{}
	if len(m.browse.artists.artists) == 0 {
		m.statusBar.notify("Library is empty")
		return m, nil
	}
	idx := rand.IntN(len(m.browse.artists.artists)) //nolint:gosec // non-security random for UI navigation
	m.browse.artists.cursor = idx
	m.browse.artists.scrollOffset = max(0, idx-m.browse.artists.height/2)
	m.browse.artists = m.browse.artists.clampCursor()
	return m, nil
}

// handleToggleFooterHintsMsg flips the footer hint strip on/off, updates the
// derived per-view height so the body reclaims (or yields) the row, and
// persists the new value to the config file.
func (m Model) handleToggleFooterHintsMsg() (Model, tea.Cmd) {
	m.showFooterHints = !m.showFooterHints
	m.cfg.ShowFooterHints = m.showFooterHints
	if m.termWidth > 0 && m.termHeight > 0 {
		m.setViewDimensions(m.termWidth, m.termHeight)
		m.clampAllScrollOffsets()
	}
	m.statusBar.notify("Footer hints: " + onOff(m.showFooterHints))

	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}

// handleToggleTrackNotifyMsg flips the automatic on-track-change macOS
// notification on/off and persists the new value to the config file.
func (m Model) handleToggleTrackNotifyMsg() (Model, tea.Cmd) {
	m.notifyOnTrackChange = !m.notifyOnTrackChange
	m.cfg.NotifyOnTrackChange = m.notifyOnTrackChange
	m.statusBar.notify("Track notify: " + onOff(m.notifyOnTrackChange))

	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}

// handleToggleQueueFollowMsg flips the Queue follow-now-playing mode,
// persists the new value to the config, and applies the scroll immediately
// when re-enabled while the Queue view is active.
func (m Model) handleToggleQueueFollowMsg() (Model, tea.Cmd) {
	m.followNowPlaying = !m.followNowPlaying
	m.cfg.QueueFollowNowPlaying = m.followNowPlaying
	if m.followNowPlaying && m.activeView == viewQueue {
		m.queue = m.queue.ScrollToNowPlaying(m.playback.currentIndex)
	}
	m.statusBar.notify("Queue follow: " + onOff(m.followNowPlaying))

	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}

// handleJumpToNowPlayingMsg switches to Library view with the cursor on the
// currently-playing track, clearing any active filter first.
func (m Model) handleJumpToNowPlayingMsg() (Model, tea.Cmd) {
	if m.playback.currentHash == "" {
		m.statusBar.notify("Nothing playing")
		return m, nil
	}
	m.library.filter = m.library.filter.Deactivate()
	m.library.filteredMatches = nil
	m = m.switchToView(viewLibrary)
	pos := m.library.findTrackPosition(m.playback.currentHash)
	m.library.cursor = pos
	m.library = m.library.clampCursor()
	return m, nil
}

// handlePlaylistShuffleMsg applies a Fisher-Yates shuffle to the focused
// playlist's tracks, either in-memory (TrackLevel) or via background load
// (ListLevel), and persists the new order when a playlistShuffler is available.
func (m Model) handlePlaylistShuffleMsg() (Model, tea.Cmd) {
	if m.activeView != viewPlaylists {
		m.statusBar.notify("No playlist focused")
		return m, nil
	}
	if m.playlists.atListLevel() {
		return m.shufflePlaylistFromList()
	}
	return m.shufflePlaylistFromTracks()
}

func (m Model) shufflePlaylistFromTracks() (Model, tea.Cmd) {
	if m.playlists.smartQuery != nil {
		m.statusBar.notify("Cannot shuffle smart playlist")
		return m, nil
	}
	if len(m.playlists.tracks) <= 1 {
		m.statusBar.notify("Playlist too small to shuffle")
		return m, nil
	}

	tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks))
	copy(tracks, m.playlists.tracks)
	for i := len(tracks) - 1; i > 0; i-- {
		j := rand.IntN(i + 1) //nolint:gosec // non-security random for playlist shuffle
		tracks[i], tracks[j] = tracks[j], tracks[i]
	}
	m.playlists.tracks = tracks
	m = m.resetPlaylistsSort()

	n := len(tracks)
	m.statusBar.notify(fmt.Sprintf("Playlist shuffled (%d tracks)", n))

	var playlistID int64
	if m.playlists.listCursor >= 0 && m.playlists.listCursor < len(m.playlists.playlists) {
		playlistID = m.playlists.playlists[m.playlists.listCursor].ID
	}

	if playlistID > 0 {
		if shuffler, ok := m.playlistDeleter.(playlistShuffler); ok {
			ids := make([]int64, n)
			for i, t := range tracks {
				ids[i] = t.ID
			}
			pid := playlistID
			return m, func() tea.Msg { _ = shuffler.SetPlaylistOrder(pid, ids); return nil }
		}
	}

	return m, nil
}

func (m Model) shufflePlaylistFromList() (Model, tea.Cmd) {
	pl, ok := m.playlists.focusedPlaylist()
	if !ok {
		m.statusBar.notify("No playlist focused")
		return m, nil
	}
	if pl.IsSmart {
		m.statusBar.notify("Cannot shuffle smart playlist")
		return m, nil
	}
	if pl.TrackCount <= 1 {
		m.statusBar.notify("Playlist too small to shuffle")
		return m, nil
	}
	if m.playlistReader == nil {
		return m, nil
	}

	reader := m.playlistReader
	var shuffler playlistShuffler
	if s, ok := m.playlistDeleter.(playlistShuffler); ok {
		shuffler = s
	}
	pid := pl.ID
	name := pl.Name

	return m, func() tea.Msg {
		tracks, err := reader.ReadPlaylistTracks(pid)
		if err != nil || len(tracks) <= 1 {
			return TransientNotifyMsg("Playlist too small to shuffle")
		}
		for i := len(tracks) - 1; i > 0; i-- {
			j := rand.IntN(i + 1) //nolint:gosec // non-security random for playlist shuffle
			tracks[i], tracks[j] = tracks[j], tracks[i]
		}
		if shuffler != nil {
			ids := make([]int64, len(tracks))
			for i, t := range tracks {
				ids[i] = t.ID
			}
			_ = shuffler.SetPlaylistOrder(pid, ids)
		}
		return TransientNotifyMsg(fmt.Sprintf("Shuffled %q (%d tracks)", name, len(tracks)))
	}
}
