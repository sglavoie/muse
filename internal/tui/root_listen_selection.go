package tui

import "github.com/sglavoie/muse/internal/tui/queue"

// selectionListen retains the outgoing measurement until a selection really
// ends it. Keeping it outside the flush queue prevents both persistence and
// counted-play mirrors from advancing for a boundary that can still be refused.
type selectionListen struct {
	listen         pendingListen
	request        uint64
	rowID          int64
	index          int
	lastSampleSecs float64
}

func (m Model) suspendListenForSelection() Model {
	if !m.playback.pending.armed {
		// Consecutive unconfirmed selections share the original measurement.
		return m
	}
	p := m.playback.pending
	p.listenedSecs = m.playback.listenedSecs
	p.totalSecs = m.playback.peakTotalSecs
	s := selectionListen{listen: p, index: m.playback.currentIndex, lastSampleSecs: m.playback.lastSampleSecs}
	if s.index >= 0 && s.index < len(m.queue.Entries) {
		s.rowID = m.queue.Entries[s.index].ID
	}
	m.playback.selectionListen = s
	m.playback.pending = pendingListen{}
	return m
}

func (m Model) flushSelectionListen() Model {
	p := m.playback.selectionListen.listen
	m.playback.selectionListen = selectionListen{}
	if p.armed && p.listenedSecs >= minListenSecs {
		m.playback.flushes = append(m.playback.flushes, p)
	}
	return m
}

func (m Model) reconcileSelectionListen(index int, entry queue.Entry) Model {
	s := m.playback.selectionListen
	sameRow := s.rowID != 0 && s.rowID == entry.ID || s.rowID == 0 && s.index == index
	if s.listen.armed && s.listen.hash == entry.Hash && sameRow {
		m.playback.selectionListen = selectionListen{}
		m.playback.pending = s.listen
		m.playback.listenedSecs = s.listen.listenedSecs
		m.playback.peakTotalSecs = s.listen.totalSecs
		m.playback.totalSecs = s.listen.totalSecs
		m.playback.lastSampleSecs = s.lastSampleSecs
		m.playback.elapsedSecs = s.lastSampleSecs
		m.playback.peakElapsedSecs = s.lastSampleSecs
		return m
	}
	// A newer Next/Previous may have moved the player, even to another
	// occurrence of this hash. That is a separate listen.
	m = m.endCurrentListen()
	return m.armPendingListen(entry.Hash, entry.Artist)
}
