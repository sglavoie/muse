package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Update handles incoming messages. Number keys switch active views; Ctrl+C
// quits. Escape-style navigation remains delegated to the active view.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.handleWindowSizeMsg(msg)

	case tea.BackgroundColorMsg:
		s := styles.NewStyles(msg.IsDark())
		m.styles = &s

	case library.ScanProgressMsg:
		m = m.handleScanProgressMsg(msg)

	case library.ScanTotalMsg:
		m = m.handleScanTotalMsg(msg)

	case library.ScanBatchMsg:
		return m.handleScanBatchMsg(msg)

	case watchBatchMsg:
		return m.handleWatchBatchMsg(msg)

	case library.DurationAuditBatchMsg:
		return m.handleDurationAuditBatchMsg(msg)

	case library.DurationAuditDoneMsg:
		return m.handleDurationAuditDoneMsg(msg)

	case library.ScanDoneMsg:
		m = m.handleScanDoneMsg()

	case library.ScanErrorMsg:
		m.statusBar.notification = newNotification(fmt.Sprintf("Scan error: %v", msg.Err))
		if m.scanErrCh != nil {
			return m, waitForScanErr(m.scanErrCh)
		}

	case tickMsg:
		return m.handleTickMsg()

	case syncPositionMsg:
		return m.handleSyncPositionMsg()

	case sessionTickMsg:
		return m.handleSessionTickMsg()

	case PlayerStatusMsg:
		m = m.handlePlayerStatusMsg(msg)

	case MissingTrackMsg:
		m = m.handleMissingTrackMsg(msg)

	case PlayerDisconnectedMsg:
		m = m.handlePlayerDisconnectedMsg()

	case PlaybackEndedMsg:
		var cmd tea.Cmd
		m, cmd = m.handlePlaybackEndedMsg()
		if m.playback.state == PlaybackPlaying {
			return m, tea.Batch(cmd, m.sessionPositionCmd())
		}
		if cmd != nil {
			return m, cmd
		}

	case PauseChangedMsg:
		return m.handlePauseChangedMsg(msg)

	case RemoteActionMsg:
		m, cmd, _ := m.handlePlaybackKeyAction(remoteAction(msg.ActionName))
		return m, cmd

	case previewStartedMsg:
		m = m.handlePreviewStartedMsg(msg)

	case previewStartErrorMsg:
		m = m.handlePreviewStartErrorMsg(msg)

	case trackInfoMsg:
		if msg.err != nil {
			m.statusBar.notification = newNotification("Track info failed: " + msg.err.Error())
		} else {
			var userTags []db.TagRow
			if m.tagReader != nil && msg.hash != "" {
				userTags, _ = m.tagReader.ReadTagsForTrack(msg.hash)
			}
			overlay := newTrackInfoOverlay(msg.path, msg.tags, msg.playCount, userTags)
			m.overlay.trackInfo = &overlay
		}

	case TrackChangedMsg:
		return m.handleTrackChangedMsg(msg)

	case FilterBarMsg:
		return m.handleFilterBarMsg()

	case libtui.TrackEnqueuedMsg:
		m = m.handleTrackEnqueuedMsg(msg)

	case BatchEnqueuedMsg:
		m = m.handleBatchEnqueuedMsg(msg)

	case PlayQueueIndexMsg:
		m = m.handlePlayQueueIndexMsg(msg)

	case MissingTrackSelectedMsg:
		m.statusBar.notification = newNotification("Cannot play — file is missing")

	case DequeueTrackMsg:
		m = m.handleDequeueTrackMsg(msg)

	case BulkSelectionToggleMsg:
		m = m.handleBulkSelectionToggleMsg(msg)

	case ClearQueueMsg:
		m = m.handleClearQueueMsg()

	case UndoMsg:
		m = m.handleUndo()

	case MoveTrackUpMsg:
		return m.handleMoveTrackUpMsg(msg)

	case MoveTrackDownMsg:
		return m.handleMoveTrackDownMsg(msg)

	case QueuePastedMsg:
		m = m.handleQueuePastedMsg(msg)

	case TransientNotifyMsg:
		m.statusBar.notification = newNotification(string(msg))

	case settingsSavedMsg:
		m = m.handleSettingsSavedMsg(msg)

	case tea.KeyPressMsg:
		return m.handleKeyPressMsg(msg)

	case playlistTracksLoadedMsg:
		m = m.handlePlaylistTracksLoadedMsg(msg)

	case playlistTracksLoadErrorMsg:
		m = m.handlePlaylistTracksLoadErrorMsg(msg)

	case PlaylistRenameMsg:
		m = m.handlePlaylistRenameMsg()

	case playlistRenamedMsg:
		m = m.handlePlaylistRenamedMsg(msg)

	case playlistRenameErrorMsg:
		m = m.handlePlaylistRenameErrorMsg(msg)

	case PlaylistCreateMsg:
		m = m.handlePlaylistCreateMsg()

	case playlistCreatedMsg:
		return m.handlePlaylistCreatedMsg(msg)

	case playlistCreateErrorMsg:
		m = m.handlePlaylistCreateErrorMsg(msg)

	case queueSavedAsPlaylistMsg:
		m = m.handleQueueSavedAsPlaylistMsg(msg)

	case queueSaveAsPlaylistErrorMsg:
		m = m.handleQueueSaveAsPlaylistErrorMsg(msg)

	case playlistAddErrorMsg:
		m = m.handlePlaylistAddErrorMsg(msg)

	case tracksAddedToPlaylistMsg:
		m = m.handleTracksAddedToPlaylistMsg(msg)

	case smartPlaylistCreatedMsg:
		return m.handleSmartPlaylistCreatedMsg(msg)

	case smartPlaylistEditedMsg:
		return m.handleSmartPlaylistEditedMsg(msg)

	case smartPlaylistCountsUpdatedMsg:
		m = m.handleSmartPlaylistCountsUpdatedMsg(msg)

	case listeningStatsMsg:
		panel := StatsPanel{
			stats:       msg.stats,
			periodLabel: statsPeriods[m.statsPeriod].label,
			err:         msg.err,
		}
		m.statsViewport.SetContent(panel.render(*m.styles))
		m.statsViewport.SetYOffset(0)

	case favoriteToggledMsg:
		m = m.handleFavoriteToggledMsg(msg)

	case favoritesLoadedMsg:
		m = m.handleFavoritesLoadedMsg(msg)

	case metadataEditedMsg:

	case GoToDismissedMsg:
		m.overlay.goTo = nil

	case GoToPickedMsg:
		m.overlay.goTo = nil
		switch msg.Kind {
		case goToArtist:
			m = m.jumpToArtist(msg.Value)
		case goToAlbum:
			m = m.jumpToAlbum(msg.Value)
		case goToGenre:
			m = m.jumpToGenre(msg.Value)
		case goToPlaylist:
			m = m.jumpToPlaylist(msg.Value)
		}

	case CommandPaletteDismissedMsg:
		m.overlay.commandPalette = nil

	case CommandPalettePickedMsg:
		m.overlay.commandPalette = nil
		if msg.Command.Exec != nil {
			return m, msg.Command.Exec()
		}

	case tagPickerDismissedMsg:
		m.overlay.tagPicker = nil

	case TagPickerToggleMsg:
		return m.handleTagPickerToggle(msg)

	case TagPickerCreateMsg:
		return m.handleTagPickerCreate(msg)

	case tagCreatedMsg:
		m = m.refreshTagsViewAfterCreate(msg)
		return m.handleTagCreated(msg)

	case tagToggledMsg:
		return m.handleTagToggled(msg)

	case emptyTagsPurgedMsg:
		for _, id := range msg.tagIDs {
			m = m.removeTagFromCaches(id)
		}

	case allTagsRemovedMsg:
		for _, id := range msg.autoDeletedTagIDs {
			m = m.removeTagFromCaches(id)
		}
		if len(msg.autoDeletedTagIDs) > 0 && m.tagWriter != nil {
			schedulePurge(&m.undo.slot, m.tagWriter.PurgeDeletedTags)
		}
		m.statusBar.notification = newNotification("All tags removed from track")

	case tagsRefreshedMsg:
		m = m.handleTagsRefreshed(msg)

	case tagViewTracksLoadedMsg:
		m = m.handleTagViewTracksLoaded(msg)

	case tagViewCountsLoadedMsg:
		m = m.handleTagViewCountsLoaded(msg)

	case tagRenamedMsg:
		m = m.handleTagRenamed(msg)

	case tagDeletedMsg:
		m = m.handleTagDeleted(msg)

	case tagEnqueueAllMsg:
		m = m.handleTagEnqueueAll2(msg)

	case tagMergeDismissedMsg:
		m.overlay.tagMergePicker = nil

	case tagMergePickedMsg:
		return m.handleTagMergePicked(msg)

	case tagMergedMsg:
		return m.handleTagMerged(msg)

	case tagFilterPickedMsg:
		return m.handleTagFilterPicked(msg)

	case tagFilterDismissedMsg:
		m = m.handleTagFilterDismissed()

	case openTagFilterPickerMsg:
		return m.handleOpenTagFilterPicker()

	case clearTagFilterMsg:
		m = m.clearTagFilter()

	case removeAllTagsFromTrackMsg:
		return m.handleRemoveAllTagsFromTrack(msg)

	case enqueueByTagMsg:
		return m.handleEnqueueByTag(msg)

	case commandPaletteActionMsg:
		return m.handleKeyAction(msg.action)

	case clipboardResultMsg:
		if msg.err != nil {
			m.statusBar.notification = newNotification("Copy failed: " + msg.err.Error())
		} else {
			m.statusBar.notification = newNotification("Copied: " + msg.text)
		}

	case configToggleMsg:
		return m.handleConfigToggleMsg(msg)

	case randomAlbumMsg:
		return m.handleRandomAlbumMsg()

	case randomArtistMsg:
		return m.handleRandomArtistMsg()

	case jumpToNowPlayingMsg:
		return m.handleJumpToNowPlayingMsg()

	case queueShuffleMsg:
		return m.handleQueueShuffleMsg()

	case queueDedupMsg:
		return m.handleQueueDedupMsg()

	case playlistShuffleMsg:
		return m.handlePlaylistShuffleMsg()

	case pltui.DestinationPickerDismissedMsg:
		m = m.handleDestinationPickerDismissedMsg()

	case pltui.DestinationPickerCreateMsg:
		m = m.handleDestinationPickerCreateMsg()

	case pltui.DestinationPickedMsg:
		return m.handleDestinationPickedMsg(msg)

	default:
		return m.handleUnhandledMsg(msg)
	}
	return m, nil
}
