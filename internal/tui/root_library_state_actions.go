package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func (m Model) handleLibraryStateAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionRescan:
		if len(m.libraryRoots) > 0 && m.extractor != nil && !m.scanning {
			if m.userEditedReader != nil {
				rows, err := m.userEditedReader.ReadUserEditedTracks()
				switch {
				case err != nil:
					slog.Warn("rescan: read user-edited tracks", "err", err)
				case len(rows) > 0:
					overlay := make(map[string]db.UserEditedRow, len(rows))
					for _, r := range rows {
						overlay[r.Hash] = r
					}
					m.userEditedOverlay = overlay
				default:
					m.userEditedOverlay = nil
				}
			}

			tracks := m.library.index.Tracks()
			snap := make(map[string]rescanEntry, len(tracks))
			for _, t := range tracks {
				snap[t.Hash] = rescanEntry{CreatedAt: t.CreatedAt, Duration: t.Duration, PlayCount: t.PlayCount, LastPlayedAt: t.LastPlayedAt}
			}
			m.rescanSnapshot = snap

			rawCh := library.Scan(m.appCtx, m.libraryRoots, m.cfg.ExcludedPaths, m.extractor)
			ch := m.bridgeScanChannel(rawCh)
			m.library.index = library.LibraryIndex{}
			m.library.cursor = 0
			m.library.scrollOffset = 0
			m.library.filter = m.library.filter.Deactivate()
			m.library.filteredMatches = nil
			m.library.selectedHashes = nil
			m.library.sortState = SortState{}
			m.library.sortedTracks = nil
			m.recentlyAdded = newRecentlyAddedView()
			m.recentlyAdded.width = m.library.width
			m.recentlyAdded.height = m.library.height
			m.scanCh = ch
			m.scanning = true
			m.scanned = 0
			m.scanTotal = 0
			m.artists = ArtistsView{width: m.artists.width, height: m.artists.height}
			m.albums = AlbumsView{width: m.albums.width, height: m.albums.height}
			m.genres = GenresView{width: m.genres.width, height: m.genres.height}
			return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(m.appCtx, m.libraryRoots, m.cfg.ExcludedPaths)), true
		}
		return m, nil, true
	case actionLibraryFilterClear:
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.ClearFilter()
		case viewRecentlyAdded:
			m.recentlyAdded = m.recentlyAdded.ClearFilter()
		case viewQueue:
			if m.queue.filter.Text != "" {
				m.queue.filter = m.queue.filter.Deactivate()
				m.queue.cursor = 0
				m.queue.scrollOffset = 0
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
