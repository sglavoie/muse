package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/tui/text"
)

// scanFinishedMsg is delivered after every scan write, including missing marks.
// It carries its own summary so a later scan cannot change the reported counts.
type scanFinishedMsg struct {
	summary      string
	err          error
	missingError bool
}

func (m Model) handleScanFinishedMsg(msg scanFinishedMsg) Model {
	if msg.err != nil {
		summary := strings.Replace(msg.summary, "Scan complete", "Scan finished with save errors", 1)
		if summary == "" {
			summary = "Scan save failed"
		}
		retry := "retry with ctrl+r"
		if msg.missingError {
			// These tracks have already left the in-memory index. A restart
			// reloads their durable state so the startup diff can retry them.
			retry = "fix the error, then restart muse to retry missing-track updates"
		}
		m.statusBar.NotifyError(fmt.Sprintf("%s — %v; %s", summary, msg.err, retry))
	} else if msg.summary != "" {
		m.statusBar.Notify(msg.summary)
	}
	return m
}

// Only full rescans have a before/after snapshot. Compare hashes so moving a
// file preserves its identity; an in-place replacement counts as a new track.
func (m Model) scanCompletionSummary() string {
	summary := fmt.Sprintf("Scan complete — %s %s", text.FormatCount(m.scanned), text.Pluralize(m.scanned, "track", "tracks"))
	if m.rescanSnapshot == nil {
		return summary
	}
	added, missing := 0, 0
	for _, track := range m.library.Index.Tracks() {
		if _, existed := m.rescanSnapshot[track.Hash]; !existed {
			added++
		}
	}
	for hash := range m.rescanSnapshot {
		if _, found := m.library.Index.FindByHash(hash); !found {
			missing++
		}
	}
	return fmt.Sprintf("%s · %s added · %s missing", summary, text.FormatCount(added), text.FormatCount(missing))
}
