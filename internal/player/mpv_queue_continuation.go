package player

import (
	"errors"
	"fmt"
)

// This file holds the two continuation operations: replaying what is already
// playing, and picking traversal back up after the queue grew. Both exist
// because the obvious implementation — route through PlayFromIndex — reanchors
// the shuffle cycle, which throws away the played history and re-permutes
// everything that is left. Neither operation is a new selection, so neither may
// start a new cycle.

// ReplayCurrent restarts the entry that is playing now from the beginning
// without moving the playback timeline: shuffle history, the cursor, the unseen
// pool, and the cycle exclusions all survive, and the successor the prefetch had
// resolved is resolved again to the same entry.
//
// It goes through the serialized replacement primitive rather than a seek, so
// the replacement and the gapless successor mpv holds cannot be observed apart:
// the replace resets mpv's playlist to the replayed entry alone, and the
// transaction's follow-up prefetch appends the successor again.
//
// Returns ErrPlaybackFailed when nothing is currently playing, and
// ErrMissingFile (after marking it) when the current entry has vanished from
// disk. In both cases the traversal is left exactly as it was.
func (m *Manager) ReplayCurrent() error {
	_, err := m.runPlaylistTxnIndex(m.replayCurrentLocked)
	return err
}

// replayCurrentLocked is ReplayCurrent's body inside an open playlist
// transaction.
//
// The caller must hold m.playlistMu.
func (m *Manager) replayCurrentLocked() (int, error) {
	m.mu.RLock()
	idx := m.currentIndex
	queueLen := len(m.queue)
	var entry QueueTrack
	if idx >= 0 && idx < queueLen {
		entry = m.queue[idx]
	}
	marker := m.marker
	m.mu.RUnlock()

	if idx < 0 || idx >= queueLen {
		return -1, fmt.Errorf("%w: no current entry to replay", ErrPlaybackFailed)
	}

	// Commit the same index the replacement is loading. It re-asserts nothing
	// under normal conditions, but an EOF promotion racing this replay can move
	// currentIndex onto a prefetched entry under m.mu alone while the load is in
	// flight; mpv is playing what this replacement loaded, so this is the commit
	// that describes mpv.
	if err := m.replaceLocked(entry, marker, func() func() { return m.commitIndexLocked(idx) }); err != nil {
		return -1, err
	}
	return idx, nil
}

// ResumeAfterQueueExtension continues the existing traversal after the queue has
// grown — the Auto-DJ refill and any append onto an exhausted queue. It returns
// the queue index that actually started playing, which is not derivable from the
// request: sequential traversal skips entries missing on disk, and shuffle
// traversal follows identities rather than positions.
//
// It advances the cycle that is already in progress instead of starting a new
// one: under shuffle it takes the next identity from forward history and then
// the unseen pool (into which reconciliation has added the new rows), and with
// shuffle off it plays the entry after the current index. Nothing here reanchors
// the way an explicit PlayFromIndex would, so a completed cycle stays completed
// and the newly appended entries are the only ones still eligible.
//
// Returns an error wrapping ErrPlaybackFailed when the extension added nothing
// playable — the queue is exhausted again.
func (m *Manager) ResumeAfterQueueExtension() (int, error) {
	return m.runPlaylistTxnIndex(m.resumeAfterQueueExtensionLocked)
}

// resumeAfterQueueExtensionLocked is ResumeAfterQueueExtension's body inside an
// open playlist transaction.
//
// The caller must hold m.playlistMu.
func (m *Manager) resumeAfterQueueExtensionLocked() (int, error) {
	m.mu.RLock()
	shuffle := m.shuffleEnabled
	m.mu.RUnlock()

	if shuffle {
		return m.advanceForwardLocked()
	}
	return m.resumeSequentialLocked()
}

// resumeSequentialLocked continues a shuffle-off traversal after the queue grew.
//
// The rows that grew the queue are not always after the cursor: a paste or a
// "play next" inserts at the cursor, and on an exhausted queue that is at or
// before the row that finished. Those rows are attempted first, in queue order,
// because currentIndex+1 would step past them and land on an already-played row
// or on nothing at all. Only then does the positional scan run, which is what
// every tail append still takes.
//
// A row whose file has vanished is dropped from the additions rather than
// retried, mirroring the way the shuffle cycle excludes what it gave up on. When
// nothing at all is playable the positional scan reports the traversal exhausted
// as before.
//
// The caller must hold m.playlistMu.
func (m *Manager) resumeSequentialLocked() (int, error) {
	m.mu.RLock()
	seqNext := m.currentIndex + 1
	queue := m.queue
	marker := m.marker
	pending := m.unreachedAdditionsLocked(seqNext)
	m.mu.RUnlock()

	for _, add := range pending {
		err := m.replaceLocked(queue[add.index], marker, func() func() { return m.commitIndexLocked(add.index) })
		if err == nil {
			return add.index, nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return -1, err
		}
		m.mu.Lock()
		m.forgetAdditionLocked(add.identity)
		m.mu.Unlock()
	}
	return m.playSequentialFromLocked(seqNext)
}
