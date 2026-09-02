package tui

import (
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// syncRecentlyAddedFromLibrary refreshes the library view's source tracks when
// the Library is in Recent mode. It is a no-op in other modes (data is loaded
// lazily when the user switches to that mode).
func (m Model) syncRecentlyAddedFromLibrary() Model {
	if m.library.Mode != libtui.LibraryModeRecent {
		return m
	}
	m.library.SourceTracks = m.library.Index.Tracks()
	if m.library.Fuzzy == nil {
		m.library.Fuzzy = library.SahilmFuzzySearcher{}
	}
	if m.library.SortState.Field == sorting.SortDefault {
		m.library.SortState = sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortDesc}
	}
	m.library = m.library.RefreshView().ClampCursor()
	return m
}

func (m Model) handleLibraryStateAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionRescan:
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

			tracks := m.library.Index.Tracks()
			snap := make(map[string]rescanEntry, len(tracks))
			prior := make(map[string]string, len(tracks))
			for _, t := range tracks {
				snap[t.Hash] = rescanEntry{CreatedAt: t.CreatedAt, Duration: t.Duration, PlayCount: t.PlayCount, LastPlayedAt: t.LastPlayedAt}
				prior[t.Path] = t.Hash
			}
			m.rescanSnapshot = snap
			m.rescanPriorPaths = prior

			scanCtx, cancel := context.WithCancel(m.appCtx)
			rawCh, scanOutcome := library.Scan(scanCtx, m.libraryRoots, m.cfg.ExcludedPaths, m.extractor)
			ch := m.bridgeScanChannel(rawCh)
			m.scanOutcome = scanOutcome
			m.scanCancel = cancel
			m.library.Index = library.LibraryIndex{}
			m.library.Cursor = 0
			m.library.ScrollOffset = 0
			m.library.Filter = m.library.Filter.Deactivate()
			m.library.FilteredMatches = nil
			m.library.SelectedHashes = nil
			m.library.SortState = sorting.SortState{}
			m.library.SortedTracks = nil
			m.scanCh = ch
			m.scanning = true
			m.scanned = 0
			m.scanTotal = 0
			m.longOp = statusbar.LongOpState{Active: true, Label: "Scan", StartedAt: time.Now()}
			m.browse.Artists = browse.ArtistsView{Width: m.browse.Artists.Width, Height: m.browse.Artists.Height}
			m.browse.Albums = browse.AlbumsView{Width: m.browse.Albums.Width, Height: m.browse.Albums.Height}
			m.browse.Genres = browse.GenresView{Width: m.browse.Genres.Width, Height: m.browse.Genres.Height}
			return m, tea.Batch(waitForNextBatch(ch), countAudioFiles(scanCtx, m.libraryRoots, m.cfg.ExcludedPaths)), true
		}
		return m, nil, true
	case keys.ActionLibraryFilterClear:
		// Clearing a committed filter changes the visible list, so exit
		// visual mode to prevent the stale anchor from selecting wrong items.
		m = exitVisualMode(m)
		switch m.ActiveView {
		case viewLibrary:
			m.library = m.library.ClearFilter()
		case viewQueue:
			if m.queue.Filter.Text != "" {
				m.queue.Filter = m.queue.Filter.Deactivate()
				m.queue.Cursor = 0
				m.queue.ScrollOffset = 0
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
