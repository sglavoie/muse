package smartrules

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func editorKey(e SmartRuleEditor, code rune) (SmartRuleEditor, Action) {
	e, act, _ := e.HandleKey(tea.KeyPressMsg{Code: code})
	return e, act
}

func ruleEditor(field db.RuleField, value string) SmartRuleEditor {
	q := db.SmartPlaylistQuery{Root: db.SmartRule{Conjunction: db.ConjAnd, Rules: []db.SmartRule{{Field: field, Operator: validOpsForField(field)[0], Value: value}}}}
	return NewSmartRuleEditorEdit(1, "Focus", q).startEditRule(0).focusEditSubField(2)
}

func TestRuleValidationKeepsInvalidInputAndNormalizesDuration(t *testing.T) {
	cases := []struct {
		field       db.RuleField
		value, want string
		valid       bool
	}{
		{db.FieldDuration, "abc", "", false}, {db.FieldDuration, "-1", "", false},
		{db.FieldDuration, "NaN", "", false}, {db.FieldDuration, "Inf", "", false},
		{db.FieldDuration, "3:60", "", false}, {db.FieldDuration, "", "", false},
		{db.FieldDuration, "3:30", "210", true}, {db.FieldDuration, "1:02:03", "3723", true},
		{db.FieldDuration, "1.25", "1.25", true}, {db.FieldDuration, "0", "0", true},
		{db.FieldPlayCount, "-2", "", false}, {db.FieldPlayCount, "2.5", "", false},
		{db.FieldPlayCount, "0", "0", true}, {db.FieldSize, "abc", "", false},
		{db.FieldSize, "4096", "4096", true}, {db.FieldDateAdded, "0", "", false},
		{db.FieldLastPlayed, "7", "7", true}, {db.FieldDateAdded, "999999999999999999999", "", false},
		{db.FieldArtist, "", "", true},
	}
	for _, tc := range cases {
		t.Run(string(tc.field)+"/"+tc.value, func(t *testing.T) {
			e := ruleEditor(tc.field, "1")
			e.ruleValueInput.SetValue(tc.value)
			e, _ = editorKey(e, tea.KeyEnter)
			if tc.valid {
				if e.step != stepRules || e.rules[0].Value != tc.want {
					t.Fatalf("did not confirm normalized rule: %+v, error %s", e.rules, e.errorText)
				}
			} else {
				if e.step != stepEditRule || e.errorText == "" || e.ruleValueInput.Value() != tc.value || !e.ruleValueInput.Focused() {
					t.Fatalf("invalid draft lost: %+v", e)
				}
				if e.rules[0].Value != "1" {
					t.Fatal("invalid input changed saved rule")
				}
				assertEditorFits(t, e)
			}
		})
	}
}

func TestSmartPlaylistDiscardPreservesDraftAndRestoredValuesClose(t *testing.T) {
	e := NewSmartRuleEditor()
	e.nameInput.SetValue("My draft")
	e, act := editorKey(e, tea.KeyEscape)
	if act == ActionClose || !e.discard.Active {
		t.Fatal("edited form closed")
	}
	e, act = editorKey(e, tea.KeyEnter)
	if act == ActionClose || e.nameInput.Value() != "My draft" {
		t.Fatal("default did not keep editing")
	}
	e.nameInput.SetValue("")
	e, act = editorKey(e, tea.KeyEscape)
	if act != ActionClose {
		t.Fatal("restored form should close immediately")
	}
	e.nameInput.SetValue("My draft")
	e, _ = editorKey(e, tea.KeyEscape)
	e, _ = editorKey(e, tea.KeyTab)
	_, act = editorKey(e, tea.KeyEnter)
	if act != ActionClose {
		t.Fatal("explicit discard did not close")
	}
}

func TestRuleDiscardLeavesOtherDraftChanges(t *testing.T) {
	e := ruleEditor(db.FieldArtist, "Original")
	e.nameInput.SetValue("Renamed")
	e.ruleValueInput.SetValue("Edited")
	e, _ = editorKey(e, tea.KeyEscape)
	if !e.discard.Active {
		t.Fatal("rule draft unprotected")
	}
	assertEditorFits(t, e)
	e, _ = editorKey(e, tea.KeyEscape)
	if e.step != stepEditRule || e.ruleValueInput.Value() != "Edited" {
		t.Fatal("keep editing lost rule")
	}
	e, _ = editorKey(e, tea.KeyEscape)
	e, _ = editorKey(e, tea.KeyTab)
	e, _ = editorKey(e, tea.KeyEnter)
	if e.step != stepRules || e.rules[0].Value != "Original" || e.Name() != "Renamed" {
		t.Fatal("discard changed other fields")
	}
	e, _ = editorKey(e, tea.KeyEscape)
	e, _ = editorKey(e, tea.KeyEscape)
	if !e.discard.Active {
		t.Fatal("playlist name still needs protection")
	}
}

func TestSmartLimitValidationAndPendingSave(t *testing.T) {
	for _, value := range []string{"abc", "-1", "1.5", "999999999999999999999"} {
		e := NewSmartRuleEditor()
		e.step = stepSort
		e.maxTracksInput.SetValue(value)
		e, act := editorKey(e, tea.KeyEnter)
		if act == ActionCommit || e.errorText == "" {
			t.Fatalf("invalid limit %q accepted", value)
		}
		assertEditorFits(t, e)
	}
	e := NewSmartRuleEditor().Submit()
	_, act := editorKey(e, tea.KeyEnter)
	if act != actionNone {
		t.Fatal("duplicate submit")
	}
	_, act = editorKey(e, tea.KeyEscape)
	if act != actionNone {
		t.Fatal("pending form closed")
	}
}

func TestFavoriteChoiceAndTagPicker(t *testing.T) {
	e := ruleEditor(db.FieldFavorite, "true")
	e, _ = editorKey(e, tea.KeyRight)
	if !strings.Contains(e.Render(80, 24, styles.NewStyles(true)), "No") {
		t.Fatal("missing No choice")
	}
	e, _ = editorKey(e, tea.KeyEnter)
	if e.rules[0].Value != "false" {
		t.Fatal("No did not become false")
	}
	tags := []db.TagRow{{Name: "jazz focus"}, {Name: "Rock"}, {Name: "東京"}}
	e = ruleEditor(db.FieldTag, "Rock").WithTags(tags)
	e, _ = editorKey(e, tea.KeyEnter)
	for _, r := range "jazz f" {
		e, _, _ = e.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if len(e.matchingTags()) != 1 {
		t.Fatal("tag search must accept j and spaces")
	}
	e, _ = editorKey(e, tea.KeyEnter)
	e, _ = editorKey(e, tea.KeyEnter)
	if e.step != stepRules || e.rules[0].Field != db.FieldTag || e.rules[0].Value != "jazz focus" {
		t.Fatalf("tag rule lost: %+v", e.rules)
	}
	if len(validOpsForField(db.FieldTag)) != 2 {
		t.Fatal("tag must offer equals/not equals only")
	}
}

func TestTagPickerScrollEmptyAndCancel(t *testing.T) {
	var tags []db.TagRow
	for i := range 30 {
		tags = append(tags, db.TagRow{Name: fmt.Sprintf("Tag %02d", i)})
	}
	e := ruleEditor(db.FieldTag, "Tag 00").WithTags(tags)
	e, _ = editorKey(e, tea.KeyEnter)
	for range 25 {
		e, _ = editorKey(e, tea.KeyDown)
	}
	e, _, _ = e.ForwardMsg(struct{}{})
	e, _ = editorKey(e, tea.KeyLeft)
	if e.tagPicker.cursor != 25 {
		t.Fatal("non-editing input reset tag cursor")
	}
	assertEditorFits(t, e)
	if !strings.Contains(e.Render(60, 14, styles.NewStyles(true)), "Tag 25") {
		t.Fatal("focused tag scrolled out of view")
	}
	e, _ = editorKey(e, tea.KeyEscape)
	if e.ruleValueInput.Value() != "Tag 00" {
		t.Fatal("cancel changed tag")
	}
	e = e.WithTags(nil)
	e, _ = editorKey(e, tea.KeyEnter)
	e, _ = editorKey(e, tea.KeyEnter)
	if e.tagPicker == nil {
		t.Fatal("empty picker should stay open")
	}
	assertEditorFits(t, e)
}

func assertEditorFits(t *testing.T, e SmartRuleEditor) {
	t.Helper()
	screen := ansi.Strip(e.Render(60, 14, styles.NewStyles(true)))
	if len(strings.Split(strings.TrimSuffix(screen, "\n"), "\n")) > 14 {
		t.Fatalf("too tall:\n%s", screen)
	}
	for _, line := range strings.Split(screen, "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Fatalf("too wide:\n%s", screen)
		}
	}
}
