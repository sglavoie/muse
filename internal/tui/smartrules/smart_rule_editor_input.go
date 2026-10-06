package smartrules

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
)

// ForwardMsg routes a generic tea.Msg to the currently focused text input.
// Returns false if no text input is active in the current step.
func (e SmartRuleEditor) ForwardMsg(msg tea.Msg) (SmartRuleEditor, tea.Cmd, bool) {
	if e.discard.Active || e.saving {
		return e, nil, true
	}
	if e.tagPicker != nil {
		return e.forwardTagPicker(msg)
	}
	switch e.step {
	case stepName:
		updated, cmd := e.nameInput.Update(msg)
		e.nameInput = updated
		return e, cmd, true
	case stepEditRule:
		if e.editSubField == 2 && !e.choiceValue() {
			updated, cmd := e.ruleValueInput.Update(msg)
			e.ruleValueInput = updated
			return e, cmd, true
		}
	case stepSort:
		if e.sortCursor == 2 {
			updated, cmd := e.maxTracksInput.Update(msg)
			e.maxTracksInput = updated
			return e, cmd, true
		}
	}
	return e, nil, false
}

// HandleKey processes a key press and returns the updated editor, result action, and any tea.Cmd.
func (e SmartRuleEditor) HandleKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	if e.saving {
		return e, actionNone, nil
	}
	if e.discard.Active {
		return e.handleDiscard(msg.String())
	}
	if e.tagPicker != nil {
		return e.handleTagPicker(msg)
	}
	e.errorText = ""
	switch e.step {
	case stepName:
		return e.handleNameKey(msg)
	case stepRules:
		return e.handleRulesKey(msg)
	case stepEditRule:
		return e.handleEditRuleKey(msg)
	case stepSort:
		return e.handleSortKey(msg)
	}
	return e, actionNone, nil
}

func (e SmartRuleEditor) handleNameKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return e.requestClose()
	case "enter":
		if strings.TrimSpace(e.nameInput.Value()) != "" {
			e.step = stepRules
		}
		return e, actionNone, nil
	default:
		updated, cmd := e.nameInput.Update(msg)
		e.nameInput = updated
		return e, actionNone, cmd
	}
}

func (e SmartRuleEditor) handleRulesKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc", "left":
		e.step = stepName
	case "enter":
		e = e.openFocusedRule()
	case "right":
		e = e.advanceToSort()
	case "a":
		e = e.startNewRule()
	case "d":
		e = e.deleteFocusedRule()
	case "tab":
		e = e.toggleConjunction()
	case "j", "down":
		e = e.nextRule()
	case "k", "up":
		e = e.prevRule()
	}
	return e, actionNone, nil
}

// openFocusedRule opens the rule under the cursor in the sub-form, or skips
// straight to the sort step when there is nothing to edit.
func (e SmartRuleEditor) openFocusedRule() SmartRuleEditor {
	if len(e.rules) == 0 {
		e.step = stepSort
		return e
	}
	return e.startEditRule(e.ruleCursor)
}

// advanceToSort moves to the sort step, refusing to leave an empty rule list.
func (e SmartRuleEditor) advanceToSort() SmartRuleEditor {
	if len(e.rules) > 0 {
		e.step = stepSort
	}
	return e
}

// deleteFocusedRule removes the rule under the cursor and keeps the cursor in
// range afterwards.
func (e SmartRuleEditor) deleteFocusedRule() SmartRuleEditor {
	if len(e.rules) == 0 || e.ruleCursor >= len(e.rules) {
		return e
	}
	e.rules = append(e.rules[:e.ruleCursor], e.rules[e.ruleCursor+1:]...)
	if e.ruleCursor > 0 && e.ruleCursor >= len(e.rules) {
		e.ruleCursor = len(e.rules) - 1
	}
	return e
}

// toggleConjunction flips the rule list between AND and OR.
func (e SmartRuleEditor) toggleConjunction() SmartRuleEditor {
	if e.conjunction == db.ConjAnd {
		e.conjunction = db.ConjOr
		return e
	}
	e.conjunction = db.ConjAnd
	return e
}

// nextRule moves the cursor one rule down, stopping at the last rule.
func (e SmartRuleEditor) nextRule() SmartRuleEditor {
	if e.ruleCursor < len(e.rules)-1 {
		e.ruleCursor++
	}
	return e
}

// prevRule moves the cursor one rule up, stopping at the first rule.
func (e SmartRuleEditor) prevRule() SmartRuleEditor {
	if e.ruleCursor > 0 {
		e.ruleCursor--
	}
	return e
}

func (e SmartRuleEditor) startNewRule() SmartRuleEditor {
	e.editingRuleIdx = -1
	e.ruleFieldIdx = 0
	e.ruleOpIdx = 0
	e.ruleValueInput = newEditorInput("")
	e.ruleValueInput.Blur()
	e.editSubField = 0
	e.step = stepEditRule
	e.initialRule = e.draftRule()
	return e
}

func (e SmartRuleEditor) startEditRule(idx int) SmartRuleEditor {
	if idx < 0 || idx >= len(e.rules) {
		return e.startNewRule()
	}
	rule := e.rules[idx]
	e.editingRuleIdx = idx
	e.ruleFieldIdx = 0
	for i, f := range allRuleFields {
		if f == rule.Field {
			e.ruleFieldIdx = i
			break
		}
	}
	ops := validOpsForField(rule.Field)
	e.ruleOpIdx = 0
	for i, op := range ops {
		if op == rule.Operator {
			e.ruleOpIdx = i
			break
		}
	}
	e.ruleValueInput = newEditorInput(rule.Value)
	e.ruleValueInput.Blur()
	e.editSubField = 0
	e.step = stepEditRule
	e.initialRule = e.draftRule()
	return e
}

func (e SmartRuleEditor) handleEditRuleKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	if e.editSubField == 2 {
		if e.choiceValue() {
			return e.handleChoiceValueKey(msg)
		}
		return e.handleRuleValueKey(msg)
	}
	return e.handleRuleSelectionKey(msg)
}

// handleRuleValueKey processes keys while the rule's value text input is focused.
func (e SmartRuleEditor) handleRuleValueKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return e.cancelRule()
	case "enter":
		e = e.confirmEditRule()
	case "tab", "down":
		e.ruleValueInput.Blur()
		e.editSubField = 0
	case "shift+tab", "up":
		e.ruleValueInput.Blur()
		e.editSubField = 1
	default:
		updated, cmd := e.ruleValueInput.Update(msg)
		e.ruleValueInput = updated
		return e, actionNone, cmd
	}
	return e, actionNone, nil
}

// handleRuleSelectionKey processes keys while the field or operator selector of
// the rule sub-form is focused.
func (e SmartRuleEditor) handleRuleSelectionKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return e.cancelRule()
	case "enter":
		e = e.confirmEditRule()
	case "tab", "j", "down":
		e = e.focusEditSubField((e.editSubField + 1) % 3)
	case "shift+tab", "k", "up":
		e = e.focusEditSubField((e.editSubField + 2) % 3)
	case "left":
		e = e.cycleEditSelection(-1)
	case "right":
		e = e.cycleEditSelection(1)
	}
	return e, actionNone, nil
}

// focusEditSubField moves the sub-form focus to sub, activating the value text
// input when that is where focus lands.
func (e SmartRuleEditor) focusEditSubField(sub int) SmartRuleEditor {
	if sub == 2 && !e.choiceValue() {
		e.ruleValueInput.Focus()
	}
	e.editSubField = sub
	return e
}

// cycleEditSelection steps the focused field or operator selector by delta,
// wrapping at both ends. Changing the field resets the operator to the first
// one valid for it.
func (e SmartRuleEditor) cycleEditSelection(delta int) SmartRuleEditor {
	switch e.editSubField {
	case 0:
		e.ruleFieldIdx = cycleIndex(e.ruleFieldIdx, delta, len(allRuleFields))
		e.ruleOpIdx = 0
		if e.currentField() == db.FieldFavorite {
			e.ruleValueInput.SetValue("true")
		}
		if e.currentField() == db.FieldTag {
			e.ruleValueInput.SetValue("")
		}
	case 1:
		e.ruleOpIdx = cycleIndex(e.ruleOpIdx, delta, len(validOpsForField(e.currentField())))
	}
	return e
}

// cycleIndex steps idx by delta (±1) within n entries, wrapping at both ends.
func cycleIndex(idx, delta, n int) int {
	if n <= 0 {
		return 0
	}
	if delta < 0 {
		if idx > 0 {
			return idx - 1
		}
		return n - 1
	}
	return (idx + 1) % n
}

func (e SmartRuleEditor) confirmEditRule() SmartRuleEditor {
	field := e.currentField()
	op := e.currentOp()
	value, err := normalizeRuleValue(field, e.ruleValueInput.Value())
	if err != nil {
		e.errorText = err.Error()
		e = e.focusEditSubField(2)
		return e
	}
	e.ruleValueInput.Blur()
	rule := db.SmartRule{Field: field, Operator: op, Value: value}
	if e.editingRuleIdx >= 0 && e.editingRuleIdx < len(e.rules) {
		rules := make([]db.SmartRule, len(e.rules))
		copy(rules, e.rules)
		rules[e.editingRuleIdx] = rule
		e.rules = rules
	} else {
		e.rules = append(e.rules, rule)
		e.ruleCursor = len(e.rules) - 1
	}
	e.step = stepRules
	return e
}

func (e SmartRuleEditor) handleSortKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	if e.sortCursor == 2 {
		return e.handleMaxTracksKey(msg)
	}
	return e.handleSortSelectionKey(msg)
}

// handleMaxTracksKey processes keys while the max-tracks text input is focused.
func (e SmartRuleEditor) handleMaxTracksKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		e.maxTracksInput.Blur()
		e.step = stepRules
	case "enter":
		return e.commit()
	case "tab", "down":
		e.maxTracksInput.Blur()
		e.sortCursor = 0
	case "shift+tab", "up":
		e.maxTracksInput.Blur()
		e.sortCursor = 1
	default:
		updated, cmd := e.maxTracksInput.Update(msg)
		e.maxTracksInput = updated
		return e, actionNone, cmd
	}
	return e, actionNone, nil
}

// handleSortSelectionKey processes keys while the sort-field or sort-direction
// selector is focused.
func (e SmartRuleEditor) handleSortSelectionKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		e.step = stepRules
	case "enter":
		return e.commit()
	case "tab", "j", "down":
		e = e.focusSortCursor((e.sortCursor + 1) % 3)
	case "shift+tab", "k", "up":
		e = e.focusSortCursor((e.sortCursor + 2) % 3)
	case "left":
		e = e.cycleSortSelection(-1)
	case "right":
		e = e.cycleSortSelection(1)
	}
	return e, actionNone, nil
}

// focusSortCursor moves the sort-step focus to cursor, activating the
// max-tracks text input when that is where focus lands.
func (e SmartRuleEditor) focusSortCursor(cursor int) SmartRuleEditor {
	if cursor == 2 {
		e.maxTracksInput.Focus()
	}
	e.sortCursor = cursor
	return e
}

// cycleSortSelection steps the focused sort selector by delta. The sort field
// wraps; the direction is a two-way toggle, so delta does not affect it.
func (e SmartRuleEditor) cycleSortSelection(delta int) SmartRuleEditor {
	switch e.sortCursor {
	case 0:
		e.sortByIdx = cycleIndex(e.sortByIdx, delta, len(sortByOptions))
	case 1:
		e.sortDir = 1 - e.sortDir
	}
	return e
}
