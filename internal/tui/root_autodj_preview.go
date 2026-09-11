package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/autodj"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/text"
)

// This file connects the Preview tab to the asynchronous planner.
//
// The contract the whole tab rests on is that reading is free of consequence.
// Opening the tab, scrolling it, and asking it to replan all go through
// djStartPreviewCmd, which plans against a captured snapshot, prepares no rows,
// writes nothing, discards the allocation the plan would have earned, and files
// its answer behind its own request counter. Nothing here reaches the queue, the
// playback history, the recent-selection ring, the discovery carry, or the audio
// preview player in root_preview.go — that one is a second mpv instance and has
// nothing to do with this.
//
// The second rule is that an explanation is only as good as its evidence. A
// committed row's reasons are read back from what was persisted at selection
// time rather than re-derived from today's library, and a signal that could not
// be read is reported as unavailable instead of being allowed to read as a
// track that scored badly.

// djPreviewAheadCap bounds how much of the committed queue the tab lists.
//
// The point of the list is to make "this batch is not what you hear next"
// concrete, and a handful of rows does that. The count above it is the whole
// truth; the remainder is reported rather than dropped.
const djPreviewAheadCap = 8

// ensureAutoDJPreview makes the Preview tab's draft current for settings.
//
// A held draft that still matches the settings and the inputs is reused as is —
// that is the difference between a tab a user can scroll and one that re-scans
// the library on every keypress. A draft that no longer matches is replaced, and
// a request already in flight for these same settings is left to finish.
func (m Model) ensureAutoDJPreview(settings radio.Settings, now time.Time) (Model, tea.Cmd) {
	if m.djPreviewFresh(settings, now) {
		return m, nil
	}
	if m.djPlan.previewPending && m.djPlan.previewSettings.Equal(settings.Normalized()) {
		return m, nil
	}
	return m.djStartPreviewCmd(settings, now)
}

// autoDJPanelPreview is the snapshot the open panel renders.
func (m Model) autoDJPanelPreview() autodj.Preview {
	if m.overlay.autoDJ == nil {
		return autodj.Preview{}
	}
	return m.autoDJPreviewFor(m.overlay.autoDJ.Draft(), time.Now())
}

// autoDJPreviewFor builds the snapshot the Preview tab renders.
func (m Model) autoDJPreviewFor(settings radio.Settings, now time.Time) autodj.Preview {
	ahead, more := m.djAheadTracks()
	v := autodj.Preview{
		Settings:  settings.Normalized(),
		Ahead:     ahead,
		AheadMore: more,
	}
	p := m.djPlan.preview
	switch {
	case !p.valid:
		v.Status = autodj.PreviewIdle
		if m.djPlan.previewPending {
			v.Status = autodj.PreviewPlanning
		}
		return v
	case p.err != nil:
		v.Status = autodj.PreviewFailed
		v.Err = p.err
		v.Settings = p.settings
		return v
	}

	v.Status = autodj.PreviewReady
	v.Settings = p.settings
	v.Replanning = m.djPlan.previewPending
	v.Stale = v.Replanning || !m.djPreviewFresh(settings, now)
	v.Outcome = p.result.Outcome
	v.DiscoveryTarget = p.result.DiscoveryTarget
	v.Tracks = djPreviewTracks(p.result.Tracks)
	v.Shortages = p.result.Shortages
	v.Reasons = p.result.Reasons
	v.Rejections = p.result.Rejections
	v.Gaps = djEvidenceGaps(p.signals)
	return v
}

// djPreviewTracks projects the planner's selections onto the display contract.
func djPreviewTracks(picked []radio.PlannedTrack) []autodj.PreviewTrack {
	tracks := make([]autodj.PreviewTrack, len(picked))
	for i, p := range picked {
		tracks[i] = autodj.PreviewTrack{
			Label:     text.TrackDisplay(p.Track.Artist, p.Track.Title, p.Track.Path),
			Discovery: p.Discovery,
			Reasons:   p.Reasons,
		}
	}
	return tracks
}

// djEvidenceGaps names the optional signals that could not be read for the draft.
//
// Only genuine failures appear. Co-occurrence that was never asked for — an
// unseeded or synthetic start has none to ask about — is not a gap, because
// reporting it would tell the user evidence went missing when none existed.
func djEvidenceGaps(signals djSignals) []autodj.EvidenceGap {
	failures := signals.Failures()
	if len(failures) == 0 {
		return nil
	}
	gaps := make([]autodj.EvidenceGap, len(failures))
	for i, f := range failures {
		gaps[i] = autodj.EvidenceGap{Signal: f.Signal}
		if f.Err != nil {
			gaps[i].Detail = f.Err.Error()
		}
	}
	return gaps
}

// djAheadTracks lists the committed rows that play before any new batch, in the
// order the player will reach them, and reports how many were left out.
//
// The order comes from the player's traversal for the same reason
// djProtectedHashes takes it from there: under shuffle the visible queue
// position of a row says nothing about when it will be heard, so listing the
// queue in storage order would be a confident statement of the wrong thing.
func (m Model) djAheadTracks() ([]autodj.QueuedTrack, int) {
	entries := m.djAheadEntries()
	shown := min(len(entries), djPreviewAheadCap)
	tracks := make([]autodj.QueuedTrack, 0, shown)
	for i, e := range entries[:shown] {
		tracks = append(tracks, autodj.QueuedTrack{
			Label:   queueEntryLabel(e),
			Playing: i == 0 && e.Hash != "" && e.Hash == m.playback.currentHash,
			DJ:      e.DJOwned,
			// Persisted evidence, never a fresh classification: this is what the
			// DJ knew when it chose the row, and re-deriving it would quietly
			// replace the record with today's opinion.
			Reasons: e.DJReasons,
		})
	}
	return tracks, len(entries) - shown
}

// djAheadEntries returns the current row plus every row still to be visited.
func (m Model) djAheadEntries() []queue.Entry {
	t, ok := m.playerTraversal()
	if !ok {
		// Same degraded fallback djProtectedHashes documents: persisted order,
		// which is only the real order when shuffle is off.
		return m.queueEntriesAt(indexRange(max(m.playback.currentIndex, 0), len(m.queue.Entries)))
	}
	start := max(t.CurrentPosition, 0)
	return m.queueEntriesAt(t.Order[min(start, len(t.Order)):])
}

// queueEntriesAt collects the queue rows named by a list of indices, skipping
// any the queue no longer holds.
func (m Model) queueEntriesAt(indices []int) []queue.Entry {
	entries := make([]queue.Entry, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(m.queue.Entries) {
			continue
		}
		entries = append(entries, m.queue.Entries[idx])
	}
	return entries
}

// indexRange returns the half-open range [start,end) as a slice of indices.
func indexRange(start, end int) []int {
	if end <= start {
		return nil
	}
	out := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, i)
	}
	return out
}

// queueEntryLabel renders a committed row for the preview list.
func queueEntryLabel(e queue.Entry) string {
	if e.Display != "" {
		return e.Display
	}
	return text.TrackDisplay(e.Artist, e.Title, e.Path)
}
