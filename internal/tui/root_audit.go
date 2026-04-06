package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// handleDurationAuditBatchMsg applies a batch of ffprobe-corrected durations
// to the in-memory library index and any matching queue entries, then
// re-schedules the next batch read.
func (m Model) handleDurationAuditBatchMsg(msg library.DurationAuditBatchMsg) (Model, tea.Cmd) {
	for _, fix := range msg.Fixes {
		m.library.index.UpdateDuration(fix.Hash, float64(fix.DurationMs)/1000)
		for i := range m.queue.entries {
			if m.queue.entries[i].Hash == fix.Hash {
				m.queue.entries[i].DurationMs = fix.DurationMs
			}
		}
	}
	if m.auditCh != nil {
		return m, waitForAuditBatch(m.auditCh)
	}
	return m, nil
}

// handleDurationAuditDoneMsg shows a notification when tracks were corrected
// and marks the audit as complete by nilling the channel.
func (m Model) handleDurationAuditDoneMsg(msg library.DurationAuditDoneMsg) (Model, tea.Cmd) {
	if msg.Corrected > 0 {
		m.statusBar.notification = newNotification(
			fmt.Sprintf("Duration audit: corrected %d %s", msg.Corrected,
				pluralize(msg.Corrected, "track", "tracks")),
		)
	}
	m.auditCh = nil
	return m, nil
}
