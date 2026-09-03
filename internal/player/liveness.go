package player

// This file holds logical playback liveness: the Manager's own model of whether
// a queue traversal is still in progress.
//
// It is deliberately separate from the two states it is easiest to confuse it
// with:
//
//   - mpv process health. An idle mpv — idle-active, an empty or fully played
//     playlist — is not a fault. It is exactly what stop-after-current and a
//     fully traversed queue leave behind, so the restart supervisor must not
//     read an idle process as a reason to restart, and this file must not read
//     a reconnect as a reason to resume.
//   - playlist index bookkeeping. currentIndex, the prefetch slots and
//     playlistGen describe where the traversal *is*, never whether it is still
//     running: after the last EOF currentIndex keeps naming the row that
//     finished, which is precisely why it cannot answer "may audio start here".
//
// The distinction matters because the gapless prefetch appends with
// `append-play`, which starts an idle mpv playlist. That is deliberate — it is
// how a playlist mpv drained before Go noticed resumes instead of stalling —
// but it also means a best-effort append is capable of starting audio. Liveness
// is what tells the two idles apart: an idle playlist under a live traversal is
// a stall to recover, while an idle playlist under an ended traversal is what
// the user asked for and must be left alone.
//
// Only an explicit operation makes an ended traversal live again. Every one of
// them reaches mpv through replaceLocked (PlayFromIndex, Next, Previous,
// RestartQueueCycle, ReplayCurrent, ResumeAfterQueueExtension, PlayQueueEntry)
// or through the two isolated session loads that bypass the transaction layer
// (LoadFilePaused, loadFileAt).
//
// Pause and Stop are not ends: muse's Stop pauses and seeks to zero, so mpv
// still holds the current entry and the traversal is still positioned on it.
// Neither leaves mpv idle, so neither can be restarted by an append.
type playbackLiveness uint8

const (
	// livenessLive means a queue traversal is in progress: mpv holds the
	// current entry — playing or paused — and best-effort playlist work may act
	// on it.
	//
	// It is the zero value on purpose. An intentional end is something the
	// Manager has to observe, and must never be inferred from a Manager that
	// simply has not played anything yet; such a Manager has currentIndex == -1,
	// which already suppresses every prefetch.
	livenessLive playbackLiveness = iota

	// livenessEnded means the traversal reached its end and mpv was left idle on
	// purpose: the user armed stop-after-current, or the queue ran out at EOF.
	// Best-effort work must not start audio from here.
	livenessEnded
)

// live reports whether a queue traversal is still in progress.
func (l playbackLiveness) live() bool { return l == livenessLive }

// markPlaybackLiveLocked records that a load this Manager asked for has reached
// mpv, so the traversal is running again. It is the counterpart of the two end
// transitions: an explicit operation is the only thing that may lift an
// intentional end.
//
// The caller must hold m.mu.
func (m *Manager) markPlaybackLiveLocked() {
	m.liveness = livenessLive
}

// markPlaybackLive is markPlaybackLiveLocked for a caller that does not hold
// m.mu.
func (m *Manager) markPlaybackLive() {
	m.mu.Lock()
	m.markPlaybackLiveLocked()
	m.mu.Unlock()
}

// markPlaybackEndedLocked records that the traversal has ended and the idle mpv
// is now in is intentional.
//
// The caller must hold m.mu.
func (m *Manager) markPlaybackEndedLocked() {
	m.liveness = livenessEnded
}

// markPlaybackEnded is markPlaybackEndedLocked for a caller that does not hold
// m.mu.
func (m *Manager) markPlaybackEnded() {
	m.mu.Lock()
	m.markPlaybackEndedLocked()
	m.mu.Unlock()
}
