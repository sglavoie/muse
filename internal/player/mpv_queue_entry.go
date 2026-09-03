package player

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/safego"
)

// SetStopAfterCurrent enables or disables "stop after current track" mode.
// When enabled, the next end-file EOF event fires OnPlaybackEnded instead of
// calling PlayFromIndex(idx+1). The flag auto-resets after firing.
func (m *Manager) SetStopAfterCurrent(v bool) {
	m.mu.Lock()
	m.stopAfterCurrent = v
	m.mu.Unlock()
	if v {
		m.clearPrefetch()
	} else {
		safego.Go("player.prefetchNext", m.prefetchNext)
	}
}

// StopAfterCurrent returns whether "stop after current track" mode is enabled.
func (m *Manager) StopAfterCurrent() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stopAfterCurrent
}

// PlayQueueEntry stats the file at entry.Path before loading it into mpv.
// If the file does not exist it marks the track and queue entry as missing via
// marker, emits a transient status message, and returns ErrMissingFile without
// touching mpv — the caller should skip to the next queue entry. Other stat
// failures are returned to the caller unchanged.
// If the file is present it sends a loadfile command to mpv (queued if not yet
// connected).
//
// It is the single-entry form of the queue playlist replacement and runs as a
// playlist transaction, so it cannot interleave with a prefetch append. It does
// not move the current queue index: the traversal entry points (PlayFromIndex,
// Next, Previous, auto-advance) own that bookkeeping and commit it inside the
// same transaction as the load.
func (m *Manager) PlayQueueEntry(entry QueueTrack, marker MissingMarker) error {
	m.playlistMu.Lock()
	defer m.playlistMu.Unlock()
	return m.replaceLocked(entry, marker, nil)
}

// markEntryMissing marks entry's track and queue row as missing through marker
// and emits the missing-file status message and callback. It is the shared side
// effect that PlayQueueEntry (Next-driven traversal) and the gapless-shuffle EOF
// commit path both invoke, so a file missing on disk produces identical durable
// markers and callbacks whether it is skipped by explicit traversal or by an
// EOF-driven gapless commit. The caller must NOT hold m.mu.
func (m *Manager) markEntryMissing(entry QueueTrack, marker MissingMarker) {
	if marker == nil {
		marker = noopMarker{}
	}
	if err := marker.MarkTrackMissing(entry.Hash); err != nil {
		slog.Error("mpv: mark track missing", "hash", entry.Hash, "err", err)
	}
	if err := marker.MarkQueueEntryMissing(entry.QueueID); err != nil {
		slog.Error("mpv: mark queue entry missing", "queueID", entry.QueueID, "err", err)
	}

	m.mu.RLock()
	fn := m.callbacks.OnStatus
	mfn := m.callbacks.OnMissingFile
	m.mu.RUnlock()
	msg := "Missing file: " + entry.Path
	if fn != nil {
		fn(msg)
	}
	if mfn != nil {
		mfn(entry.Path)
	}
	if fn == nil && mfn == nil {
		slog.Warn(msg)
	}
}
