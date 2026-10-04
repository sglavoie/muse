package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// This file holds the two contextual entry points into the DJ lifecycle: a
// start that targets a collection, and a start that targets what the user has
// selected or focused.
//
// Both exist so no caller has to assemble a start by hand. Enabling radio mode,
// persisting that, capturing the seed, announcing what was seeded, and asking
// the refill for a first batch are five steps that have to happen together, and
// a view that performed four of them would leave a session running that never
// plays anything, or a radio mode that does not survive a restart.
//
// Neither decides precedence itself. That lives in djResolveSeed, so "an
// explicit selection beats the row under the cursor, which beats the collection
// on screen" is stated once and every entry point inherits it.

// djStartFromCollection starts a session seeded from an explicitly targeted
// collection: the open mix, the Insights drilldown.
//
// The synthetic radio seed is set alongside because the pre-DJ radio scorer
// still reads it in builds with no DJ queue writer; it describes the same
// collection the DJ session is seeded from.
//
// The announcement names the seed that was actually captured rather than the
// collection that was asked for. They differ exactly when a bulk selection
// inside the collection outranked it, and that is the case the user most needs
// told — a mix opened with three tracks ticked seeds from the three.
func (m Model) djStartFromCollection(label string, tracks []library.Track, now time.Time) (Model, tea.Cmd) {
	seed := buildMixSeed(tracks, m.trackTags)
	m.radio.syntheticSeed = &seed
	m.radio.seedHash = ""
	m = m.radioSetEnabled(true)

	m, seedLabel := m.djStartFromTracks(label, tracks, now)
	m.statusBar.PushMsg("Radio: seeded from "+seedLabel,
		time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
	return m.startRadioEnqueueCmd()
}

// djStartFromSelection starts a session seeded from what the active view has
// selected or focused.
//
// The seed comes from djResolveSeed, not from the visible-track list the export
// commands fall back to. That fallback answers "what is on screen", which is the
// right question for an export and the wrong one here: a user who asked to start
// from their selection and got the whole library instead has been given a DJ
// that ignores the only instruction they gave it. So a start with nothing
// selected and nothing focused is refused outright rather than widened.
func (m Model) djStartFromSelection(now time.Time) (Model, tea.Cmd) {
	hash, synthetic, ok := m.djRadioSeedForSelection()
	if !ok {
		return m, func() tea.Msg {
			return statusbar.TransientNotifyMsg("Select or focus tracks to start Auto-DJ from")
		}
	}
	m.radio.seedHash = hash
	m.radio.syntheticSeed = synthetic
	m = m.radioSetEnabled(true)

	m, seedLabel := m.djStartFor(djSeedContextual, now)
	m.statusBar.PushMsg("Auto-DJ: seeded from "+seedLabel,
		time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
	return m.startRadioEnqueueCmd()
}

// djCanStartFromSelection reports whether a contextual start has something more
// specific than the visible list to seed from. It is what dims the palette
// command rather than letting it silently widen to everything on screen, and it
// answers from the same function that produces the seed, so the command cannot
// be offered in a state the start would then refuse.
func (m Model) djCanStartFromSelection() bool {
	_, _, ok := m.djRadioSeedForSelection()
	return ok
}

// djRadioSeedForSelection maps a contextual start onto the pre-DJ radio
// scorer's own seed fields, which take a different shape from a SeedSnapshot: a
// track hash for a single row, a synthetic aggregate for a set of them.
//
// It covers the two rungs of the precedence ladder a contextual start may use —
// an explicit bulk selection, then the focused row — and deliberately not the
// third. visibleTracks narrows to the selection whenever one exists, so the
// selection branch reads it back rather than re-deriving it.
func (m Model) djRadioSeedForSelection() (string, *radio.SeedTrack, bool) {
	if m.hasBulkSelection() {
		if tracks, _, found := m.visibleTracks(); found && len(tracks) > 0 {
			seed := buildMixSeed(tracks, m.trackTags)
			return "", &seed, true
		}
	}
	if track, found := m.focusedTrackForPreview(); found && track.Hash != "" {
		return track.Hash, nil, true
	}
	return "", nil, false
}
