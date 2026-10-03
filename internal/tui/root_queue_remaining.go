package tui

import (
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/text"
)

// queueRemaining describes the finite queued batch, never future repeats or DJ
// refills. Unknown durations are signalled rather than presented as exact.
func (m Model) queueRemaining() (ms int64, count int, unknown bool) {
	for _, idx := range m.remainingQueueIndices() {
		if idx < 0 || idx >= len(m.queue.Entries) {
			continue
		}
		e := m.queue.Entries[idx]
		if e.Missing {
			continue
		}
		count++
		ms += max(0, e.DurationMs)
		unknown = unknown || e.DurationMs <= 0
	}
	currentMs, currentN, currentUnknown := m.currentQueueRemaining()
	return ms + currentMs, count + currentN, unknown || currentUnknown
}

func (m Model) currentQueueRemaining() (ms int64, count int, unknown bool) {
	if !m.queuePlaybackActive() {
		return 0, 0, false
	}
	idx := m.playback.currentIndex
	if idx < 0 || idx >= len(m.queue.Entries) {
		return 0, 0, false
	}
	total := m.playback.totalSecs
	if total <= 0 {
		total = float64(m.queue.Entries[idx].DurationMs) / 1000
	}
	left := max(0, total-m.playback.elapsedSecs)
	if left > 0 || total <= 0 {
		return int64(left * 1000), 1, total <= 0
	}
	return 0, 0, false
}

func (m Model) queuePlaybackActive() bool {
	return m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused
}

func (m Model) remainingQueueIndices() []int {
	if m.playback.queueExhausted {
		return nil
	}
	if t, ok := m.playerTraversal(); ok {
		if t.RemainingIndices != nil {
			return t.RemainingIndices
		}
		// Older/simple players expose only the contiguous shuffled timeline.
		start := t.CurrentPosition + 1
		end := start + t.Remaining
		if t.Shuffled && start >= 0 && end >= start && end <= len(t.Order) {
			return t.Order[start:end]
		}
	}
	start := max(0, m.playback.currentIndex+1)
	rows := make([]int, 0, max(0, len(m.queue.Entries)-start))
	for i := start; i < len(m.queue.Entries); i++ {
		rows = append(rows, i)
	}
	return rows
}

func (m Model) queueRemainingLabel() string {
	ms, _, unknown := m.queueRemaining()
	duration := text.FormatDurationMs(ms)
	if unknown {
		duration += "+"
	}
	label := " remaining"
	if !m.queuePlaybackActive() || m.radio.enabled || m.playback.repeatMode != playback.RepeatOff || m.playback.stopAfterCurrent {
		label = " queued"
	}
	return duration + label
}
