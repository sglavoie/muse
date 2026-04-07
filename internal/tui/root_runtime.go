package tui

import (
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

// pluralize returns singular when n == 1, plural otherwise.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// scheduleSoftDelete cancels any in-flight purge timer from the previous undo
// slot, calls softDeleteFn to perform the soft-delete, then schedules a
// time.AfterFunc purge via purgeFn. Returns the cancel channel that the new
// undo slot should store.
func (m *Model) scheduleSoftDelete(softDeleteFn func(), purgeFn func(beforeUnix int64)) chan struct{} {
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
		m.undo.slot.cancelPurge = nil
	}
	cancel := make(chan struct{})
	softDeleteTime := time.Now().Unix()
	softDeleteFn()
	if purgeFn != nil {
		time.AfterFunc(undoWindowDuration, func() {
			select {
			case <-cancel:
				return
			default:
				purgeFn(softDeleteTime + 1)
			}
		})
	}
	return cancel
}

// schedulePurge sets up a deferred purge of soft-deleted queue entries.
// It closes any existing cancel channel on the slot, creates a new one,
// and schedules purgeFn to run after undoWindowDuration.
func schedulePurge(slot *undoSlot, purgeFn func(int64) error) {
	if slot.cancelPurge != nil {
		close(slot.cancelPurge)
	}
	cancel := make(chan struct{})
	slot.cancelPurge = cancel
	softDeleteTime := time.Now().Unix()
	time.AfterFunc(undoWindowDuration, func() {
		select {
		case <-cancel:
			return
		default:
			if err := purgeFn(softDeleteTime + 1); err != nil {
				slog.Warn("purge failed", "err", err)
			}
		}
	})
}

// syncPlayerQueue rebuilds the player's in-memory queue from the current TUI
// queue entries. Call this after any operation that reorders the TUI queue
// (e.g. MoveTrackUp/Down) so that the player's auto-advance resolves indices
// against the same ordering the user sees.
func (m Model) syncPlayerQueue() {
	if qs, ok := m.player.(playerQueueSetter); ok {
		pt := make([]player.QueueTrack, len(m.queue.entries))
		for i, e := range m.queue.entries {
			pt[i] = player.QueueTrack{
				Path:    e.Path,
				Hash:    e.Hash,
				QueueID: e.ID,
				Artist:  e.Artist,
				Title:   e.Title,
				Source:  e.SourcePlaylist,
			}
		}
		qs.SetQueue(pt)
	}
}

// setUndoSlot replaces the current undo slot and derives the status-bar
// notification from action.description(). cancelPurge may be nil for actions
// that do not schedule a soft-delete purge timer. Any previous slot's purge
// channel is closed automatically so callers need not manage its lifecycle.
func (m *Model) setUndoSlot(action undoAction, cancelPurge chan struct{}) {
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
	}
	m.undo.slot = undoSlot{
		action:      action,
		expiresAt:   time.Now().Add(undoWindowDuration),
		cancelPurge: cancelPurge,
	}
	m.statusBar.notification = newNotificationFor(action.description(), undoWindowDuration)
}

// syncPositionCmd schedules one position-sync tick.
func syncPositionCmd() tea.Cmd {
	return tea.Tick(positionSyncInterval, func(time.Time) tea.Msg {
		return syncPositionMsg{}
	})
}

// sessionPositionCmd returns a Cmd that writes the current track hash and
// elapsed position (in milliseconds) to the session table. Returns nil when
// no sessionWriter is configured.
func (m Model) sessionPositionCmd() tea.Cmd {
	if m.sessionWriter == nil {
		return nil
	}
	sw := m.sessionWriter
	hash := m.playback.currentHash
	positionMs := int64(m.playback.elapsedSecs * 1000)
	return func() tea.Msg {
		_ = sw.UpdateSessionPosition(hash, positionMs)
		return nil
	}
}

// sessionTickCmd schedules one periodic session-position tick.
func sessionTickCmd() tea.Cmd {
	return tea.Tick(sessionTickInterval, func(time.Time) tea.Msg {
		return sessionTickMsg{}
	})
}

// tickCmd schedules a 1-second tick for elapsed-time display updates.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// countAudioFiles returns a Cmd that runs library.CountAudioFiles in a
// background goroutine and dispatches a ScanTotalMsg when done.
func countAudioFiles(ctx context.Context, roots []string, excludedPaths []string) tea.Cmd {
	return func() tea.Msg {
		return library.ScanTotalMsg{Total: library.CountAudioFiles(ctx, roots, excludedPaths)}
	}
}

// waitForNextBatch returns a Cmd that reads up to scanBatchSize tracks from ch
// and dispatches a ScanBatchMsg. When the channel closes with no remaining
// tracks it dispatches ScanDoneMsg instead.
func waitForNextBatch(ch <-chan library.Track) tea.Cmd {
	return func() tea.Msg {
		var batch []library.Track
		for track := range ch {
			batch = append(batch, track)
			if len(batch) >= scanBatchSize {
				return library.ScanBatchMsg{Tracks: batch}
			}
		}
		if len(batch) > 0 {
			return library.ScanBatchMsg{Tracks: batch}
		}
		return library.ScanDoneMsg{}
	}
}

// waitForScanErr returns a Cmd that reads a single ScanErrorMsg from ch and
// dispatches it. The channel is already closed when Init runs so this Cmd
// terminates quickly; it is re-scheduled only while items remain.
func waitForScanErr(ch <-chan library.ScanErrorMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// waitForAuditBatch returns a Cmd that reads up to scanBatchSize fixes from
// the audit channel and dispatches a DurationAuditBatchMsg. When the channel
// closes with remaining fixes it dispatches them; on an empty close it
// dispatches DurationAuditDoneMsg to signal completion.
func waitForAuditBatch(ch <-chan library.DurationFix) tea.Cmd {
	return func() tea.Msg {
		var fixes []library.DurationFix
		for fix := range ch {
			fixes = append(fixes, fix)
			if len(fixes) >= scanBatchSize {
				return library.DurationAuditBatchMsg{Fixes: fixes}
			}
		}
		if len(fixes) > 0 {
			return library.DurationAuditBatchMsg{Fixes: fixes}
		}
		return library.DurationAuditDoneMsg{}
	}
}

// waitForWatchBatch returns a Cmd that reads up to scanBatchSize events from
// the watcher channel and dispatches a watchBatchMsg carrying both new/updated
// tracks and deleted paths. When the channel closes (context cancelled) it
// returns nil, stopping further scheduling.
func waitForWatchBatch(ch <-chan library.WatchEvent) tea.Cmd {
	return func() tea.Msg {
		var tracks []library.Track
		var deleted []string
		for event := range ch {
			if event.Deleted {
				deleted = append(deleted, event.Path)
			} else {
				tracks = append(tracks, event.Track)
			}
			if len(tracks)+len(deleted) >= scanBatchSize {
				return watchBatchMsg{tracks: tracks, deleted: deleted}
			}
		}
		if len(tracks)+len(deleted) > 0 {
			return watchBatchMsg{tracks: tracks, deleted: deleted}
		}
		return nil
	}
}

// Init satisfies the tea.Model interface. It schedules the first scan/watch
// readers, periodic playback/session ticks, and startup overlay refreshes.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.scanCh != nil {
		cmds = append(cmds, waitForNextBatch(m.scanCh))
		if len(m.libraryRoots) > 0 {
			cmds = append(cmds, countAudioFiles(m.appCtx, m.libraryRoots, m.cfg.ExcludedPaths))
		}
	}
	if m.scanErrCh != nil {
		cmds = append(cmds, waitForScanErr(m.scanErrCh))
	}
	if m.watchCh != nil {
		cmds = append(cmds, waitForWatchBatch(m.watchCh))
	}
	if m.auditCh != nil {
		cmds = append(cmds, waitForAuditBatch(m.auditCh))
	}
	if _, ok := m.player.(positionSyncer); ok {
		cmds = append(cmds, syncPositionCmd())
	}
	cmds = append(cmds, tickCmd())
	cmds = append(cmds, sessionTickCmd())
	cmds = append(cmds, tea.RequestBackgroundColor)
	if cmd := evalSmartPlaylistCountsCmd(m.playlists.playlists, m.smartEval); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := purgeEmptyTagsCmd(m.tags, m.tagReader, m.tagWriter); cmd != nil {
		cmds = append(cmds, cmd)
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}
