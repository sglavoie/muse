package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// rescanEntry holds per-track values snapshotted before a full rescan clears
// the in-memory library index. Used to restore created_at, duration, and
// play stats when the scanner cannot provide them.
type rescanEntry struct {
	CreatedAt    int64
	Duration     float64
	PlayCount    int
	LastPlayedAt int64
}

// handleScanProgressMsg updates the status bar with the current scan progress.
func (m Model) handleScanProgressMsg(msg library.ScanProgressMsg) Model {
	m.statusBar.handleScanProgress(msg)
	return m
}

// handleScanTotalMsg records the expected total and updates the progress display.
func (m Model) handleScanTotalMsg(msg library.ScanTotalMsg) Model {
	m.scanTotal = msg.Total
	if m.scanning {
		m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
	}
	return m
}

// handleScanBatchMsg inserts scanned tracks into the library index and refreshes
// derived views (artists, albums) as needed.
func (m Model) handleScanBatchMsg(msg library.ScanBatchMsg) (Model, tea.Cmd) {
	for _, t := range msg.Tracks {
		t = m.preserveTrackMetadata(t)
		if edited, ok := m.userEditedOverlay[t.Hash]; ok {
			t.Title = edited.Title
			t.Artist = edited.Artist
			t.Album = edited.Album
			t.Genre = edited.Genre
		}
		m.library.index.Insert(t)
	}
	m.scanned += len(msg.Tracks)
	m.statusBar.handleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
	// Recompute sorted tracks if a sort is active so the cached copy
	// includes newly scanned tracks.
	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	m = m.syncRecentlyAddedFromLibrary()
	// Invalidate the cached fuzzy-match results so that stale TrackIndex
	// values from the previous index state cannot cause an out-of-bounds
	// panic when the filter is active and the user navigates or renders.
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	m = m.refreshBrowseDerivedViews()
	if m.scanCh != nil {
		return m, waitForNextBatch(m.scanCh)
	}
	return m, nil
}

// handleScanDoneMsg clears the scanning state.
func (m Model) handleScanDoneMsg() Model {
	m.scanning = false
	m.scanCh = nil
	m.rescanSnapshot = nil
	return m
}

// bridgeScanChannel wraps raw with a goroutine that persists each track to the
// database via m.scanWriter before forwarding it. When no scanWriter is
// configured, the raw channel is returned as-is.
func (m Model) bridgeScanChannel(raw <-chan library.Track) <-chan library.Track {
	if m.scanWriter == nil {
		return raw
	}
	ch := make(chan library.Track, 256)
	go func() {
		defer close(ch)
		batch := make([]library.Track, 0, m.upsertBatchSize)
		type genrePopulator interface {
			PopulateTrackGenres([]library.Track) error
		}
		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := m.scanWriter.UpsertBatch(batch); err != nil {
				slog.Warn("rescan: batch upsert", "err", err, "count", len(batch))
			}
			if gp, ok := m.scanWriter.(genrePopulator); ok {
				if err := gp.PopulateTrackGenres(batch); err != nil {
					slog.Warn("rescan: batch genre populate", "err", err, "count", len(batch))
				}
			}
			batch = batch[:0]
		}
		for t := range raw {
			ch <- t
			batch = append(batch, t)
			if len(batch) >= m.upsertBatchSize {
				flush()
			}
		}
		flush()
	}()
	return ch
}

func (m Model) preserveTrackMetadata(track library.Track) library.Track {
	// Try the live index first (works for watcher-driven inserts where the
	// index has not been cleared).
	if existing, ok := m.library.index.FindByHash(track.Hash); ok {
		if existing.CreatedAt != 0 {
			track.CreatedAt = existing.CreatedAt
		}
		if track.Duration == 0 && existing.Duration != 0 {
			track.Duration = existing.Duration
		}
		if existing.PlayCount > track.PlayCount {
			track.PlayCount = existing.PlayCount
		}
		if existing.LastPlayedAt > track.LastPlayedAt {
			track.LastPlayedAt = existing.LastPlayedAt
		}
		return track
	}
	if existing, ok := m.library.index.FindByPath(track.Path); ok {
		if existing.CreatedAt != 0 {
			track.CreatedAt = existing.CreatedAt
		}
		if track.Duration == 0 && existing.Duration != 0 {
			track.Duration = existing.Duration
		}
		if existing.PlayCount > track.PlayCount {
			track.PlayCount = existing.PlayCount
		}
		if existing.LastPlayedAt > track.LastPlayedAt {
			track.LastPlayedAt = existing.LastPlayedAt
		}
		return track
	}
	// Fall back to the pre-rescan snapshot (index was cleared for a full
	// rescan but we captured these values beforehand).
	if entry, ok := m.rescanSnapshot[track.Hash]; ok {
		if entry.CreatedAt != 0 {
			track.CreatedAt = entry.CreatedAt
		}
		if track.Duration == 0 && entry.Duration != 0 {
			track.Duration = entry.Duration
		}
		if entry.PlayCount > track.PlayCount {
			track.PlayCount = entry.PlayCount
		}
		if entry.LastPlayedAt > track.LastPlayedAt {
			track.LastPlayedAt = entry.LastPlayedAt
		}
	}
	if track.CreatedAt == 0 {
		track.CreatedAt = time.Now().Unix()
	}
	return track
}
