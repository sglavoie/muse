package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleDurationAuditBatchMsg applies a batch of ffprobe-corrected durations
// to the in-memory library index and any matching queue entries, then
// re-schedules the next batch read. Derived views that cache track data
// (recently-added, sorted library, history, etc.) are refreshed so their
// duration columns stay in sync.
func (m Model) handleDurationAuditBatchMsg(msg library.DurationAuditBatchMsg) (Model, tea.Cmd) {
	m.auditCorrected += len(msg.Fixes)
	for _, fix := range msg.Fixes {
		m.library.index.UpdateDuration(fix.Hash, float64(fix.DurationMs)/1000)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == fix.Hash {
				m.queue.entries[i].DurationMs = fix.DurationMs
			}
		}
	}
	m = m.syncRecentlyAddedFromLibrary()
	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	// Propagate corrected durations to all derived views so their
	// duration columns stay in sync without requiring navigation away
	// and back.
	fixMap := make(map[string]int64, len(msg.Fixes))
	for _, fix := range msg.Fixes {
		fixMap[fix.Hash] = fix.DurationMs
	}
	if m.library.mode == LibraryModeHistory && len(m.library.sourceTracks) > 0 {
		for i := range m.library.sourceTracks {
			if ms, ok := fixMap[m.library.sourceTracks[i].Hash]; ok {
				m.library.sourceTracks[i].Duration = float64(ms) / 1000
			}
		}
		if m.library.sortState.Field != SortDefault {
			m.library = m.library.recomputeSortedTracks()
		}
		if m.library.filter.Text != "" {
			m.library = m.library.rebuildFilterCache()
		}
	}
	if m.playlists.level == pltui.TrackLevel {
		for i := range m.playlists.tracks {
			if ms, ok := fixMap[m.playlists.tracks[i].Hash]; ok {
				m.playlists.tracks[i].DurationMs = ms
			}
		}
		m = m.preservePlaylistsSort()
	}
	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		for i := range m.favorites.tracks {
			if ms, ok := fixMap[m.favorites.tracks[i].Hash]; ok {
				m.favorites.tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.favorites.level == favtui.TrackLevel {
		for i := range m.favorites.drilldownTracks {
			if ms, ok := fixMap[m.favorites.drilldownTracks[i].Hash]; ok {
				m.favorites.drilldownTracks[i].Duration = float64(ms) / 1000
			}
		}
		m.favorites = m.favorites.recomputeFavoritesSort()
	}
	if m.tagsView.level == tagtui.TrackLevel {
		for i := range m.tagsView.tracks {
			if ms, ok := fixMap[m.tagsView.tracks[i].Hash]; ok {
				m.tagsView.tracks[i].DurationMs = ms
			}
		}
		m.tagsView = m.tagsView.recomputeTagsSort()
	}
	if m.artists.level == arttui.TrackLevel {
		for i := range m.artists.tracks {
			if ms, ok := fixMap[m.artists.tracks[i].Hash]; ok {
				m.artists.tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.artists = m.artists.recomputeArtistsSort()
	}
	if m.albums.level == albtui.TrackLevel {
		for i := range m.albums.tracks {
			if ms, ok := fixMap[m.albums.tracks[i].Hash]; ok {
				m.albums.tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.albums = m.albums.recomputeAlbumsSort()
	}
	if m.genres.level == gentui.TrackLevel {
		for i := range m.genres.tracks {
			if ms, ok := fixMap[m.genres.tracks[i].Hash]; ok {
				m.genres.tracks[i].Duration = float64(ms) / 1000
			}
		}
		m.genres = m.genres.recomputeGenresSort()
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
		m.statusBar.notify(
			fmt.Sprintf("Duration audit: corrected %d %s", corrected,
				pluralize(corrected, "track", "tracks")),
		)
	}
	m.auditCh = nil
	m.auditCorrected = 0
	return m, nil
}
