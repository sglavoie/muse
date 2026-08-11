package player

import (
	"errors"
	"log/slog"
	"os"
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
		go m.prefetchNext()
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
func (m *Manager) PlayQueueEntry(entry QueueTrack, marker MissingMarker) error {
	if marker == nil {
		marker = noopMarker{}
	}
	if _, err := os.Stat(entry.Path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		m.markEntryMissing(entry, marker)
		return ErrMissingFile
	}

	return m.admit(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdLoadfile, entry.Path, "replace", 0, "pause=no"); err != nil {
			return err
		}
		m.bestEffortUnpauseWith(c)
		return nil
	})
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
