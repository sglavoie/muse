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
// An end is not instantaneous, which is why there are three states rather than
// two. Only stop-after-current can be decided where the end-file is read: the
// flag says the traversal stops here whatever the queue holds. A natural end is
// decided by the advance that follows it, inside a playlist transaction on a
// background goroutine, and until that transaction runs nothing distinguishes a
// track that will be followed by another from the last one in the queue.
// livenessEnding is that interval, and it counts as not-live: the restart
// supervisor and the best-effort prefetch both have to act during it, and
// treating an EOF that may be an end as if the traversal were still running is
// what lets them reinstate a track that had finished playing.
//
// Only an explicit operation makes an ended traversal live again. Every one of
// them reaches mpv through replaceLocked (PlayFromIndex, Next, Previous,
// RestartQueueCycle, ReplayCurrent, ResumeAfterQueueExtension, PlayQueueEntry)
// or through the two isolated session loads that bypass the transaction layer
// (LoadFilePaused, loadFileAt).
//
// An mpv restart is not one of those operations, which is why liveness has to
// survive it: the process is new but the traversal is the same one, and the
// bookkeeping that says where it is (lastTrackPath, currentIndex) still names
// the row an intentional end finished on. resumeAfterRestart is what keeps the
// distinction — it carries this value into the recovery and only reaches
// loadFileAt for a traversal that was still running.
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

	// livenessEnding means a natural end-file has been read and the successor
	// question has not been answered yet. mpv is idle or draining and the entry
	// lastTrackPath names has finished playing, so nothing may reinstate it: the
	// state is not-live for exactly as long as that is true.
	//
	// It resolves into livenessLive as soon as a successor is confirmed — the
	// tracked prefetch mpv had already started gaplessly, or a load the fallback
	// advance sends — and into livenessEnded when neither happens. It is entered
	// in the same critical section that snapshots the EOF, before either outcome
	// is known, because that is the only ordering in which a recovery snapshot
	// cannot slip in front of it.
	livenessEnding
)

// live reports whether a queue traversal is still in progress. A pending end is
// not live: the decision it is waiting on can only confirm the end or start a
// successor, and neither outcome makes the track that just finished resumable.
func (l playbackLiveness) live() bool { return l == livenessLive }

// String names the state for the logs, where "not live" now covers two distinct
// situations and the number alone would not say which one suppressed a resume.
func (l playbackLiveness) String() string {
	switch l {
	case livenessLive:
		return "live"
	case livenessEnding:
		return "ending"
	case livenessEnded:
		return "ended"
	default:
		return "unknown"
	}
}

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

// markPlaybackEndingLocked records that a natural end-file has been read and the
// decision that will either continue the traversal or exhaust it has not
// resolved yet.
//
// It is deliberately Locked-only, with no unlocked counterpart: the mark has to
// be published in the same critical section that captures the EOF state, or a
// recovery snapshot taken in between reads the completed entry as still live and
// reloads it. Everything the EOF decision then does — promoting a prefetch,
// falling back to the traversal advance — happens after this state is already
// visible, so the pending end is ordered ahead of every reader of liveness
// rather than racing them.
//
// A traversal that is not live is left alone: an end that has already been
// confirmed is not reopened by a later end-file, and there is nothing to keep
// pending. The generation the EOF belongs to needs no check here for the same
// reason the mark is atomic — a replacement cannot have landed between the
// snapshot and this line, because they are one lock acquisition.
//
// The caller must hold m.mu.
func (m *Manager) markPlaybackEndingLocked() {
	if m.liveness != livenessLive {
		return
	}
	m.liveness = livenessEnding
}

// settlePlaybackEnding resolves a pending end whose advance will never report
// back: the transaction was superseded — by a queue-owned replacement or by an
// mpv restart opening a new playlist generation — or it failed outright.
//
// The outcome is the end, not a return to live. mpv played the entry out and
// nothing loaded a successor, so the traversal is over in the only sense this
// state models, and lastTrackPath names a completed row that best-effort work
// must not reinstate. Anything that did load a successor has already marked the
// traversal live through replaceLocked, and that is what the state check
// preserves.
func (m *Manager) settlePlaybackEnding() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.liveness != livenessEnding {
		return
	}
	m.liveness = livenessEnded
}
