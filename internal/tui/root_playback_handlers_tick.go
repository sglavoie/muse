package tui

import (
	"math"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// maxListenDeltaSecs bounds the gap between two elapsed samples that counts as
// audio actually heard. It is roughly 3x the 1s tick, so a jump beyond it is a
// seek rather than playback and only resyncs the baseline.
const maxListenDeltaSecs = 3.0

// shouldCountPlay returns true when the user has listened to enough of the
// track to count it as a play: at least 50% of the duration or 30 seconds,
// whichever comes first.
func shouldCountPlay(listenedSecs, totalSecs float64) bool {
	if totalSecs <= 0 || listenedSecs <= 0 {
		return false
	}
	return listenedSecs >= 30 || listenedSecs >= totalSecs*0.5
}

// finitePositive reports whether v is a finite, non-negative real number. Used
// to reject NaN/Inf/negative readings from mpv before they corrupt playback
// progress, session listen totals, or RecordListen aggregates.
func finitePositive(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

// handleTickMsg updates elapsed time for the main player, preview player, and
// now-playing overlay, and clears expired status bar notifications.
func (m Model) handleTickMsg() (Model, tea.Cmd) {
	var mainDurationCmd tea.Cmd
	if m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused {
		if er, ok := m.player.(elapsedReader); ok {
			if elapsed, total, err := er.ElapsedPosition(); err == nil && finitePositive(elapsed) && finitePositive(total) {
				m.playback.elapsedSecs = elapsed
				m.playback.totalSecs = total
				// A seek in either direction only resyncs the baseline: skipped
				// audio must not accumulate, and re-listened audio accumulates
				// again on the following ticks.
				if delta := elapsed - m.playback.lastSampleSecs; delta > 0 && delta <= maxListenDeltaSecs {
					m.playback.listenedSecs += delta
				}
				m.playback.lastSampleSecs = elapsed
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
				m.overlay.preview.Elapsed = elapsed
				m.overlay.preview.Total = total
				if !m.overlay.previewDurationFilled && total > 0 {
					m, previewDurationCmd = m.correctStoredDuration(m.overlay.preview.Hash, total)
					m.overlay.previewDurationFilled = true
				}
			}
		} else {
			m = m.dismissPreview()
		}
	}

	if m.overlay.nowPlaying != nil {
		m.overlay.nowPlaying.Elapsed = m.playback.elapsedSecs
		m.overlay.nowPlaying.Total = m.playback.totalSecs
		m.overlay.nowPlaying.State = m.playback.state
	}
	m.statusBar.PruneNotifications()
	// Schedule a precise fade tick at expiry-minus-fade-window for the
	// soonest pending notification, when one is within the next tick window.
	// The 1s tickCmd would otherwise miss the 250 ms fade window.
	fadeCmd := notificationFadeCmd(m.statusBar.NextFadeAt())
	// The tick is the only hook every Insights invalidation reaches: undo
	// performs on a path whose Cmd is discarded, so an eager reload at each
	// invalidation site would miss it.
	insightsCmd := m.insightsActiveRefreshCmd()
	m, radioCleanupCmd := m.startRadioBatchCleanupCmd()
	return m, tea.Batch(tickCmd(), previewDurationCmd, mainDurationCmd, fadeCmd, insightsCmd, radioCleanupCmd)
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
	t, ok := m.library.Index.FindByHash(hash)
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

	m.library.Index.UpdateDuration(hash, mpvTotal)
	m.library = m.library.RefreshView()
	m = m.syncRecentlyAddedFromLibrary()
	newDurationMs := int64(mpvTotal * 1000)
	for i := range m.queue.Entries {
		if m.queue.Entries[i].Hash == hash {
			m.queue.Entries[i].DurationMs = newDurationMs
		}
	}
	m = m.preserveQueueSort()
	if m.library.Mode == libtui.LibraryModeHistory {
		for i := range m.library.SourceTracks {
			if m.library.SourceTracks[i].Hash == hash {
				m.library.SourceTracks[i].Duration = mpvTotal
			}
		}
		m.library = m.library.RefreshView()
	}
	if m.playlists.Level == pltui.TrackLevel {
		for i := range m.playlists.Tracks {
			if m.playlists.Tracks[i].Hash == hash {
				m.playlists.Tracks[i].DurationMs = newDurationMs
			}
		}
		m = m.preservePlaylistsSort()
	}
	if m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks {
		for i := range m.favorites.Tracks {
			if m.favorites.Tracks[i].Hash == hash {
				m.favorites.Tracks[i].Duration = mpvTotal
			}
		}
		m.favorites = m.favorites.RecomputeFavoritesSort()
	}
	if m.favorites.Level == favtui.TrackLevel {
		for i := range m.favorites.DrilldownTracks {
			if m.favorites.DrilldownTracks[i].Hash == hash {
				m.favorites.DrilldownTracks[i].Duration = mpvTotal
			}
		}
		m.favorites = m.favorites.RecomputeFavoritesSort()
	}
	if m.tagsView.Level == tagtui.TrackLevel {
		for i := range m.tagsView.Tracks {
			if m.tagsView.Tracks[i].Hash == hash {
				m.tagsView.Tracks[i].DurationMs = newDurationMs
			}
		}
		m.tagsView = m.tagsView.RecomputeTagsSort()
	}
	if m.browse.Artists.Level == arttui.TrackLevel {
		for i := range m.browse.Artists.Tracks {
			if m.browse.Artists.Tracks[i].Hash == hash {
				m.browse.Artists.Tracks[i].Duration = mpvTotal
			}
		}
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
	}
	if m.browse.Albums.Level == albtui.TrackLevel {
		for i := range m.browse.Albums.Tracks {
			if m.browse.Albums.Tracks[i].Hash == hash {
				m.browse.Albums.Tracks[i].Duration = mpvTotal
			}
		}
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
	}
	if m.browse.Genres.Level == gentui.TrackLevel {
		for i := range m.browse.Genres.Tracks {
			if m.browse.Genres.Tracks[i].Hash == hash {
				m.browse.Genres.Tracks[i].Duration = mpvTotal
			}
		}
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
	}
	// Propagate to cached mix tracks so opening a mix shows the corrected
	// duration, and refresh the active mix's trackView when at track level.
	for id, cached := range m.mixes.CachedMixes {
		for i := range cached {
			if cached[i].Hash == hash {
				cached[i].Duration = mpvTotal
			}
		}
		m.mixes.CachedMixes[id] = cached
	}
	if m.mixes.Level == 1 {
		m.mixes.TrackView = m.mixes.TrackView.RefreshView()
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
	if m.playback.state == playback.StatePlaying {
		cmd = m.sessionPositionCmd()
	}
	return m, tea.Batch(sessionTickCmd(), cmd)
}
