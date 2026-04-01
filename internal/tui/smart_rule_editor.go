package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// editorStep identifies the current step of the SmartRuleEditor state machine.
type editorStep int

const (
	stepName     editorStep = iota // playlist name entry
	stepRules                      // rule list management
	stepEditRule                   // sub-form for a single rule
	stepSort                       // sort/limit configuration
)

// ruleEditorAction is the result of a HandleKey call on a SmartRuleEditor.
type ruleEditorAction int

const (
	ruleEditorNone   ruleEditorAction = iota
	ruleEditorClose                   // esc from stepName → dismiss overlay
	ruleEditorCommit                  // enter on stepSort → persist and close
)

// SmartRuleEditor is the multi-step overlay for creating and editing smart playlists.
type SmartRuleEditor struct {
	isEdit bool
	editID int64 // playlist ID when editing an existing smart playlist

	step editorStep

	// stepName
	nameInput textinput.Model

	// stepRules
	rules       []db.SmartRule
	conjunction db.RuleConjunction
	ruleCursor  int

	// stepEditRule sub-form
	editingRuleIdx int // -1 = new rule, ≥0 = editing rules[idx]
	ruleFieldIdx   int // index into allRuleFields
	ruleOpIdx      int // index into validOpsForField(currentField)
	ruleValueInput textinput.Model
	editSubField   int // 0=field, 1=operator, 2=value

	// stepSort
	sortCursor     int // 0=sortBy, 1=sortDir, 2=maxTracks
	sortByIdx      int
	sortDir        int // 0=asc, 1=desc
	maxTracksInput textinput.Model
}

// allRuleFields is the ordered list of selectable rule fields.
var allRuleFields = []db.RuleField{
	db.FieldArtist, db.FieldTitle, db.FieldAlbum, db.FieldGenre,
	db.FieldDuration, db.FieldDateAdded, db.FieldPlayCount,
	db.FieldLastPlayed, db.FieldPath, db.FieldSize, db.FieldFavorite,
}

// sortByOptions are the selectable sort fields; index 0 means "no sort".
var sortByOptions = []string{"(default)", "artist", "title", "album", "duration", "date_added"}

// ruleFieldLabels maps RuleField to a display name.
var ruleFieldLabels = map[db.RuleField]string{
	db.FieldArtist:     "Artist",
	db.FieldTitle:      "Title",
	db.FieldAlbum:      "Album",
	db.FieldGenre:      "Genre",
	db.FieldDuration:   "Duration (s)",
	db.FieldDateAdded:  "Date added",
	db.FieldPlayCount:  "Play count",
	db.FieldLastPlayed: "Last played",
	db.FieldPath:       "Path",
	db.FieldSize:       "Size (bytes)",
	db.FieldFavorite:   "Is favorite",
}

// ruleOpLabels maps RuleOperator to a display label.
var ruleOpLabels = map[db.RuleOperator]string{
	db.OpEquals:      "equals",
	db.OpNotEquals:   "not equals",
	db.OpContains:    "contains",
	db.OpNotContains: "doesn't contain",
	db.OpStartsWith:  "starts with",
	db.OpEndsWith:    "ends with",
	db.OpGreaterThan: ">",
	db.OpLessThan:    "<",
	db.OpInLast:      "in last N days",
}

// ruleFieldType categorises a RuleField for operator selection.
func ruleFieldType(f db.RuleField) string {
	switch f {
	case db.FieldDuration, db.FieldPlayCount, db.FieldSize:
		return "numeric"
	case db.FieldDateAdded, db.FieldLastPlayed:
		return "date"
	case db.FieldFavorite:
		return "bool"
	default:
		return "string"
	}
}

// validOpsForField returns the operators valid for the given field.
func validOpsForField(f db.RuleField) []db.RuleOperator {
	switch ruleFieldType(f) {
	case "numeric":
		return []db.RuleOperator{db.OpEquals, db.OpNotEquals, db.OpGreaterThan, db.OpLessThan}
	case "date":
		return []db.RuleOperator{db.OpInLast}
	case "bool":
		return []db.RuleOperator{db.OpEquals, db.OpNotEquals}
	default: // string
		return []db.RuleOperator{
			db.OpContains, db.OpNotContains, db.OpEquals, db.OpNotEquals,
			db.OpStartsWith, db.OpEndsWith,
		}
	}
}

// ruleDisplayLabel returns a human-readable label for a leaf SmartRule.
func ruleDisplayLabel(r db.SmartRule) string {
	fieldLbl := ruleFieldLabels[r.Field]
	if fieldLbl == "" {
		fieldLbl = string(r.Field)
	}
	opLbl := ruleOpLabels[r.Operator]
	if opLbl == "" {
		opLbl = string(r.Operator)
	}
	return fmt.Sprintf("%s %s %q", fieldLbl, opLbl, r.Value)
}

func newEditorInput(initial string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	if initial != "" {
		ti.SetValue(initial)
		ti.CursorEnd()
	}
	ti.Focus()
	return ti
}

func newEditorInputDigits(initial string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Validate = func(s string) error {
		for _, r := range s {
			if r < '0' || r > '9' {
				return fmt.Errorf("digits only")
			}
		}
		return nil
	}
	if initial != "" {
		ti.SetValue(initial)
		ti.CursorEnd()
	}
	return ti
}

// newSmartRuleEditor creates an empty editor for creating a new smart playlist.
func newSmartRuleEditor() SmartRuleEditor {
	return SmartRuleEditor{
		conjunction:    db.ConjAnd,
		editingRuleIdx: -1,
		nameInput:      newEditorInput(""),
		maxTracksInput: newEditorInputDigits(""),
	}
}

// newSmartRuleEditorEdit creates an editor pre-populated for editing an existing playlist.
func newSmartRuleEditorEdit(id int64, name string, q db.SmartPlaylistQuery) SmartRuleEditor {
	maxTracksStr := ""
	if q.MaxTracks > 0 {
		maxTracksStr = strconv.Itoa(q.MaxTracks)
	}
	e := SmartRuleEditor{
		isEdit:         true,
		editID:         id,
		conjunction:    db.ConjAnd,
		editingRuleIdx: -1,
		nameInput:      newEditorInput(name),
		maxTracksInput: newEditorInputDigits(maxTracksStr),
	}
	if q.Root.Conjunction != "" {
		e.conjunction = q.Root.Conjunction
	}
	e.rules = append(e.rules, q.Root.Rules...)
	for i, opt := range sortByOptions {
		if opt == q.SortBy || (q.SortBy == "" && i == 0) {
			e.sortByIdx = i
			break
		}
	}
	if q.SortDir == "desc" {
		e.sortDir = 1
	}
	return e
}

// Name returns the trimmed playlist name from the name input.
func (e SmartRuleEditor) Name() string {
	return strings.TrimSpace(e.nameInput.Value())
}

// buildQuery assembles the SmartPlaylistQuery from the editor state.
func (e SmartRuleEditor) buildQuery() db.SmartPlaylistQuery {
	rules := e.rules
	if rules == nil {
		rules = []db.SmartRule{}
	}
	root := db.SmartRule{
		Conjunction: e.conjunction,
		Rules:       rules,
	}
	sortBy := sortByOptions[e.sortByIdx]
	if sortBy == "(default)" {
		sortBy = ""
	}
	var sortDir string
	if sortBy != "" {
		sortDir = "asc"
		if e.sortDir == 1 {
			sortDir = "desc"
		}
	}
	maxTracks := 0
	if s := strings.TrimSpace(e.maxTracksInput.Value()); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxTracks = n
		}
	}
	return db.SmartPlaylistQuery{
		Root:      root,
		SortBy:    sortBy,
		SortDir:   sortDir,
		MaxTracks: maxTracks,
	}
}

// currentField returns the RuleField at ruleFieldIdx.
func (e SmartRuleEditor) currentField() db.RuleField {
	if e.ruleFieldIdx < 0 || e.ruleFieldIdx >= len(allRuleFields) {
		return allRuleFields[0]
	}
	return allRuleFields[e.ruleFieldIdx]
}

// currentOp returns the RuleOperator at ruleOpIdx for the current field.
func (e SmartRuleEditor) currentOp() db.RuleOperator {
	ops := validOpsForField(e.currentField())
	if e.ruleOpIdx < 0 || e.ruleOpIdx >= len(ops) {
		return ops[0]
	}
	return ops[e.ruleOpIdx]
}

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
func (e SmartRuleEditor) HandleKey(msg tea.KeyPressMsg) (SmartRuleEditor, ruleEditorAction, tea.Cmd) {
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
	return e, ruleEditorNone, nil
}

func (e SmartRuleEditor) handleNameKey(msg tea.KeyPressMsg) (SmartRuleEditor, ruleEditorAction, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return e, ruleEditorClose, nil
	case "enter":
		if strings.TrimSpace(e.nameInput.Value()) != "" {
			e.step = stepRules
		}
		return e, ruleEditorNone, nil
	default:
		updated, cmd := e.nameInput.Update(msg)
		e.nameInput = updated
		return e, ruleEditorNone, cmd
	}
}

func (e SmartRuleEditor) handleRulesKey(msg tea.KeyPressMsg) (SmartRuleEditor, ruleEditorAction, tea.Cmd) {
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
			return e, ruleEditorNone, nil
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
	return e, ruleEditorNone, nil
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

func (e SmartRuleEditor) handleEditRuleKey(msg tea.KeyPressMsg) (SmartRuleEditor, ruleEditorAction, tea.Cmd) {
	if e.editSubField == 2 {
		// Value input receives most keys.
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
			return e, ruleEditorNone, cmd
		}
		return e, ruleEditorNone, nil
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
			e.ruleOpIdx = 0 // reset operator when field changes
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
	return e, ruleEditorNone, nil
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

func (e SmartRuleEditor) handleSortKey(msg tea.KeyPressMsg) (SmartRuleEditor, ruleEditorAction, tea.Cmd) {
	if e.sortCursor == 2 {
		// maxTracks input: ValidateFunc rejects non-digits; navigation keys are intercepted.
		switch msg.String() {
		case "esc":
			e.maxTracksInput.Blur()
			e.step = stepRules
		case "enter":
			return e, ruleEditorCommit, nil
		case "tab", "down":
			e.maxTracksInput.Blur()
			e.sortCursor = 0
		case "shift+tab", "up":
			e.maxTracksInput.Blur()
			e.sortCursor = 1
		default:
			updated, cmd := e.maxTracksInput.Update(msg)
			e.maxTracksInput = updated
			return e, ruleEditorNone, cmd
		}
		return e, ruleEditorNone, nil
	}
	switch msg.String() {
	case "esc":
		e.step = stepRules
	case "enter":
		return e, ruleEditorCommit, nil
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
	return e, ruleEditorNone, nil
}

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
	if e.isEdit {
		title = " Edit Smart Playlist "
	}
	box := OverlayBox{
		Title: title,
		Rows: []OverlayRow{
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
	rows := []OverlayRow{
		{Text: fmt.Sprintf("Match %s of the following:", conjLabel)},
		{Blank: true},
	}
	if len(e.rules) == 0 {
		rows = append(rows, OverlayRow{Text: "(no rules — all tracks match)"})
	} else {
		for i, r := range e.rules {
			marker := "  "
			if i == e.ruleCursor {
				marker = "▸ "
			}
			rows = append(rows, OverlayRow{
				Text:   marker + ruleDisplayLabel(r),
				Styled: i == e.ruleCursor,
			})
		}
	}
	rows = append(rows, OverlayRow{Blank: true})
	hint := "a add · enter edit · d del · tab AND/OR · → next"
	if len(e.rules) == 0 {
		hint = "a add"
	}
	box := OverlayBox{
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

	box := OverlayBox{
		Title: title,
		Rows: []OverlayRow{
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
	if e.isEdit {
		saveLabel = "save"
	}

	box := OverlayBox{
		Title: " Sort & Limit ",
		Rows: []OverlayRow{
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
