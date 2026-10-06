package autodj

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// HandleKey processes one key press and reports what the root model should do.
//
// The panel never returns a tea.Cmd: everything it can decide on its own it
// decides here, and everything it cannot — applying the draft, closing the
// overlay — is the caller's to perform. That keeps the component testable
// without a program running behind it.
func (p TunePanel) HandleKey(msg tea.KeyPressMsg) (TunePanel, Action) {
	key := msg.String()
	if p.discard.Active {
		var discard bool
		p.discard, discard = p.discard.HandleKey(key)
		if discard {
			return p, ActionCancel
		}
		return p, actionNone
	}
	return p.handlePanelKey(key)
}

func (p TunePanel) handlePanelKey(key string) (TunePanel, Action) {
	if p.picker != nil {
		return p.handlePickerKey(key), actionNone
	}
	// The name box is checked before the panel-wide keys, not after. It is a
	// text field: esc must close it rather than the overlay, tab must not page
	// away from a half-typed name, and ctrl+s must not commit the draft from
	// under a gesture the user has not finished making.
	if p.prompt != nil {
		return p.handlePromptKey(key)
	}
	switch key {
	case "esc":
		return p.requestCancel()
	case "tab":
		return p.switchTab(1)
	case "shift+tab":
		return p.switchTab(-1)
	case "ctrl+s":
		return p.requestCommit()
	}
	switch p.tab {
	case tabPreview:
		return p.handlePreviewKey(key)
	case tabRecipes:
		return p.handleRecipesKey(key)
	}
	if key == "enter" {
		return p.activateRow()
	}
	return p.handleEditKey(key), actionNone
}

// maxPreviewOffset bounds the stored scroll position. The render clamps to the
// real line count, but the offset itself must not be able to run away while a
// key repeats on a page that has already reached its end.
const maxPreviewOffset = 512

// switchTab moves to the neighbouring page.
//
// Arriving on Preview asks the root for a draft plan. It is a request and not a
// command: the root reuses a plan that is still valid for these settings and
// these inputs, and computes a new one only when something moved. Either way the
// answer is read-only, so switching tabs cannot change what is playing or what
// is queued.
func (p TunePanel) switchTab(delta int) (TunePanel, Action) {
	p.tab = panelTab((int(p.tab) + int(tabCount) + delta) % int(tabCount))
	p.previewOffset = 0
	if p.tab == tabPreview {
		return p, ActionPreview
	}
	return p, actionNone
}

// handlePreviewKey scrolls the read-only page or asks for a fresh draft.
func (p TunePanel) handlePreviewKey(key string) (TunePanel, Action) {
	switch key {
	case "up", "k":
		p.previewOffset = max(p.previewOffset-1, 0)
	case "down", "j":
		p.previewOffset = min(p.previewOffset+1, maxPreviewOffset)
	case "r":
		p.previewOffset = 0
		return p, ActionPreview
	}
	return p, actionNone
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

func (p TunePanel) requestCancel() (TunePanel, Action) {
	if p.Dirty() {
		p.discard = overlay.DiscardPrompt{Active: true}
		return p, actionNone
	}
	return p, ActionCancel
}
