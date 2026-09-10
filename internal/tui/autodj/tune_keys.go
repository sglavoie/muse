package autodj

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/radio"
)

// HandleKey processes one key press and reports what the root model should do.
//
// The panel never returns a tea.Cmd: everything it can decide on its own it
// decides here, and everything it cannot — applying the draft, closing the
// overlay — is the caller's to perform. That keeps the component testable
// without a program running behind it.
func (p TunePanel) HandleKey(msg tea.KeyPressMsg) (TunePanel, Action) {
	key := msg.String()
	if p.picker != nil {
		return p.handlePickerKey(key), actionNone
	}
	switch key {
	case "esc":
		return p, ActionCancel
	case "enter":
		return p.activateRow()
	case "ctrl+s":
		return p.requestCommit()
	}
	return p.handleEditKey(key), actionNone
}

// handleEditKey applies the keys that only ever move the focus or change the
// draft, none of which the root model has anything to do.
func (p TunePanel) handleEditKey(key string) TunePanel {
	switch key {
	case "up", "k":
		return p.moveCursor(-1)
	case "down", "j":
		return p.moveCursor(1)
	case "left", "h":
		return p.adjust(-1)
	case "right", "l":
		return p.adjust(1)
	case "x":
		return p.clearFocusedField()
	case "r":
		return p.repairUnresolved()
	}
	return p
}

// handlePickerKey routes a key to the open picker and files its answer.
func (p TunePanel) handlePickerKey(key string) TunePanel {
	updated, action := p.picker.handleKey(key)
	switch action {
	case pickerCancel:
		p.picker = nil
		return p
	case pickerConfirm:
		set := withFieldValues(p.filterSet(updated.exclude), updated.field, updated.selection())
		p = p.withFilterSet(updated.exclude, set)
		p.picker = nil
		p.err = nil
		return p
	}
	p.picker = &updated
	return p
}

// activateRow performs the focused row's enter gesture: filter rows open their
// picker, and the commit row is the button.
func (p TunePanel) activateRow() (TunePanel, Action) {
	r := p.focusedRow()
	if r == rowCommit {
		return p.requestCommit()
	}
	fr, ok := filterRows[r]
	if !ok {
		return p, actionNone
	}
	picker := newFilterPicker(fr.field, fr.exclude,
		p.fieldOptions(fr.field), fieldValues(p.filterSet(fr.exclude), fr.field))
	p.picker = &picker
	return p, actionNone
}

// requestCommit validates the draft and asks for it to be applied.
//
// A commit already in flight is ignored rather than queued: two settings
// snapshots racing into one session is exactly what the refresh supersession
// upstream exists to prevent, and a second press is far more likely to be
// impatience than a second intention.
func (p TunePanel) requestCommit() (TunePanel, Action) {
	if p.busy {
		return p, actionNone
	}
	if err := p.commitRefusal(); err != nil {
		p.err = err
		return p, actionNone
	}
	p.err = nil
	return p, ActionCommit
}

// moveCursor wraps the focus around the row list.
func (p TunePanel) moveCursor(delta int) TunePanel {
	p.cursor = (p.cursor + int(rowCount) + delta) % int(rowCount)
	return p
}

// adjust applies a left/right step to the focused row.
func (p TunePanel) adjust(delta int) TunePanel {
	switch p.focusedRow() {
	case rowPreset:
		return p.adjustPreset(delta)
	case rowDiscovery:
		p.draft = p.draft.TuneDiscovery(delta)
		p.err = nil
	case rowAnchor:
		p.draft.Anchor = toggleAnchor(p.draft.Anchor)
		p.err = nil
	}
	return p
}

// adjustPreset moves to the neighbouring preset.
//
// WithPreset carries the stock discovery target and nothing else, which is what
// makes "change the preset" safe for a user who has built up a filter set: the
// presets tune discovery, so switching between them must leave the inclusions,
// the exclusions, and the anchor exactly where they were.
func (p TunePanel) adjustPreset(delta int) TunePanel {
	next := presetCycle(p.draft.Preset(), delta)
	tuned, err := p.draft.WithPreset(next)
	if err != nil {
		p.err = err
		return p
	}
	p.draft = tuned
	p.err = nil
	return p
}

func toggleAnchor(mode radio.AnchorMode) radio.AnchorMode {
	if mode == radio.AnchorFollow {
		return radio.AnchorSeed
	}
	return radio.AnchorFollow
}

// clearFocusedField empties the focused filter row.
func (p TunePanel) clearFocusedField() TunePanel {
	fr, ok := filterRows[p.focusedRow()]
	if !ok {
		return p
	}
	p = p.withFilterSet(fr.exclude, withFieldValues(p.filterSet(fr.exclude), fr.field, nil))
	p.err = nil
	return p
}
