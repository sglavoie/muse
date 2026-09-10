package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/statusbar"
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
	m.statusBar.HandleScanProgress(msg)
	if m.longOp.Active {
		m.longOp = m.longOp.Update(msg.Scanned, msg.Total, time.Now())
	}
	return m
}

// handleScanTotalMsg records the expected total and updates the progress display.
func (m Model) handleScanTotalMsg(msg library.ScanTotalMsg) Model {
	m.scanTotal = msg.Total
	if m.scanning {
		m.statusBar.HandleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
		if m.longOp.Active {
			m.longOp = m.longOp.Update(m.scanned, m.scanTotal, time.Now())
		}
	}
	return m
}

// handleScanBatchMsg inserts scanned tracks into the library index and refreshes
// derived views (artists, albums) as needed.
func (m Model) handleScanBatchMsg(msg library.ScanBatchMsg) (Model, tea.Cmd) {
	// Normalize each track once (preserved metadata + user-edited overlay) and
	// reuse the same normalized slice for both the index insertion and every
	// cache reconciliation target, so the caches never diverge from the index.
	normalized := make([]library.Track, len(msg.Tracks))
	var displacedHashes []string
	for i, t := range msg.Tracks {
		t = m.normalizeScannedTrack(t)
		normalized[i] = t
		if t.Path == "" {
			// Path-less entries cannot be reconciled by path; dedup by hash
			// only, matching the prior Insert behavior. Real scanned files
			// always carry a path, so the replacement handling below applies to
			// every genuine on-disk discovery.
			m.library.Index.Insert(t)
			continue
		}
		// Detect an in-place content replacement: a *different* hash currently
		// occupies this exact path. The displaced identity's file is gone (its
		// content was overwritten on disk), so record its hash and replace the
		// stale index entry via UpsertByPath instead of Insert — Insert dedups
		// only by hash and would leave both identities at one path, showing the
		// old and new content for the same file.
		if old, ok := m.library.Index.FindByPath(t.Path); ok && old.Hash != "" && old.Hash != t.Hash {
			displacedHashes = append(displacedHashes, old.Hash)
		}
		m.library.Index.UpsertByPath(t.Path, t)
	}
	m.scanned += len(msg.Tracks)
	m.statusBar.HandleScanProgress(library.ScanProgressMsg{Scanned: m.scanned, Total: m.scanTotal})
	if m.longOp.Active {
		m.longOp = m.longOp.Update(m.scanned, m.scanTotal, time.Now())
	}
	// Recompute sorted tracks if a sort is active so the cached copy
	// includes newly scanned tracks.
	if m.library.SortState.Field != sorting.SortDefault {
		m.library = m.library.RecomputeSortedTracks()
	}
	m = m.syncRecentlyAddedFromLibrary()
	// Heal path and metadata for every cache holding a playable Path so that
	// the startup diff scan and manual rescan reconcile same-hash moves/renames
	// exactly like the live watcher does. Use the normalized slice — not the raw
	// scanned tracks — so queue and collection caches receive the same preserved
	// duration and user-edited metadata that landed in the index.
	m = m.reconcileTrackCaches(normalized)
	// A content replacement discovered by the scan leaves the displaced hash's
	// queue instances pointing at a path that now holds different content. The
	// reconcile fan-out above only heals same-hash entries, so those stale
	// instances are untouched by it; mark them unavailable so they cannot play
	// the replacement under stale metadata. Durable tracks.missing marking for
	// the displaced identity is done by the startup app scan bridge.
	for _, h := range displacedHashes {
		m.queue.MarkMissingByHash(h)
	}
	// Invalidate the cached fuzzy-match results so that stale TrackIndex
	// values from the previous index state cannot cause an out-of-bounds
	// panic when the filter is active and the user navigates or renders.
	if m.library.Filter.Text != "" {
		m.library = m.library.RebuildFilterCache()
	}
	m = m.refreshBrowseDerivedViews()
	// The batch changed the candidate set itself: new tracks, replaced content
	// at a path, and healed metadata all move what Auto-DJ may select and how
	// its filters classify it.
	m.djInvalidate()
	if m.scanCh != nil {
		return m, waitForNextBatch(m.scanCh)
	}
	return m, nil
}

// handleScanDoneMsg clears the scanning state. On a normally-completed rescan
// it first reconciles files that vanished from disk (present before the rescan,
// absent from the rebuilt index) so the upsert-only scan path still honors the
// deletion contract the live watcher provides.
func (m Model) handleScanDoneMsg() (Model, tea.Cmd) {
	var cmd tea.Cmd
	// A hash worker that panicked closes the scan channel exactly like a
	// finished one, so the ScanDoneMsg alone cannot be trusted as "the whole
	// library was rescanned". Files it never hashed are absent from the rebuilt
	// index but still on disk, and reconciling deletions from that would mark
	// them missing. Report the truncation and leave the deletion step alone.
	truncated := m.scanOutcome.Err()
	if truncated != nil {
		slog.Error("rescan incomplete: skipping deletion reconciliation", "err", truncated)
		m.statusBar.Notify("Rescan incomplete: some files could not be hashed; deletions were not reconciled")
	}
	// A canceled scan clears rescanPriorPaths before this message lands, and a
	// normal completion still has m.scanning == true (only this handler flips it
	// false). Both guards ensure deletions are computed only from a full,
	// intact scan.
	if truncated == nil && m.scanning && len(m.rescanPriorPaths) > 0 {
		m, cmd = m.reconcileRescanDeletions()
	}
	m.scanning = false
	m.scanCh = nil
	m.scanOutcome = nil
	m.rescanSnapshot = nil
	m.rescanPriorPaths = nil
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	m.longOp = statusbar.LongOpState{}
	// A scan moves the coverage denominator even when it was canceled partway:
	// every batch it did deliver is already in the index and the database.
	m.insightsInvalidate()
	// Deletion reconciliation runs above, after the last batch was already
	// applied, so this is the bump that covers the tracks a rescan removed from
	// the candidate set.
	m.djInvalidate()
	return m, cmd
}

// reconcileRescanDeletions compares the paths present before a full rescan
// against the freshly rebuilt index and routes any file that disappeared
// through the same deletion reconciliation the live watcher uses: it drops the
// path from the index and marks every matching queue instance unavailable, then
// returns a Cmd that persists the missing flag by hash. A prior path whose hash
// resurfaced at a new location is a move/rename (already healed by the scan
// batch), not a deletion, so it is skipped.
func (m Model) reconcileRescanDeletions() (Model, tea.Cmd) {
	var deletedPaths []string
	var deletedHashes []string
	for path, hash := range m.rescanPriorPaths {
		if _, ok := m.library.Index.FindByPath(path); ok {
			continue // still on disk under the same path
		}
		if _, ok := m.library.Index.FindByHash(hash); ok {
			continue // same content moved to a new path — not a deletion
		}
		deletedPaths = append(deletedPaths, path)
		deletedHashes = append(deletedHashes, hash)
	}
	if len(deletedPaths) == 0 {
		return m, nil
	}
	// In-memory: drop from the index and mark queued instances missing via the
	// same helper the live watcher path uses.
	m = m.watchApplyDeletes(deletedPaths)
	// Durable: persist the missing mark so the deletion survives the next cached
	// startup. bridgeScanChannel flushes every present upsert before closing its
	// channel (which is what produced this ScanDoneMsg), so no later upsert can
	// clear these marks.
	marker, ok := m.scanWriter.(trackMissingMarker)
	if !ok {
		return m, nil
	}
	hashes := deletedHashes
	// Coverage counts what the database still holds, so the reload has to wait
	// for these marks; the command reports no completion of its own.
	pending := m.insightsPending
	pending.begin()
	return m, func() tea.Msg {
		defer pending.done()
		for _, h := range hashes {
			if err := marker.MarkTrackMissing(h); err != nil {
				slog.Warn("rescan: mark track missing", "hash", h, "err", err)
			}
		}
		return nil
	}
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

// normalizeScannedTrack applies the same metadata normalization the index
// insertion relies on: it preserves created_at, non-zero duration, and play
// stats from the existing index/snapshot, then overlays any user-edited
// title/artist/album/genre and track/disc number. Both the scan and watcher
// fan-outs use this so the index and every reconciled cache observe identical
// values.
func (m Model) normalizeScannedTrack(track library.Track) library.Track {
	track = m.preserveTrackMetadata(track)
	if edited, ok := m.userEditedOverlay[track.Hash]; ok {
		track.Title = edited.Title
		track.Artist = edited.Artist
		track.Album = edited.Album
		track.Genre = edited.Genre
		track.TrackNumber = edited.TrackNumber
		track.DiscNumber = edited.DiscNumber
	}
	return track
}

func (m Model) preserveTrackMetadata(track library.Track) library.Track {
	// Try the live index first (works for watcher-driven inserts where the
	// index has not been cleared).
	if existing, ok := m.library.Index.FindByHash(track.Hash); ok {
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
	if existing, ok := m.library.Index.FindByPath(track.Path); ok {
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
