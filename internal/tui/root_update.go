package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Update dispatches msg and then drains any listen the handler ended. The
// drain lives here rather than at each end site because several of those sites
// are value-receiver helpers that cannot return a tea.Cmd, and updateInner's
// many early returns make this the only point that observes all of them.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	// Every branch of updateInner returns this Model.
	updated := next.(Model)
	updated, flushCmd := updated.drainListenFlushes()
	if flushCmd == nil {
		return updated, cmd
	}
	return updated, tea.Batch(cmd, flushCmd)
}

// updateInner handles incoming messages. Number keys switch active views;
// Ctrl+C quits. Escape-style navigation remains delegated to the active view.
func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m.handleScanDoneMsg()

	case library.ScanErrorMsg:
		m.statusBar.notify(fmt.Sprintf("Scan error: %v", msg.Err))
		if m.scanErrCh != nil {
			return m, waitForScanErr(m.scanErrCh)
		}

	case library.WatchStatusMsg:
		// The background library pipeline reports watcher availability exactly
		// once. On failure the watcher never delivers live change events, so
		// mark the status bar degraded and tell the user to rely on manual
		// rescan. A setup error and a graceful fd-exhaustion fallback are
		// distinguished so the notification matches what actually happened.
		if !msg.Available {
			m.watchDegraded = true
			if msg.Err != nil {
				m.statusBar.notify("File watcher failed to start; use manual rescan (Shift+R)")
			} else {
				m.statusBar.notify("File watcher unavailable; use manual rescan (Shift+R)")
			}
		}

	case tickMsg:
		return m.handleTickMsg()

	case pulseTickMsg:
		// Receipt forces a redraw so the now-playing row returns to its
		// normal styling once pulseUntil has elapsed. No state changes.
		return m, nil

	case notificationFadeMsg:
		// Receipt forces a redraw so the status-bar notification renders its
		// dim fade frame for the last notificationFadeWindow of its lifetime.
		// No state changes — render-time check on isFading() picks up the dim.
		return m, nil

	case syncPositionMsg:
		return m.handleSyncPositionMsg()

	case sessionTickMsg:
		return m.handleSessionTickMsg()

	case playback.PlayerStatusMsg:
		m = m.handlePlayerStatusMsg(msg)

	case playback.MissingTrackMsg:
		m = m.handleMissingTrackMsg(msg)

	case playback.PlayerDisconnectedMsg:
		m = m.handlePlayerDisconnectedMsg()

	case playback.PlaybackEndedMsg:
		var cmd tea.Cmd
		m, cmd = m.handlePlaybackEndedMsg()
		if m.playback.state == playback.StatePlaying {
			return m, tea.Batch(cmd, m.sessionPositionCmd())
		}
		if cmd != nil {
			return m, cmd
		}

	case playback.PauseChangedMsg:
		return m.handlePauseChangedMsg(msg)

	case playback.RemoteActionMsg:
		m, cmd, _ := m.handlePlaybackKeyAction(playback.RemoteAction(msg.ActionName))
		return m, cmd

	case previewStartedMsg:
		m = m.handlePreviewStartedMsg(msg)

	case previewStartErrorMsg:
		m = m.handlePreviewStartErrorMsg(msg)

	case overlay.TrackInfoMsg:
		if msg.Err != nil {
			m.statusBar.notify("Track info failed: " + msg.Err.Error())
		} else {
			var userTags []db.TagRow
			if m.tagReader != nil && msg.Hash != "" {
				userTags, _ = m.tagReader.ReadTagsForTrack(msg.Hash)
			}
			overlay := overlay.NewTrackInfoOverlay(msg.Path, msg.Tags, msg.PlayCount, userTags)
			m.overlay.trackInfo = &overlay
		}

	case playback.TrackChangedMsg:
		return m.handleTrackChangedMsg(msg)

	case playback.PlayRecordedMsg:
		m = m.handlePlayRecordedMsg(msg)

	case FilterBarMsg:
		return m.handleFilterBarMsg()

	case libtui.TrackEnqueuedMsg:
		m = m.handleTrackEnqueuedMsg(msg)

	case BatchEnqueuedMsg:
		m = m.handleBatchEnqueuedMsg(msg)

	case playback.PlayQueueIndexMsg:
		return m.handlePlayQueueIndexMsg(msg)

	case playback.PlayFromIndexResultMsg:
		m = m.handlePlayFromIndexResultMsg(msg)

	case playback.MissingTrackSelectedMsg:
		return m.handleMissingTrackSelectedMsg(msg)

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
		m.statusBar.notify(string(msg))

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

	case favoriteToggledMsg:
		m = m.handleFavoriteToggledMsg(msg)

	case favoriteTargetDismissedMsg:
		m.overlay.favoriteTargetPicker = nil

	case favoriteTargetPickedMsg:
		return m.handleFavoriteTargetPicked(msg)

	case favoritesLoadedMsg:
		m = m.handleFavoritesLoadedMsg(msg)

	case metadataEditedMsg:
		if msg.err != nil {
			m.statusBar.notify("Metadata save failed: " + msg.err.Error())
		}
		// Genres, health findings and every library-derived shelf read the
		// values this edit changed. The index was rewritten before the write was
		// even attempted, so a failed write leaves them just as stale.
		m.insightsInvalidate()

	case overlay.GoToDismissedMsg:
		m.overlay.goTo = nil

	case overlay.GoToPickedMsg:
		m.overlay.goTo = nil
		switch msg.Kind {
		case overlay.GoToArtist:
			m = m.jumpToArtist(msg.Value)
		case overlay.GoToAlbum:
			m = m.jumpToAlbum(msg.Value)
		case overlay.GoToGenre:
			m = m.jumpToGenre(msg.Value)
		case overlay.GoToPlaylist:
			m = m.jumpToPlaylist(msg.Value)
		}

	case CommandPaletteDismissedMsg:
		m.overlay.commandPalette = nil

	case CommandPalettePickedMsg:
		m.overlay.commandPalette = nil
		// In a fixture-backed harness run, gate key/text-driven palette
		// selections through the same allowlist as direct command actions so a
		// scenario cannot execute a side-effectful command (Finder, clipboard,
		// open-directory) by opening the palette and pressing enter.
		if m.fixtureSafeCommands && !isSafeDirectCommand(msg.Command.Key()) {
			name := msg.Command.Name
			return m, func() tea.Msg {
				return TransientNotifyMsg(fmt.Sprintf("Command %q blocked in fixture mode", name))
			}
		}
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
			scheduleStandalonePurge(m.tagWriter.PurgeDeletedTags)
		}
		m.statusBar.notify("All tags removed from track")

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

	case RadioBatchEnqueuedMsg:
		return m.handleRadioBatchEnqueuedMsg(msg)

	case mixCountsLoadedMsg:
		m = m.handleMixCountsLoadedMsg(msg)

	case mixLoadedMsg:
		m = m.handleMixLoadedMsg(msg)

	case insightsLoadedMsg:
		m = m.handleInsightsLoadedMsg(msg)

	case insightsMergedMsg:
		return m.handleInsightsMergedMsg(msg)

	case recommendationCacheRebuiltMsg:
		// Cache rebuilt; no additional state change needed — playsSinceRefresh
		// is already reset by handleMixCountsLoadedMsg which fires in parallel.

	case mixSavedAsPlaylistMsg:
		m = m.handleMixSavedAsPlaylistMsg(msg)

	case mixSaveAsPlaylistErrorMsg:
		m.statusBar.notify("Save mix failed: " + msg.err.Error())

	case mixDismissedMsg:
		m = m.handleMixDismissedMsg(msg)

	case mixDismissErrorMsg:
		m.statusBar.notify("Dismiss failed: " + msg.err.Error())

	case mixUndismissedMsg:
		m = m.handleMixUndismissedMsg(msg)

	case mixUndismissErrorMsg:
		m.statusBar.notify("Undismiss failed: " + msg.err.Error())

	case commandPaletteActionMsg:
		return m.handleCommandPaletteAction(msg.action, msg.view)

	case navigateLibraryModeMsg:
		return m.handleNavigateLibraryModeMsg(msg)

	case navigateBrowseModeMsg:
		return m.handleNavigateBrowseModeMsg(msg)

	case desktop.ClipboardResultMsg:
		if msg.Err != nil {
			m.statusBar.notify("Copy failed: " + msg.Err.Error())
		} else {
			m.statusBar.notify("Copied: " + msg.Text)
		}

	case metadataExportRequestedMsg:
		return m.handleMetadataExportRequest()

	case metadataExportedMsg:
		m = m.handleMetadataExportedMsg(msg)

	case metadataImportRequestedMsg:
		return m.handleMetadataImportRequest()

	case metadataImportParsedMsg:
		m = m.handleMetadataImportParsedMsg(msg)

	case metadataImportAppliedMsg:
		m = m.handleMetadataImportAppliedMsg(msg)

	case configToggleMsg:
		return m.handleConfigToggleMsg(msg)

	case randomAlbumMsg:
		return m.handleRandomAlbumMsg()

	case randomArtistMsg:
		return m.handleRandomArtistMsg()

	case jumpToNowPlayingMsg:
		return m.handleJumpToNowPlayingMsg()

	case toggleFooterHintsMsg:
		return m.handleToggleFooterHintsMsg()

	case toggleTrackNotifyMsg:
		return m.handleToggleTrackNotifyMsg()

	case toggleQueueFollowMsg:
		return m.handleToggleQueueFollowMsg()

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
