package tui

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

type queueReplacer interface {
	ReplaceQueue([]int64, []library.Track, string) ([]int64, error)
	UndoReplaceQueue([]int64, []int64) error
}

type queueReplaceRequestedMsg struct {
	tracks   []library.Track
	source   string
	view     viewID
	selected bool
}

func queueEntryIDs(entries []queue.Entry) []int64 {
	ids := make([]int64, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}

// Replacement commits on the update loop, like undo, so no queue gesture can
// race the model snapshot. The writer also checks for unacknowledged appends.
func (m Model) handleQueueReplace(msg queueReplaceRequestedMsg) Model {
	m.queueReplacementPending = false
	writer, ok := m.queueWriter.(queueReplacer)
	if !ok || len(msg.tracks) == 0 {
		return m
	}
	old := m.queue
	old.Entries = slices.Clone(old.Entries)
	var ids []int64
	err := m.withStableQueuePlayback(func() error {
		var err error
		ids, err = writer.ReplaceQueue(queueEntryIDs(old.Entries), msg.tracks, msg.source)
		return err
	})
	if err != nil {
		m.statusBar.NotifyError("Replace queue failed: " + err.Error())
		return m
	}
	// The prior cleanup may purge by timestamp. Hold it until these new
	// tombstones are restored or released, including within the same second.
	previousCleanup := m.undo.slot.cleanup
	m.undo.slot.cleanup = nil
	action := undoReplaceQueueAction{queue: old, index: m.playback.currentIndex,
		state: m.playback.state, elapsed: m.playback.elapsedSecs, previousCleanup: previousCleanup}
	m = m.supersedeRadioBatch()
	m = m.resetPlaybackState()
	m = m.resetQueueState()
	m = exitVisualMode(m)
	m.queue.Entries = newQueueEntries(msg.tracks, ids, msg.source)
	m.syncPlayerQueue()
	before := time.Now().Unix() + 1
	undoer := m.undoer
	m.setUndoSlot(action, func() {
		if previousCleanup != nil {
			previousCleanup()
		}
		if undoer != nil {
			if err := undoer.PurgeDeletedQueueEntries(before); err != nil {
				slog.Warn("replacement cleanup failed", "err", err)
			}
		}
	})
	m.queueDirty = true
	if msg.selected && msg.view == m.ActiveView && sameQueueActionTracks(msg.tracks, m.selectedEnqueueTracks()) {
		m = exitVisualMode(m).clearActiveViewSelection()
	}
	first := msg.tracks[0]
	return m.tryAutoPlay(first.Hash, first.Artist, first.Title, msg.source)
}

type undoReplaceQueueAction struct {
	queue           queue.View
	index           int
	state           playback.State
	elapsed         float64
	previousCleanup func()
}

func (a undoReplaceQueueAction) kind() undoKind { return undoKindReplaceQueue }
func (a undoReplaceQueueAction) description() string {
	return "Queue replaced — press u to restore previous queue"
}
func (a undoReplaceQueueAction) perform(m Model) (Model, error) {
	writer, ok := m.queueWriter.(queueReplacer)
	if !ok {
		return m, fmt.Errorf("queue replacement is unavailable")
	}
	if err := m.withStableQueuePlayback(func() error {
		return writer.UndoReplaceQueue(queueEntryIDs(m.queue.Entries), queueEntryIDs(a.queue.Entries))
	}); err != nil {
		return m, err
	}
	m = m.supersedeRadioBatch()
	m = m.resetPlaybackState()
	width, height := m.queue.Width, m.queue.Height
	m.queue = a.queue
	m.queue.SetSize(width, height)
	m.queue.SavingAsPlaylist, m.queue.SavingFromSelection = false, false
	m.queue.Entries = slices.Clone(a.queue.Entries)
	m.syncPlayerQueue()
	m.queueDirty = true
	if a.previousCleanup != nil {
		a.previousCleanup()
	}
	if m.undoer != nil {
		schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
	}
	m.statusBar.Notify(fmt.Sprintf("Undone: restored previous queue (%d tracks)", len(m.queue.Entries)))
	return m.restoreReplacementPlayback(a), nil
}

func (m Model) restoreReplacementPlayback(a undoReplaceQueueAction) Model {
	if m.player == nil || a.index < 0 || a.index >= len(m.queue.Entries) ||
		(a.state != playback.StatePlaying && a.state != playback.StatePaused) {
		return m
	}
	err := m.playRequestExclusive(func() error { return resumeReplacementPlayer(m.player, a) })
	if err != nil {
		m.statusBar.NotifyError("Queue restored; could not resume playback: " + err.Error())
		return m
	}
	entry := m.queue.Entries[a.index]
	m.playback.currentIndex, m.playback.currentHash = a.index, entry.Hash
	m.playback.state, m.playback.elapsedSecs = a.state, a.elapsed
	m.statusBar.Artist, m.statusBar.Title = entry.Artist, entry.Title
	m.statusBar.PlaylistSource = queueSourceName(entry.SourcePlaylist)
	return m
}

func (m Model) requestQueueReplacement() (tea.Model, tea.Cmd, bool) {
	if m.queueReplacementPending || !m.atEnqueueTrackLevel() {
		return m, nil, true
	}
	if _, ok := m.queueWriter.(queueReplacer); !ok {
		m.statusBar.NotifyError("Queue replacement is unavailable")
		return m, nil, true
	}
	tracks, source := m.queueActionTracks()
	if len(tracks) == 0 {
		return m, nil, true
	}
	m.queueReplacementPending = true
	view, selected := m.ActiveView, m.activeViewSelectionCount() > 0
	return m, func() tea.Msg {
		return queueReplaceRequestedMsg{tracks: tracks, source: source, view: view, selected: selected}
	}, true
}

// queueActionTracks is shared by replacement and selection-aware play-next.
func (m Model) queueActionTracks() ([]library.Track, string) {
	source := ""
	if m.ActiveView == viewPlaylists {
		_, source, _, _, _ = m.resolvePlaylistForEnqueue()
	}
	if m.ActiveView == viewInsights {
		source = m.insights.DrillLabel
	}
	if m.activeViewSelectionCount() > 0 {
		return m.selectedEnqueueTracks(), source
	}
	if track, ok := m.focusedTrackForPreview(); ok {
		return []library.Track{track}, source
	}
	return nil, source
}

func sameQueueActionTracks(a, b []library.Track) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Hash != b[i].Hash {
			return false
		}
	}
	return true
}

func resumeReplacementPlayer(p playPauser, a undoReplaceQueueAction) error {
	if err := p.PlayFromIndex(a.index); err != nil {
		return err
	}
	if seeker, ok := p.(interface{ SeekAbsolute(float64) error }); ok {
		if err := seeker.SeekAbsolute(a.elapsed); err != nil {
			return err
		}
	}
	if a.state == playback.StatePaused {
		return p.Pause()
	}
	return nil
}

// Do not cancel an outstanding playback gesture just to attempt a DB write:
// a failed transaction must leave its acknowledgement and playback intact.
func (m Model) withStableQueuePlayback(commit func() error) error {
	if m.playback.pendingSelection {
		return fmt.Errorf("playback is changing; retry when it settles")
	}
	gate := m.playback.requests
	if gate == nil {
		return commit()
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if len(gate.pending) != 0 {
		return fmt.Errorf("playback is changing; retry when it settles")
	}
	return commit()
}
