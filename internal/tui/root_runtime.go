package tui

import (
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
)

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

// scheduleStandalonePurge schedules a deferred hard-delete of soft-deleted
// rows that are NOT tied to an undo slot (e.g. auto-deleted tags). Unlike
// schedulePurge, it does not touch the undo slot's cancel channel, so it
// cannot corrupt an unrelated undo operation's purge timer.
func scheduleStandalonePurge(purgeFn func(int64) error) {
	softDeleteTime := time.Now().Unix()
	time.AfterFunc(undoWindowDuration, func() {
		if err := purgeFn(softDeleteTime + 1); err != nil {
			slog.Warn("standalone purge failed", "err", err)
		}
	})
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
// queue entries and updates the player's current index to match the TUI's
// playback index. Call this after any operation that reorders the TUI queue
// (e.g. MoveTrackUp/Down, shuffle, dedup) so that the player's auto-advance
// resolves indices against the same ordering the user sees.
func (m Model) syncPlayerQueue() {
	if qs, ok := m.player.(playerQueueSetter); ok {
		pt := make([]player.QueueTrack, len(m.queue.Entries))
		for i, e := range m.queue.Entries {
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
	// Keep the player's current index in sync so that auto-advance after
	// shuffle, dedup, move, or paste uses the correct position.
	//
	// A negative index is not a position, it is the absence of one: the TUI
	// clears it when the queue runs dry or when the playing row is removed. The
	// player clamps negatives to 0, so pushing it would move the player's
	// traversal back to the head of the queue and make the next resume replay a
	// cycle that already finished. Leave the player's own last position alone
	// and let the resume path continue from it.
	if is, ok := m.player.(playerIndexSetter); ok && m.playback.currentIndex >= 0 {
		is.SetCurrentIndex(m.playback.currentIndex)
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
	m.statusBar.NotifyUndo(action.description(), m.undo.slot.expiresAt)
}

// syncPositionCmd schedules one position-sync tick.
func syncPositionCmd() tea.Cmd {
	return tea.Tick(playback.PositionSyncInterval, func(time.Time) tea.Msg {
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
	return tea.Tick(playback.SessionTickInterval, func(time.Time) tea.Msg {
		return sessionTickMsg{}
	})
}

// tickCmd schedules a 1-second tick for elapsed-time display updates.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// pulseDuration is the on-screen lifetime of the now-playing pulse after a
// track change.
const pulseDuration = 250 * time.Millisecond

// pulseCmd schedules a tick that fires when the now-playing pulse window
// expires so the View() rebuilds with normal (non-inverted) styling.
func pulseCmd() tea.Cmd {
	return tea.Tick(pulseDuration, func(time.Time) tea.Msg {
		return pulseTickMsg{}
	})
}

// notificationFadeCmd schedules a tick that fires at the given absolute time
// so the View() rebuilds with the dim "fade" frame for an expiring
// notification. Returns nil when at is zero or already in the past.
func notificationFadeCmd(at time.Time) tea.Cmd {
	if at.IsZero() {
		return nil
	}
	d := time.Until(at)
	if d <= 0 {
		return nil
	}
	return tea.Tick(d, func(time.Time) tea.Msg {
		return notificationFadeMsg{}
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
// dispatches it, rescheduling itself on each message so the subscription
// survives the first error.
//
// It blocks for as long as the background pipeline lives: the stream stays open
// past the initial scan so the watcher forwarder can still report a failure
// mid-session. Shutdown closes it, and the closed channel yields nil.
func waitForScanErr(ch <-chan library.ScanErrorMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// waitForWatchStatus returns a Cmd that blocks for the single watcher
// availability report emitted by the background library pipeline once its
// asynchronous watcher setup completes, then dispatches it. The channel emits
// exactly one WatchStatusMsg and is then closed, so this Cmd is not
// re-scheduled; a closed channel with no message yields nil.
func waitForWatchStatus(ch <-chan library.WatchStatusMsg) tea.Cmd {
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

// waitForWatchBatch returns a Cmd that blocks for the first watcher event, then
// non-blockingly drains any already-buffered companion events up to the
// scanBatchSize cap before dispatching a single watchBatchMsg. This delivers a
// lone event on a still-open channel immediately instead of waiting for the cap
// to fill. When the channel closes (context cancelled) with no pending event it
// returns nil, stopping further scheduling.
func waitForWatchBatch(ch <-chan library.WatchEvent) tea.Cmd {
	return func() tea.Msg {
		var tracks []library.Track
		var deleted []string
		add := func(event library.WatchEvent) {
			if event.Deleted {
				deleted = append(deleted, event.Path)
			} else {
				tracks = append(tracks, event.Track)
			}
		}

		// Block for the first event so an open, idle channel does not busy-wait.
		event, ok := <-ch
		if !ok {
			return nil
		}
		add(event)

		// Drain buffered companions without blocking, up to the cap.
		for len(tracks)+len(deleted) < scanBatchSize {
			select {
			case event, ok := <-ch:
				if !ok {
					return watchBatchMsg{tracks: tracks, deleted: deleted}
				}
				add(event)
			default:
				return watchBatchMsg{tracks: tracks, deleted: deleted}
			}
		}
		return watchBatchMsg{tracks: tracks, deleted: deleted}
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
	if m.watchStatusCh != nil {
		cmds = append(cmds, waitForWatchStatus(m.watchStatusCh))
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
	if cmd := evalSmartPlaylistCountsCmd(m.playlists.Playlists, m.smartEval); cmd != nil {
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
