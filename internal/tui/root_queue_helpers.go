package tui

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

// newQueueEntries builds QueueEntry values from matched tracks and DB row IDs.
// source is the playlist name to record on each entry (empty for individual enqueues).
func newQueueEntries(tracks []library.Track, ids []int64, source string) []QueueEntry {
	entries := make([]QueueEntry, len(tracks))
	for i, t := range tracks {
		entries[i] = QueueEntry{
			ID:             ids[i],
			Path:           t.Path,
			Hash:           t.Hash,
			Display:        trackDisplay(t.Artist, t.Title, t.Path),
			Artist:         t.Artist,
			Title:          t.Title,
			Album:          t.Album,
			DurationMs:     int64(t.Duration * 1000),
			SourcePlaylist: source,
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
	m.queue.cursor, m.queue.scrollOffset = clampAfterRemoval(
		m.queue.cursor, m.queue.scrollOffset, n)
}

// resetPlaybackState stops the player and clears all playback-related fields
// in the model and status bar.
func (m Model) resetPlaybackState() Model {
	if cp, ok := m.player.(commandPlayer); ok {
		_ = cp.Stop() // stop before new playback, failure is non-fatal
	}
	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.playback.peakElapsedSecs = 0
	m.playback.peakTotalSecs = 0
	m.playback.durationCorrected = false
	m.playback.stopAfterCurrent = false
	m.playback.queueExhausted = false
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.playback.currentHash = ""
	m.playback.currentIndex = -1
	m.missingTrackCount = 0
	if sat, ok := m.player.(stopAfterToggler); ok {
		sat.SetStopAfterCurrent(false)
	}
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

func (m Model) stopPlaybackIfCurrentMissing() Model {
	if len(m.queue.entries) == 0 {
		return m.resetPlaybackState()
	}
	if m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash) {
		return m.resetPlaybackState()
	}
	return m
}
