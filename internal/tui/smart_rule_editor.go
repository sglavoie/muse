package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
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
