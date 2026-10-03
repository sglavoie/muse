package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/overlay"
)

// buildQueueSummary computes aggregate statistics from the current queue
// entries and playback position.
func (m Model) buildQueueSummary() *overlay.QueueSummaryOverlay {
	artists := make(map[string]struct{})
	albums := make(map[string]struct{})
	sources := make(map[string]struct{})
	var totalMs int64
	remainMs, remainN, unknown := m.queueRemaining()

	for _, entry := range m.queue.Entries {
		totalMs += entry.DurationMs
		if entry.Artist != "" {
			artists[entry.Artist] = struct{}{}
		}
		if entry.Album != "" {
			albums[entry.Album] = struct{}{}
		}
		if entry.SourcePlaylist != "" {
			sources[entry.SourcePlaylist] = struct{}{}
		}
	}

	return &overlay.QueueSummaryOverlay{
		Tracks:        len(m.queue.Entries),
		Artists:       len(artists),
		Albums:        len(albums),
		Sources:       len(sources),
		RemainMs:      remainMs,
		RemainN:       remainN,
		RemainUnknown: unknown,
		TotalMs:       totalMs,
	}
}

// handleQueueSummaryKeyPress handles key events while the queue summary
// overlay is active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleQueueSummaryKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.queueSummary == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.queueSummary = nil
		return m.markQuitting(), tea.Quit, true
	case "esc", "q":
		m.overlay.queueSummary = nil
		return m, nil, true
	default:
		return m, nil, true
	}
}
