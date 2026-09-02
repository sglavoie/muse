package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handleDurationAuditBatchMsg applies a batch of ffprobe-corrected durations
// to the in-memory library index and any matching queue entries, then
// re-schedules the next batch read. Derived views that cache track data
// (recently-added, sorted library, history, etc.) are refreshed so their
// duration columns stay in sync.
func (m Model) handleDurationAuditBatchMsg(msg library.DurationAuditBatchMsg) (Model, tea.Cmd) {
	for _, fix := range msg.Fixes {
		if m.library.Index.UpdateDuration(fix.Hash, float64(fix.DurationMs)/1000) {
			m.auditCorrected++
		}
		for i := range m.queue.Entries {
			if m.queue.Entries[i].Hash == fix.Hash {
				m.queue.Entries[i].DurationMs = fix.DurationMs
			}
		}
	}
	m = m.preserveQueueSort()
	m = m.syncRecentlyAddedFromLibrary()
	m.library = m.library.RefreshView()
	// Propagate corrected durations to all derived views so their
	// duration columns stay in sync without requiring navigation away
	// and back.
	fixMap := make(map[string]int64, len(msg.Fixes))
	for _, fix := range msg.Fixes {
		fixMap[fix.Hash] = fix.DurationMs
	}
	if m.library.Mode == libtui.LibraryModeHistory && len(m.library.SourceTracks) > 0 {
		for i := range m.library.SourceTracks {
			if ms, ok := fixMap[m.library.SourceTracks[i].Hash]; ok {
				m.library.SourceTracks[i].Duration = float64(ms) / 1000
			}
		}
		m.library = m.library.RefreshView()
	}
	if m.playlists.Level == pltui.TrackLevel {
		for i := range m.playlists.Tracks {
			if ms, ok := fixMap[m.playlists.Tracks[i].Hash]; ok {
				m.playlists.Tracks[i].DurationMs = ms
			}
		}
		m = m.preservePlaylistsSort()
	}
	if m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks {
		for i := range m.favorites.Tracks {
			if ms, ok := fixMap[m.favorites.Tracks[i].Hash]; ok {
				m.favorites.Tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.favorites = m.favorites.RecomputeFavoritesSort()
	}
	if m.favorites.Level == favtui.TrackLevel {
		for i := range m.favorites.DrilldownTracks {
			if ms, ok := fixMap[m.favorites.DrilldownTracks[i].Hash]; ok {
				m.favorites.DrilldownTracks[i].Duration = float64(ms) / 1000
			}
		}
		m.favorites = m.favorites.RecomputeFavoritesSort()
	}
	if m.tagsView.Level == tagtui.TrackLevel {
		for i := range m.tagsView.Tracks {
			if ms, ok := fixMap[m.tagsView.Tracks[i].Hash]; ok {
				m.tagsView.Tracks[i].DurationMs = ms
			}
		}
		m.tagsView = m.tagsView.RecomputeTagsSort()
	}
	if m.browse.Artists.Level == arttui.TrackLevel {
		for i := range m.browse.Artists.Tracks {
			if ms, ok := fixMap[m.browse.Artists.Tracks[i].Hash]; ok {
				m.browse.Artists.Tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
	}
	if m.browse.Albums.Level == albtui.TrackLevel {
		for i := range m.browse.Albums.Tracks {
			if ms, ok := fixMap[m.browse.Albums.Tracks[i].Hash]; ok {
				m.browse.Albums.Tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
	}
	if m.browse.Genres.Level == gentui.TrackLevel {
		for i := range m.browse.Genres.Tracks {
			if ms, ok := fixMap[m.browse.Genres.Tracks[i].Hash]; ok {
				m.browse.Genres.Tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
	}
	// Propagate to cached mix tracks so opening a mix shows corrected
	// durations, and refresh the active mix's trackView when at track level.
	for id, cached := range m.mixes.CachedMixes {
		for i := range cached {
			if ms, ok := fixMap[cached[i].Hash]; ok {
				cached[i].Duration = float64(ms) / 1000
			}
		}
		m.mixes.CachedMixes[id] = cached
	}
	if m.mixes.Level == 1 {
		m.mixes.TrackView = m.mixes.TrackView.RefreshView()
	}
	if m.auditCh != nil {
		return m, waitForAuditBatch(m.auditCh)
	}
	return m, nil
}

// handleDurationAuditDoneMsg shows a notification when tracks were corrected
// and marks the audit as complete by nilling the channel.
func (m Model) handleDurationAuditDoneMsg(_ library.DurationAuditDoneMsg) (Model, tea.Cmd) {
	corrected := m.auditCorrected
	if corrected > 0 {
		m.statusBar.Notify(
			fmt.Sprintf("Duration audit: corrected %d %s", corrected,
				text.Pluralize(corrected, "track", "tracks")),
		)
	}
	m.auditCh = nil
	m.auditCorrected = 0
	return m, nil
}
