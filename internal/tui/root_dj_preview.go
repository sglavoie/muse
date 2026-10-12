package tui

import (
	"errors"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// djStartPreviewCmd plans a draft batch for the Preview tab.
//
// Purity is the whole contract. The command reads the same snapshot a live
// refill would, and passes the live allocation in so the draft shows the
// discovery split the user would actually get — but it prepares nothing, writes
// nothing, and returns a message the handler files in its own field. The
// returned NextAllocation is discarded, the recent-selection ring is untouched,
// and the live request identity is not advanced, so a preview cannot supersede a
// refill that is keeping playback alive.
func (m Model) djStartPreviewCmd(settings radio.Settings, now time.Time) (Model, tea.Cmd) {
	m.djPlan.previewID++
	m.djPlan.previewPending = true
	m.djPlan.previewSettings = settings.Normalized()
	id := m.djPlan.previewID
	in := m.captureDJInputs(now)
	req := radio.PlanRequest{
		Settings:    settings.Normalized(),
		Seed:        m.djPlanSeed(now),
		BatchSize:   radioBatchSize,
		RequestedAt: now,
	}
	fingerprint := m.djPreviewFingerprint(req.Settings, in.dataRev, in.queueRev, now)
	alloc := m.dj.allocation
	reader := m.recReader
	return m, func() tea.Msg {
		result, signals, err := djPlanBatch(in, req, alloc, reader, now)
		return djPreviewPlannedMsg{
			RequestID:   id,
			settings:    req.Settings,
			result:      result,
			signals:     signals,
			err:         err,
			at:          now,
			fingerprint: fingerprint,
		}
	}
}

// djPreviewFingerprint stamps a draft's own settings onto the inputs it was
// planned from. It differs from djFingerprint in exactly that: a preview is
// planned for the settings being edited, which are not the ones in force.
func (m Model) djPreviewFingerprint(
	settings radio.Settings, dataRev, queueRev uint64, now time.Time,
) djPlanFingerprint {
	fp := m.djFingerprint(dataRev, queueRev, now)
	fp.settings = settings
	return fp
}

// djPreviewFresh reports whether the held draft still describes what planning
// these settings right now would produce.
//
// Reuse is the point. Switching to the Preview tab, scrolling it, and coming
// back to it are all common, and re-planning on each would spend a full
// candidate scan to redraw the same five tracks. What must never be reused is a
// draft whose inputs moved: the settings, the library, the rows the batch may
// not select again, the seed, and the hour all change the answer, and
// sameInputs is the one predicate that knows all five.
func (m Model) djPreviewFresh(settings radio.Settings, now time.Time) bool {
	p := m.djPlan.preview
	if !p.valid {
		return false
	}
	return p.fingerprint.sameInputs(
		m.djPreviewFingerprint(settings.Normalized(), m.djData.dataRev, m.djQueueRevision(), now))
}

// djSupersedePreview retires any draft in flight. Escape discarding an unapplied
// edit must also discard the preview computed for it, or the tab would settle
// back onto a draft the user just abandoned.
func (m Model) djSupersedePreview() Model {
	m.djPlan.previewID++
	m.djPlan.previewPending = false
	m.djPlan.previewSettings = radio.Settings{}
	m.djPlan.preview = djPreview{}
	return m
}

// handleDJPreviewPlannedMsg files a draft result.
//
// It touches nothing but the preview fields on purpose: no queue, no recent
// ring, no allocation, no live request identity, no database. A stale draft is
// dropped outright rather than reconciled, because a preview left nothing behind
// to reconcile.
func (m Model) handleDJPreviewPlannedMsg(msg djPreviewPlannedMsg) Model {
	if msg.RequestID != m.djPlan.previewID {
		return m
	}
	m.djPlan.previewPending = false
	m.djPlan.preview = djPreview{
		valid:       true,
		settings:    msg.settings,
		result:      msg.result,
		signals:     msg.signals,
		err:         msg.err,
		at:          msg.at,
		fingerprint: msg.fingerprint,
	}
	if msg.err != nil {
		slog.Warn("dj preview: plan unavailable", "err", msg.err)
	}
	return m
}

// rejectDJPreparation retires hidden rows a result will not publish.
//
// A preparation that is already gone is not a failure: restart recovery converts
// abandoned rows into ordinary tombstones, so ErrQueueBatchStale means the work
// this call exists to do has already happened. Any other failure retains the
// exact preparation for the periodic retry, because hidden rows that outlive
// their result would be republished by the next accept that names them.
func (m Model) rejectDJPreparation(prep db.DJPreparation) Model {
	if m.djQueue == nil || len(prep.IDs) == 0 {
		return m
	}
	if err := m.djQueue.RejectDJBatch(prep); err != nil {
		if errors.Is(err, db.ErrQueueBatchStale) {
			return m
		}
		slog.Warn("dj: discarded batch rejection failed", "err", err)
		m.djPlan.rejects = append(m.djPlan.rejects, prep)
	}
	return m
}

// startDJRejectRetryCmd starts at most one retained rejection retry, mirroring
// the Radio cleanup FIFO: repeated database failures stay bounded while every
// abandoned preparation remains live until its rejection wins.
func (m Model) startDJRejectRetryCmd() (Model, tea.Cmd) {
	if m.djPlan.rejectInFlight || len(m.djPlan.rejects) == 0 || m.djQueue == nil {
		return m, nil
	}
	prep := m.djPlan.rejects[0]
	writer := m.djQueue
	m.djPlan.rejectInFlight = true
	m.djPlan.rejectAttempted = prep
	return m, func() tea.Msg {
		return djRejectRetryMsg{Prep: prep, Err: writer.RejectDJBatch(prep)}
	}
}

// handleDJRejectRetryMsg removes only the exact front preparation once its
// rejection succeeds, or once recovery has made it moot.
func (m Model) handleDJRejectRetryMsg(msg djRejectRetryMsg) Model {
	if !m.djPlan.rejectInFlight || len(m.djPlan.rejects) == 0 ||
		!sameRadioQueueIDs(m.djPlan.rejects[0].IDs, msg.Prep.IDs) {
		return m
	}
	m.djPlan.rejectInFlight = false
	if msg.Err != nil && !errors.Is(msg.Err, db.ErrQueueBatchStale) {
		slog.Warn("dj: discarded batch rejection retry failed", "err", msg.Err)
		return m
	}
	m.djPlan.rejects = m.djPlan.rejects[1:]
	if len(m.djPlan.rejects) == 0 {
		m.djPlan.rejects = nil
	}
	return m
}
