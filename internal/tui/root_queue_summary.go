package tui

import tea "charm.land/bubbletea/v2"

// buildQueueSummary computes aggregate statistics from the current queue
// entries and playback position.
func (m Model) buildQueueSummary() *QueueSummaryOverlay {
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
		(m.playback.state == PlaybackPlaying || m.playback.state == PlaybackPaused) {
		leftSecs := m.playback.totalSecs - m.playback.elapsedSecs
		if leftSecs > 0 {
			remainMs += int64(leftSecs * 1000)
			remainN++
		}
	}

	return &QueueSummaryOverlay{
		tracks:   len(m.queue.entries),
		artists:  len(artists),
		albums:   len(albums),
		sources:  len(sources),
		remainMs: remainMs,
		remainN:  remainN,
		totalMs:  totalMs,
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
