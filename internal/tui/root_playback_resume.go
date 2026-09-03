package tui

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// This file holds the single path every natural queue extension takes back into
// playback: the Auto-DJ refill, a single or batch enqueue onto a dry queue, a
// paste, and a queue that grew while PlaybackEndedMsg was in flight. They all
// mean the same thing — the cycle that was running is not finished after all —
// so they all continue it through ResumeAfterQueueExtension rather than
// reanchoring it with PlayFromIndex. An explicit PlayFromIndex would re-permute
// the shuffle cycle around the appended row and replay entries already heard.
//
// Starting an empty queue is deliberately not routed through here: there is no
// cycle to continue, so those paths keep anchoring with PlayFromIndex.

// playerTraversal returns the player's traversal snapshot and whether the player
// exposes one at all. Lightweight players (and most test stubs) implement only
// playPauser, so every caller needs a visible-order fallback.
func (m Model) playerTraversal() (player.QueueTraversal, bool) {
	reader, ok := m.player.(queueTraversalReader)
	if !ok {
		return player.QueueTraversal{}, false
	}
	return reader.QueueTraversal(), true
}

// queueExtensionResumable reports whether the traversal still has a scheduled
// row ahead of the entry that just ended — the signal that the queue was
// extended while playback was ending, rather than genuinely running out.
//
// The visible-index fallback is what this replaces for real players: with
// shuffle on, "current index plus one is in range" describes the durable row
// order, not the order the Manager will actually visit, so it both misses live
// rows and invents ones the cycle has already consumed.
func (m Model) queueExtensionResumable() bool {
	if t, ok := m.playerTraversal(); ok {
		return t.Remaining > 0
	}
	return m.playback.currentIndex >= 0 && m.playback.currentIndex+1 < len(m.queue.Entries)
}

// resumeExtendedQueue continues playback after the queue grew, reporting whether
// it started. fallbackIndex is the queue index to anchor from when the player
// does not implement queueExtensionResumer; it is ignored otherwise, because the
// row the traversal lands on is the player's decision (sequential traversal skips
// missing files, shuffle traversal follows identities rather than positions).
//
// On success playback state and the status bar are filled from the row that
// actually started, and queueExhausted is cleared. On failure the caller's rows
// stay in the queue, the model stays idle and still exhausted so the next
// extension can retry, and the user is told; the caller must not respond by
// scheduling another enqueue.
func (m Model) resumeExtendedQueue(fallbackIndex int) (Model, bool) {
	if m.player == nil {
		return m, false
	}
	idx, err := m.resumeExtendedQueueIndex(fallbackIndex)
	if err != nil {
		slog.Warn("queue extension resume failed", "err", err, "fallback_index", fallbackIndex)
		m.statusBar.Notify("Resume failed: " + err.Error())
		player.PlaySound(player.SoundError)
		return m, false
	}

	m.playback.state = playback.StatePlaying
	m = m.resetPlaybackProgress()
	m.playback.queueExhausted = false

	if idx < 0 || idx >= len(m.queue.Entries) {
		// The player resumed onto a row this model cannot name yet (its queue
		// grew from a source the TUI has not merged in). Leave the metadata to
		// the TrackChangedMsg that follows rather than describing the wrong row.
		return m, true
	}
	entry := m.queue.Entries[idx]
	m.playback.currentHash = entry.Hash
	m.playback.currentIndex = idx
	m.statusBar.Artist = entry.Artist
	m.statusBar.Title = entry.Title
	m.statusBar.PlaylistSource = queueSourceName(entry.SourcePlaylist)
	return m, true
}

// resumeExtendedQueueIndex performs the resume itself and returns the queue
// index that started playing. The PlayFromIndex fallback can only report the
// index it was handed: a player without the extension has no traversal to ask.
func (m Model) resumeExtendedQueueIndex(fallbackIndex int) (int, error) {
	if resumer, ok := m.player.(queueExtensionResumer); ok {
		return resumer.ResumeAfterQueueExtension()
	}
	if err := m.player.PlayFromIndex(fallbackIndex); err != nil {
		return -1, err
	}
	return fallbackIndex, nil
}
