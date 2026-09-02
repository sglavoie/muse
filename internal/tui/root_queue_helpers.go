package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/text"
)

// batchEnqueueTracksCmd appends tracks transactionally and returns the shared
// message consumed by the queue, player, autoplay, and undo handlers. source is
// persisted on each queue row; label is the user-facing origin shown by
// playback and undo state.
func (m Model) batchEnqueueTracksCmd(tracks []library.Track, source, label string) tea.Cmd {
	if m.batchEnqueuer == nil || len(tracks) == 0 {
		return nil
	}
	be := m.batchEnqueuer
	wasEmpty := len(m.queue.entries) == 0
	return func() tea.Msg {
		ids, err := be.AppendTracks(tracks, source)
		if err != nil {
			return BatchEnqueuedMsg{Err: err}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(tracks, ids, source),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: label,
		}
	}
}

// newQueueEntries builds QueueEntry values from matched tracks and DB row IDs.
// source is the playlist name to record on each entry (empty for individual enqueues).
func newQueueEntries(tracks []library.Track, ids []int64, source string) []QueueEntry {
	entries := make([]QueueEntry, len(tracks))
	for i, t := range tracks {
		entries[i] = QueueEntry{
			ID:             ids[i],
			Path:           t.Path,
			Hash:           t.Hash,
			Display:        text.TrackDisplay(t.Artist, t.Title, t.Path),
			Artist:         t.Artist,
			Title:          t.Title,
			Album:          t.Album,
			DurationMs:     int64(t.Duration * 1000),
			SourcePlaylist: source,
			TrackNumber:    t.TrackNumber,
			DiscNumber:     t.DiscNumber,
		}
	}
	return entries
}

// queueContainsHash reports whether any entry in entries has the given hash.
func queueContainsHash(entries []QueueEntry, hash string) bool {
	for _, e := range entries {
		if e.Hash == hash {
			return true
		}
	}
	return false
}

func queueSourceName(source string) string {
	if source == "" {
		return "Queue"
	}
	return source
}

func queueTrackFromEntry(entry QueueEntry) player.QueueTrack {
	return player.QueueTrack{
		Path:    entry.Path,
		Hash:    entry.Hash,
		QueueID: entry.ID,
		Artist:  entry.Artist,
		Title:   entry.Title,
		Source:  entry.SourcePlaylist,
	}
}

// clampQueueCursorAndScroll adjusts the queue cursor and scroll offset so they
// remain within bounds after entries have been removed. When a filter is active
// the cursor indexes into the filtered list, so we clamp against the filtered
// count rather than the full entry count.
func (m *Model) clampQueueCursorAndScroll() {
	n := len(m.queue.filteredEntries())
	m.queue.cursor, m.queue.scrollOffset = layout.ClampAfterRemoval(
		m.queue.cursor, m.queue.scrollOffset, n)
}

// resetPlaybackState stops the player and clears all playback-related fields
// in the model and status bar.
func (m Model) resetPlaybackState() Model {
	m = m.endCurrentListen()
	if cp, ok := m.player.(commandPlayer); ok {
		_ = cp.Stop() // stop before new playback, failure is non-fatal
	}
	m.playback.state = playback.StateIdle
	m = m.resetPlaybackProgress()
	m.playback.stopAfterCurrent = false
	m.playback.queueExhausted = false
	m.statusBar.Artist = ""
	m.statusBar.Title = ""
	m.statusBar.PlaylistSource = ""
	m.playback.currentHash = ""
	m.playback.currentIndex = -1
	m.missingTrackCount = 0
	if sat, ok := m.player.(stopAfterToggler); ok {
		sat.SetStopAfterCurrent(false)
	}
	return m
}

func (m Model) resetPlaybackProgress() Model {
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.playback.peakElapsedSecs = 0
	m.playback.peakTotalSecs = 0
	m.playback.listenedSecs = 0
	m.playback.lastSampleSecs = 0
	m.playback.durationCorrected = false
	return m
}

// playNextIndex returns the queue index where a "play next" track should be
// inserted: immediately after the currently playing track. When nothing is
// playing, it returns 0 (front of queue).
func (m Model) playNextIndex() int {
	if m.playback.currentIndex < 0 || m.playback.currentIndex >= len(m.queue.entries) {
		return 0
	}
	return m.playback.currentIndex + 1
}

func (m Model) resetQueueState() Model {
	m.queue.entries = nil
	m.queue.cursor = 0
	m.queue.scrollOffset = 0
	m.queue.filter = m.queue.filter.Deactivate()
	m.queue.savingAsPlaylist = false
	m.queue.savingFromSelection = false
	return m.resetQueueSort()
}

func (m *Model) adjustPlaybackIndexAfterRemoval(index int) {
	if index < m.playback.currentIndex {
		m.playback.currentIndex--
	} else if index == m.playback.currentIndex {
		m.playback.currentIndex = -1
	}
}

func (m *Model) adjustPlaybackIndexAfterRemovals(indices []int) {
	if m.playback.currentIndex < 0 {
		return
	}
	removedBefore := 0
	removedCurrent := false
	for _, index := range indices {
		if index < m.playback.currentIndex {
			removedBefore++
		} else if index == m.playback.currentIndex {
			removedCurrent = true
		}
	}
	if removedCurrent {
		m.playback.currentIndex = -1
		return
	}
	m.playback.currentIndex -= removedBefore
}

// snapshotTracks returns a shallow copy of the track slice so that a goroutine
// can safely iterate without racing against index mutations in the Update loop.
func snapshotTracks(src []library.Track) []library.Track {
	dst := make([]library.Track, len(src))
	copy(dst, src)
	return dst
}

// snapshotStringSet returns a copy of a map[string]struct{} for goroutine-safe reads.
func snapshotStringSet(src map[string]struct{}) map[string]struct{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]struct{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// snapshotTrackTags deep-copies the trackTags map (hash → tag-ID set) so that
// a goroutine can read it without racing against applyTagToggle in the Update loop.
func snapshotTrackTags(src map[string]map[int64]struct{}) map[string]map[int64]struct{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]map[int64]struct{}, len(src))
	for hash, inner := range src {
		cp := make(map[int64]struct{}, len(inner))
		for id, v := range inner {
			cp[id] = v
		}
		dst[hash] = cp
	}
	return dst
}

func (m Model) stopPlaybackIfCurrentMissing() Model {
	if len(m.queue.entries) == 0 {
		return m.resetPlaybackState()
	}
	if m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash) {
		return m.resetPlaybackState()
	}
	return m
}

// relocateCurrentIndexByHash re-points playback.currentIndex at the first
// entry whose Hash matches playback.currentHash, when the index was lost
// (set to -1) by a removal but the underlying track is still present in the
// queue under a different row. This happens during cut+paste of the playing
// track (the original row gets soft-deleted while the pasted clone — same
// hash, new ID — remains) and during selection-dequeue when duplicates of
// the playing hash exist elsewhere in the queue. Without this re-anchoring,
// the TUI would render no "now playing" marker even though the player is
// still streaming the file.
func (m *Model) relocateCurrentIndexByHash() {
	if m.playback.currentIndex >= 0 || m.playback.currentHash == "" {
		return
	}
	for i, entry := range m.queue.entries {
		if entry.Hash == m.playback.currentHash {
			m.playback.currentIndex = i
			return
		}
	}
}
