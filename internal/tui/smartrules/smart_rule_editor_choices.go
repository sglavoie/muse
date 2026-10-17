package smartrules

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

type ruleTagPicker struct {
	input  textinput.Model
	cursor int
}

// WithTags snapshots the catalog and refreshes labels without changing identity.
func (e SmartRuleEditor) WithTags(tags []db.TagRow) SmartRuleEditor {
	clean := !e.hasChanges()
	e.tags = append([]db.TagRow(nil), tags...)
	e.rules = refreshTagLabels(e.rules, tags)
	if clean {
		e = e.rememberInitial()
	}
	return e
}

func refreshTagLabels(rules []db.SmartRule, tags []db.TagRow) []db.SmartRule {
	rules = append([]db.SmartRule(nil), rules...)
	for i := range rules {
		rules[i].Rules = refreshTagLabels(rules[i].Rules, tags)
		if rules[i].Field != db.FieldTag {
			continue
		}
		for _, tag := range tags {
			if tag.ID == rules[i].TagID || (rules[i].TagID == 0 && strings.EqualFold(tag.Name, rules[i].Value)) {
				rules[i].TagID, rules[i].Value = tag.ID, tag.Name
				break
			}
		}
	}
	return rules
}

func (e SmartRuleEditor) choiceValue() bool {
	return e.currentField() == db.FieldFavorite || e.currentField() == db.FieldTag
}

func (e SmartRuleEditor) handleChoiceValueKey(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return e.cancelRule()
	case "tab", "down":
		e.editSubField = 0
	case "shift+tab", "up":
		e.editSubField = 1
	case "left", "right", "space", "enter":
		return e.chooseValue(msg.String())
	}
	return e, actionNone, nil
}

func (e SmartRuleEditor) chooseValue(key string) (SmartRuleEditor, Action, tea.Cmd) {
	if e.currentField() == db.FieldTag {
		e.tagPicker = &ruleTagPicker{input: newEditorInput("")}
		for i, tag := range e.tags {
			if tag.Name == e.ruleValueInput.Value() {
				e.tagPicker.cursor = i
				break
			}
		}
		return e, actionNone, nil
	}
	if key == "enter" {
		return e.confirmEditRule(), actionNone, nil
	}
	value := e.ruleValueInput.Value()
	e.ruleValueInput.SetValue(strconv.FormatBool(value != "true" && value != "1"))
	return e, actionNone, nil
}

func (e SmartRuleEditor) matchingTags() []db.TagRow {
	query := strings.ToLower(e.tagPicker.input.Value())
	var matches []db.TagRow
	for _, tag := range e.tags {
		if strings.Contains(strings.ToLower(tag.Name), query) {
			matches = append(matches, tag)
		}
	}
	return matches
}

func (e SmartRuleEditor) handleTagPicker(msg tea.KeyPressMsg) (SmartRuleEditor, Action, tea.Cmd) {
	picker := *e.tagPicker
	matches := e.matchingTags()
	var cmd tea.Cmd
	switch msg.String() {
	case "esc":
		e.tagPicker = nil
		return e, actionNone, nil
	case "enter":
		if len(matches) > 0 {
			tag := matches[min(picker.cursor, len(matches)-1)]
			e.ruleValueInput.SetValue(tag.Name)
			e.ruleTagID = tag.ID
			e.tagPicker = nil
			e.editSubField = 0
		}
		return e, actionNone, nil
	case "up":
		picker.cursor = max(0, picker.cursor-1)
	case "down":
		picker.cursor = min(max(0, len(matches)-1), picker.cursor+1)
	default:
		previous := picker.input.Value()
		picker.input, cmd = picker.input.Update(msg)
		if picker.input.Value() != previous {
			picker.cursor = 0
		}
	}
	e.tagPicker = &picker
	return e, actionNone, cmd
}

func (e SmartRuleEditor) renderTagPicker(width, height int, s styles.Styles) string {
	matches := e.matchingTags()
	picker := *e.tagPicker
	picker.input.SetWidth(max(1, min(44, width-12)))
	rows := []overlay.Row{{Label: "Search:", Text: picker.input.View(), Styled: true}}
	visible := max(1, min(8, height-10))
	start := max(0, picker.cursor-visible+1)
	end := min(len(matches), start+visible)
	for i := start; i < end; i++ {
		marker := text.FocusMarker(i == picker.cursor)
		rows = append(rows, overlay.Row{Text: marker + matches[i].Name, Styled: i == picker.cursor})
	}
	if len(matches) == 0 {
		rows = append(rows, overlay.Row{Text: "No matching tags — create tags in the Tags view"})
	}
	title := " Choose tag "
	if len(matches) > 0 {
		title = fmt.Sprintf(" Choose tag · %d–%d of %d ", start+1, end, len(matches))
	}
	return (overlay.Box{Title: title, Rows: rows, Hint: "type to search · ↑↓ move · enter choose · esc back", MaxInnerWidth: 56}).Render(width, height, s)
}

func (e SmartRuleEditor) forwardTagPicker(msg tea.Msg) (SmartRuleEditor, tea.Cmd, bool) {
	picker := *e.tagPicker
	var cmd tea.Cmd
	previous := picker.input.Value()
	picker.input, cmd = picker.input.Update(msg)
	if picker.input.Value() != previous {
		picker.cursor = 0
	}
	e.tagPicker = &picker
	return e, cmd, true
}
