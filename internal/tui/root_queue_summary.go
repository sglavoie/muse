package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// buildQueueSummary computes aggregate statistics from the current queue
// entries and playback position.
func (m Model) buildQueueSummary() *overlay.QueueSummaryOverlay {
	artists := make(map[string]struct{})
	albums := make(map[string]struct{})
	sources := make(map[string]struct{})
	var totalMs int64
	var remainMs int64
	var remainN int

	for i, entry := range m.queue.entries {
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
		if m.playback.currentIndex >= 0 && i > m.playback.currentIndex {
			remainMs += entry.DurationMs
			remainN++
		}
	}

	if m.playback.currentIndex >= 0 &&
		(m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused) {
		leftSecs := m.playback.totalSecs - m.playback.elapsedSecs
		if leftSecs > 0 {
			remainMs += int64(leftSecs * 1000)
			remainN++
		}
	}

	return &overlay.QueueSummaryOverlay{
		Tracks:   len(m.queue.entries),
		Artists:  len(artists),
		Albums:   len(albums),
		Sources:  len(sources),
		RemainMs: remainMs,
		RemainN:  remainN,
		TotalMs:  totalMs,
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
		return m, tea.Quit, true
	case "esc", "q":
		m.overlay.queueSummary = nil
		return m, nil, true
	default:
		return m, nil, true
	}
}
