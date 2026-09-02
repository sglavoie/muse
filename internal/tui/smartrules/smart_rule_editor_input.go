package smartrules

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
)

// ForwardMsg routes a generic tea.Msg to the currently focused text input.
// Returns false if no text input is active in the current step.
func (e SmartRuleEditor) ForwardMsg(msg tea.Msg) (SmartRuleEditor, tea.Cmd, bool) {
	switch e.step {
	case stepName:
		updated, cmd := e.nameInput.Update(msg)
		e.nameInput = updated
		return e, cmd, true
	case stepEditRule:
		if e.editSubField == 2 {
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
	return e, ActionNone, nil
}

func (e SmartRuleEditor) handleNameKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return e, ActionClose, nil
	case "enter":
		if strings.TrimSpace(e.nameInput.Value()) != "" {
			e.step = stepRules
		}
		return e, ActionNone, nil
	default:
		updated, cmd := e.nameInput.Update(msg)
		e.nameInput = updated
		return e, ActionNone, cmd
	}
}

func (e SmartRuleEditor) handleRulesKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc", "left":
		e.step = stepName
	case "enter":
		if len(e.rules) == 0 {
			e.step = stepSort
		} else {
			e = e.startEditRule(e.ruleCursor)
		}
	case "right":
		if len(e.rules) == 0 {
			return e, ActionNone, nil
		}
		e.step = stepSort
	case "a":
		e = e.startNewRule()
	case "d":
		if len(e.rules) > 0 && e.ruleCursor < len(e.rules) {
			e.rules = append(e.rules[:e.ruleCursor], e.rules[e.ruleCursor+1:]...)
			if e.ruleCursor > 0 && e.ruleCursor >= len(e.rules) {
				e.ruleCursor = len(e.rules) - 1
			}
		}
	case "tab":
		if e.conjunction == db.ConjAnd {
			e.conjunction = db.ConjOr
		} else {
			e.conjunction = db.ConjAnd
		}
	case "j", "down":
		if e.ruleCursor < len(e.rules)-1 {
			e.ruleCursor++
		}
	case "k", "up":
		if e.ruleCursor > 0 {
			e.ruleCursor--
		}
	}
	return e, ActionNone, nil
}

func (e SmartRuleEditor) startNewRule() SmartRuleEditor {
	e.editingRuleIdx = -1
	e.ruleFieldIdx = 0
	e.ruleOpIdx = 0
	e.ruleValueInput = newEditorInput("")
	e.ruleValueInput.Blur()
	e.editSubField = 0
	e.step = stepEditRule
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
	return e
}

func (e SmartRuleEditor) handleEditRuleKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	if e.editSubField == 2 {
		switch msg.String() {
		case "esc":
			e.ruleValueInput.Blur()
			e.step = stepRules
		case "enter":
			e.ruleValueInput.Blur()
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
			return e, ActionNone, cmd
		}
		return e, ActionNone, nil
	}

	switch msg.String() {
	case "esc":
		e.step = stepRules
	case "enter":
		e = e.confirmEditRule()
	case "tab", "j", "down":
		newSub := (e.editSubField + 1) % 3
		if newSub == 2 {
			e.ruleValueInput.Focus()
		}
		e.editSubField = newSub
	case "shift+tab", "k", "up":
		newSub := (e.editSubField + 2) % 3
		if newSub == 2 {
			e.ruleValueInput.Focus()
		}
		e.editSubField = newSub
	case "left":
		switch e.editSubField {
		case 0:
			if e.ruleFieldIdx > 0 {
				e.ruleFieldIdx--
			} else {
				e.ruleFieldIdx = len(allRuleFields) - 1
			}
			e.ruleOpIdx = 0
		case 1:
			ops := validOpsForField(e.currentField())
			if e.ruleOpIdx > 0 {
				e.ruleOpIdx--
			} else {
				e.ruleOpIdx = len(ops) - 1
			}
		}
	case "right":
		switch e.editSubField {
		case 0:
			e.ruleFieldIdx = (e.ruleFieldIdx + 1) % len(allRuleFields)
			e.ruleOpIdx = 0
		case 1:
			ops := validOpsForField(e.currentField())
			e.ruleOpIdx = (e.ruleOpIdx + 1) % len(ops)
		}
	}
	return e, ActionNone, nil
}

func (e SmartRuleEditor) confirmEditRule() SmartRuleEditor {
	field := e.currentField()
	op := e.currentOp()
	value := e.ruleValueInput.Value()
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
		switch msg.String() {
		case "esc":
			e.maxTracksInput.Blur()
			e.step = stepRules
		case "enter":
			return e, ActionCommit, nil
		case "tab", "down":
			e.maxTracksInput.Blur()
			e.sortCursor = 0
		case "shift+tab", "up":
			e.maxTracksInput.Blur()
			e.sortCursor = 1
		default:
			updated, cmd := e.maxTracksInput.Update(msg)
			e.maxTracksInput = updated
			return e, ActionNone, cmd
		}
		return e, ActionNone, nil
	}

	switch msg.String() {
	case "esc":
		e.step = stepRules
	case "enter":
		return e, ActionCommit, nil
	case "tab", "j", "down":
		newCursor := (e.sortCursor + 1) % 3
		if newCursor == 2 {
			e.maxTracksInput.Focus()
		}
		e.sortCursor = newCursor
	case "shift+tab", "k", "up":
		newCursor := (e.sortCursor + 2) % 3
		if newCursor == 2 {
			e.maxTracksInput.Focus()
		}
		e.sortCursor = newCursor
	case "left":
		switch e.sortCursor {
		case 0:
			if e.sortByIdx > 0 {
				e.sortByIdx--
			} else {
				e.sortByIdx = len(sortByOptions) - 1
			}
		case 1:
			e.sortDir = 1 - e.sortDir
		}
	case "right":
		switch e.sortCursor {
		case 0:
			e.sortByIdx = (e.sortByIdx + 1) % len(sortByOptions)
		case 1:
			e.sortDir = 1 - e.sortDir
		}
	}
	return e, ActionNone, nil
}
