package tui

import (
	"math"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// shouldCountPlay returns true when the user has listened to enough of the
// track to count it as a play: at least 50% of the duration or 30 seconds,
// whichever comes first.
func shouldCountPlay(elapsedSecs, totalSecs float64) bool {
	if totalSecs <= 0 || elapsedSecs <= 0 {
		return false
	}
	return elapsedSecs >= 30 || elapsedSecs >= totalSecs*0.5
}

// finitePositive reports whether v is a finite, non-negative real number. Used
// to reject NaN/Inf/negative readings from mpv before they corrupt playback
// progress, session listen totals, or RecordPlay aggregates.
func finitePositive(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

// handleTickMsg updates elapsed time for the main player, preview player, and
// now-playing overlay, and clears expired status bar notifications.
func (m Model) handleTickMsg() (Model, tea.Cmd) {
	var mainDurationCmd tea.Cmd
	if m.playback.state == PlaybackPlaying || m.playback.state == PlaybackPaused {
		if er, ok := m.player.(elapsedReader); ok {
			if elapsed, total, err := er.ElapsedPosition(); err == nil && finitePositive(elapsed) && finitePositive(total) {
				m.playback.elapsedSecs = elapsed
				m.playback.totalSecs = total
				if elapsed > m.playback.peakElapsedSecs {
					m.playback.peakElapsedSecs = elapsed
					m.playback.peakTotalSecs = total
				}
			}
		}
		if !m.playback.durationCorrected && m.playback.totalSecs > 0 && m.playback.currentHash != "" {
			m, mainDurationCmd = m.correctStoredDuration(m.playback.currentHash, m.playback.totalSecs)
			m.playback.durationCorrected = true
		}
	}

	var previewDurationCmd tea.Cmd
	if m.overlay.preview != nil && m.overlay.previewPlayer != nil {
		if elapsed, total, err := m.overlay.previewPlayer.ElapsedPosition(); err == nil {
			if finitePositive(elapsed) && finitePositive(total) {
				m.overlay.preview.elapsed = elapsed
				m.overlay.preview.total = total
				if !m.overlay.previewDurationFilled && total > 0 {
					m, previewDurationCmd = m.correctStoredDuration(m.overlay.preview.hash, total)
					m.overlay.previewDurationFilled = true
				}
			}
		} else {
			m = m.dismissPreview()
		}
	}

	if m.overlay.nowPlaying != nil {
		m.overlay.nowPlaying.elapsed = m.playback.elapsedSecs
		m.overlay.nowPlaying.total = m.playback.totalSecs
		m.overlay.nowPlaying.state = m.playback.state
	}
	m.statusBar.pruneNotifications()
	// Schedule a precise fade tick at expiry-minus-fade-window for the
	// soonest pending notification, when one is within the next tick window.
	// The 1s tickCmd would otherwise miss the 250 ms fade window.
	fadeCmd := notificationFadeCmd(m.statusBar.nextFadeAt())
	// The tick is the only hook every Insights invalidation reaches: undo
	// performs on a path whose Cmd is discarded, so an eager reload at each
	// invalidation site would miss it.
	insightsCmd := m.insightsActiveRefreshCmd()
	return m, tea.Batch(tickCmd(), previewDurationCmd, mainDurationCmd, fadeCmd, insightsCmd)
}

// correctStoredDuration compares the mpv-reported total with the stored library
// duration and, if they differ by more than 2 seconds (or the stored value is
// zero), updates the library index, queue entries, and returns a tea.Cmd that
// persists the correction to the database.
func (m Model) correctStoredDuration(hash string, mpvTotal float64) (Model, tea.Cmd) {
	const threshold = 2.0
	// Reject non-finite or non-positive mpvTotal values outright so a bogus
	// mpv reading cannot overwrite a known-good stored duration with a
	// negative/NaN value.
	if mpvTotal <= 0 || math.IsNaN(mpvTotal) || math.IsInf(mpvTotal, 0) {
		return m, nil
	}
	t, ok := m.library.index.FindByHash(hash)
	if !ok {
		return m, nil
	}
	diff := t.Duration - mpvTotal
	if diff < 0 {
		diff = -diff
	}
	if t.Duration != 0 && diff <= threshold {
		return m, nil
	}

	m.library.index.UpdateDuration(hash, mpvTotal)
	m.library = m.library.refreshView()
	m = m.syncRecentlyAddedFromLibrary()
	newDurationMs := int64(mpvTotal * 1000)
	for i := range m.queue.entries {
		if m.queue.entries[i].Hash == hash {
			m.queue.entries[i].DurationMs = newDurationMs
		}
	}
	m = m.preserveQueueSort()
	if m.library.mode == LibraryModeHistory {
		for i := range m.library.sourceTracks {
			if m.library.sourceTracks[i].Hash == hash {
				m.library.sourceTracks[i].Duration = mpvTotal
			}
		}
		m.library = m.library.refreshView()
	}
	if m.playlists.level == pltui.TrackLevel {
		for i := range m.playlists.tracks {
			if m.playlists.tracks[i].Hash == hash {
				m.playlists.tracks[i].DurationMs = newDurationMs
			}
		}
		m = m.preservePlaylistsSort()
	}
	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		for i := range m.favorites.tracks {
			if m.favorites.tracks[i].Hash == hash {
				m.favorites.tracks[i].Duration = mpvTotal
			}
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.favorites.level == favtui.TrackLevel {
		for i := range m.favorites.drilldownTracks {
			if m.favorites.drilldownTracks[i].Hash == hash {
				m.favorites.drilldownTracks[i].Duration = mpvTotal
			}
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.tagsView.level == tagtui.TrackLevel {
		for i := range m.tagsView.tracks {
			if m.tagsView.tracks[i].Hash == hash {
				m.tagsView.tracks[i].DurationMs = newDurationMs
			}
		}
		m.tagsView = m.tagsView.recomputeTagsSort()
	}
	if m.browse.artists.level == arttui.TrackLevel {
		for i := range m.browse.artists.tracks {
			if m.browse.artists.tracks[i].Hash == hash {
				m.browse.artists.tracks[i].Duration = mpvTotal
			}
		}
		m.browse.artists = m.browse.artists.recomputeArtistsSort()
	}
	if m.browse.albums.level == albtui.TrackLevel {
		for i := range m.browse.albums.tracks {
			if m.browse.albums.tracks[i].Hash == hash {
				m.browse.albums.tracks[i].Duration = mpvTotal
			}
		}
		m.browse.albums = m.browse.albums.recomputeAlbumsSort()
	}
	if m.browse.genres.level == gentui.TrackLevel {
		for i := range m.browse.genres.tracks {
			if m.browse.genres.tracks[i].Hash == hash {
				m.browse.genres.tracks[i].Duration = mpvTotal
			}
		}
		m.browse.genres = m.browse.genres.recomputeGenresSort()
	}
	// Propagate to cached mix tracks so opening a mix shows the corrected
	// duration, and refresh the active mix's trackView when at track level.
	for id, cached := range m.mixes.cachedMixes {
		for i := range cached {
			if cached[i].Hash == hash {
				cached[i].Duration = mpvTotal
			}
		}
		m.mixes.cachedMixes[id] = cached
	}
	if m.mixes.level == 1 {
		m.mixes.trackView = m.mixes.trackView.refreshView()
	}

	var cmd tea.Cmd
	if dw := m.durationWriter; dw != nil {
		cmd = func() tea.Msg {
			_ = dw.UpdateTrackDuration(hash, newDurationMs)
			return nil
		}
	}
	return m, cmd
}

// handleSyncPositionMsg persists the current playback position via the player.
func (m Model) handleSyncPositionMsg() (Model, tea.Cmd) {
	if ps, ok := m.player.(positionSyncer); ok {
		_ = ps.SyncPosition()
	}
	return m, syncPositionCmd()
}

// handleSessionTickMsg fires a periodic session position update when playing.
func (m Model) handleSessionTickMsg() (Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.playback.state == PlaybackPlaying {
		cmd = m.sessionPositionCmd()
	}
	return m, tea.Batch(sessionTickCmd(), cmd)
}
