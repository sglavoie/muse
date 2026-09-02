package smartrules

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Render returns the centered overlay string for the current editor step.
func (e SmartRuleEditor) Render(termWidth, termHeight int, s styles.Styles) string {
	switch e.step {
	case stepName:
		return e.renderNameStep(termWidth, termHeight, s)
	case stepRules:
		return e.renderRulesStep(termWidth, termHeight, s)
	case stepEditRule:
		return e.renderEditRuleStep(termWidth, termHeight, s)
	case stepSort:
		return e.renderSortStep(termWidth, termHeight, s)
	}
	return ""
}

func (e SmartRuleEditor) renderNameStep(termWidth, termHeight int, s styles.Styles) string {
	title := " Create Smart Playlist "
	if e.IsEdit {
		title = " Edit Smart Playlist "
	}
	box := overlay.Box{
		Title: title,
		Rows: []overlay.Row{
			{Blank: true},
			{Label: "Name:", Text: e.nameInput.View(), Styled: true},
			{Blank: true},
		},
		Hint: "enter to continue · esc to cancel",
	}
	return box.Render(termWidth, termHeight, s)
}

func (e SmartRuleEditor) renderRulesStep(termWidth, termHeight int, s styles.Styles) string {
	conjLabel := "AND"
	if e.conjunction == db.ConjOr {
		conjLabel = "OR"
	}
	rows := []overlay.Row{
		{Text: fmt.Sprintf("Match %s of the following:", conjLabel)},
		{Blank: true},
	}
	if len(e.rules) == 0 {
		rows = append(rows, overlay.Row{Text: "(no rules — all tracks match)"})
	} else {
		for i, r := range e.rules {
			marker := "  "
			if i == e.ruleCursor {
				marker = "▸ "
			}
			rows = append(rows, overlay.Row{
				Text:   marker + ruleDisplayLabel(r),
				Styled: i == e.ruleCursor,
			})
		}
	}
	rows = append(rows, overlay.Row{Blank: true})
	hint := "a add · enter edit · d del · tab AND/OR · → next"
	if len(e.rules) == 0 {
		hint = "a add"
	}
	box := overlay.Box{
		Title:         " Rules ",
		Rows:          rows,
		Hint:          hint,
		MaxInnerWidth: 60,
	}
	return box.Render(termWidth, termHeight, s)
}

func (e SmartRuleEditor) renderEditRuleStep(termWidth, termHeight int, s styles.Styles) string {
	field := e.currentField()
	op := e.currentOp()

	fieldLabel := ruleFieldLabels[field]
	if fieldLabel == "" {
		fieldLabel = string(field)
	}
	opLabel := ruleOpLabels[op]
	if opLabel == "" {
		opLabel = string(op)
	}

	var valueText string
	if e.editSubField == 2 {
		valueText = e.ruleValueInput.View()
	} else {
		v := e.ruleValueInput.Value()
		if v == "" {
			valueText = "(empty)"
		} else {
			valueText = v
		}
	}

	title := " Add Rule "
	if e.editingRuleIdx >= 0 {
		title = " Edit Rule "
	}

	box := overlay.Box{
		Title: title,
		Rows: []overlay.Row{
			{Blank: true},
			{Label: "Field:", Text: "◀ " + fieldLabel + " ▶", Styled: e.editSubField == 0},
			{Label: "Operator:", Text: "◀ " + opLabel + " ▶", Styled: e.editSubField == 1},
			{Label: "Value:", Text: valueText, Styled: e.editSubField == 2},
			{Blank: true},
		},
		Hint: "↑↓ move · ←→ cycle/edit · enter confirm · esc cancel",
	}
	return box.Render(termWidth, termHeight, s)
}

func (e SmartRuleEditor) renderSortStep(termWidth, termHeight int, s styles.Styles) string {
	sortByLabel := sortByOptions[e.sortByIdx]
	sortDirLabel := "asc"
	if e.sortDir == 1 {
		sortDirLabel = "desc"
	}

	var maxTracksText string
	switch {
	case e.sortCursor == 2:
		maxTracksText = e.maxTracksInput.View()
	case strings.TrimSpace(e.maxTracksInput.Value()) == "":
		maxTracksText = "(unlimited)"
	default:
		maxTracksText = e.maxTracksInput.Value()
	}

	saveLabel := "create"
	if e.IsEdit {
		saveLabel = "save"
	}

	box := overlay.Box{
		Title: " Sort & Limit ",
		Rows: []overlay.Row{
			{Blank: true},
			{Label: "Sort by:", Text: "◀ " + sortByLabel + " ▶", Styled: e.sortCursor == 0},
			{Label: "Direction:", Text: "◀ " + sortDirLabel + " ▶", Styled: e.sortCursor == 1},
			{Label: "Max tracks:", Text: maxTracksText, Styled: e.sortCursor == 2},
			{Blank: true},
		},
		Hint: fmt.Sprintf("↑↓ move · ←→ cycle · enter %s · esc back", saveLabel),
	}
	return box.Render(termWidth, termHeight, s)
}
