package tui

import (
	"context"
	"log/slog"
	"time"

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
			prior := make(map[string]string, len(tracks))
			for _, t := range tracks {
				snap[t.Hash] = rescanEntry{CreatedAt: t.CreatedAt, Duration: t.Duration, PlayCount: t.PlayCount, LastPlayedAt: t.LastPlayedAt}
				prior[t.Path] = t.Hash
			}
			m.rescanSnapshot = snap
			m.rescanPriorPaths = prior

			scanCtx, cancel := context.WithCancel(m.appCtx)
			rawCh := library.Scan(scanCtx, m.libraryRoots, m.cfg.ExcludedPaths, m.extractor)
			ch := m.bridgeScanChannel(rawCh)
			m.scanCancel = cancel
			m.library.index = library.LibraryIndex{}
			m.library.cursor = 0
			m.library.scrollOffset = 0
			m.library.filter = m.library.filter.Deactivate()
			m.library.filteredMatches = nil
			m.library.selectedHashes = nil
			m.library.sortState = SortState{}
			m.library.sortedTracks = nil
			m.scanCh = ch
			m.scanning = true
			m.scanned = 0
			m.scanTotal = 0
			m.longOp = longOpState{Active: true, Label: "Scan", StartedAt: time.Now()}
			m.browse.artists = ArtistsView{width: m.browse.artists.width, height: m.browse.artists.height}
			m.browse.albums = AlbumsView{width: m.browse.albums.width, height: m.browse.albums.height}
			m.browse.genres = GenresView{width: m.browse.genres.width, height: m.browse.genres.height}
			return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(scanCtx, m.libraryRoots, m.cfg.ExcludedPaths)), true
		}
		return m, nil, true
	case actionLibraryFilterClear:
		// Clearing a committed filter changes the visible list, so exit
		// visual mode to prevent the stale anchor from selecting wrong items.
		m = exitVisualMode(m)
		switch m.activeView {
		case viewLibrary:
			m.library = m.library.ClearFilter()
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
