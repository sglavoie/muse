package smartrules

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

func (e SmartRuleEditor) rememberInitial() SmartRuleEditor {
	e.initialName = e.nameInput.Value()
	e.initialQuery, _ = db.MarshalSmartPlaylistQuery(e.BuildQuery())
	e.initialLimit = e.maxTracksInput.Value()
	return e
}

func (e SmartRuleEditor) hasChanges() bool {
	query, _ := db.MarshalSmartPlaylistQuery(e.BuildQuery())
	return e.nameInput.Value() != e.initialName || query != e.initialQuery || e.maxTracksInput.Value() != e.initialLimit
}

func (e SmartRuleEditor) requestClose() (SmartRuleEditor, Action, tea.Cmd) {
	if e.hasChanges() {
		e.discard = overlay.DiscardPrompt{Active: true}
		e.discardRule = false
		return e, actionNone, nil
	}
	return e, ActionClose, nil
}

func (e SmartRuleEditor) draftRule() db.SmartRule {
	rule := db.SmartRule{Field: e.currentField(), Operator: e.currentOp(), Value: e.ruleValueInput.Value()}
	if rule.Field == db.FieldTag {
		rule.TagID = e.ruleTagID
	}
	return rule
}

func (e SmartRuleEditor) cancelRule() (SmartRuleEditor, Action, tea.Cmd) {
	draft := e.draftRule()
	if draft.Field != e.initialRule.Field || draft.Operator != e.initialRule.Operator || draft.Value != e.initialRule.Value || draft.TagID != e.initialRule.TagID {
		e.discard = overlay.DiscardPrompt{Active: true}
		e.discardRule = true
	} else {
		e.step = stepRules
	}
	return e, actionNone, nil
}

func (e SmartRuleEditor) handleDiscard(key string) (SmartRuleEditor, Action, tea.Cmd) {
	var discard bool
	e.discard, discard = e.discard.HandleKey(key)
	if !discard {
		return e, actionNone, nil
	}
	if e.discardRule {
		e.ruleValueInput.Blur()
		e.step = stepRules
		return e, actionNone, nil
	}
	return e, ActionClose, nil
}

// Submit freezes the form while its write is pending, preventing duplicate saves.
func (e SmartRuleEditor) Submit() SmartRuleEditor {
	e.saving = true
	e.errorText = ""
	return e
}

// SaveFailed retains the complete draft and re-enables editing and retry.
func (e SmartRuleEditor) SaveFailed(err error) SmartRuleEditor {
	e.saving = false
	e.errorText = "Save failed: " + err.Error() + ". Enter to retry."
	return e
}

func (e SmartRuleEditor) commit() (SmartRuleEditor, Action, tea.Cmd) {
	if value := strings.TrimSpace(e.maxTracksInput.Value()); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			e.errorText = "Max tracks: use a whole number; blank or 0 means unlimited."
			return e, actionNone, nil
		}
	}
	return e, ActionCommit, nil
}

// normalizeRuleValue validates before leaving the rule form. Stored durations
// remain seconds, matching the database evaluator and existing playlists.
func normalizeRuleValue(field db.RuleField, value string) (string, error) {
	s := strings.TrimSpace(value)
	switch field {
	case db.FieldDuration:
		n, err := parseRuleDuration(s)
		if err != nil {
			return "", fmt.Errorf("duration: use seconds, m:ss, or h:mm:ss")
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	case db.FieldPlayCount, db.FieldSize:
		return wholeNumber(s, false)
	case db.FieldDateAdded, db.FieldLastPlayed:
		return wholeNumber(s, true)
	case db.FieldFavorite:
		if s == "true" || s == "1" {
			return "true", nil
		}
		return "false", nil
	case db.FieldTag:
		if value == "" {
			return "", fmt.Errorf("choose a tag; create tags in the Tags view")
		}
	}
	return value, nil
}

func wholeNumber(s string, positive bool) (string, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || strings.Trim(s, "0123456789") != "" {
		return "", fmt.Errorf("use a whole number of zero or more")
	}
	if positive && n == 0 {
		return "", fmt.Errorf("use a whole number of days greater than zero")
	}
	return strconv.FormatInt(n, 10), nil
}

// Existing rules can contain fractional seconds; clock notation follows Seek.
func parseRuleDuration(value string) (float64, error) {
	if strings.Contains(value, ":") {
		return overlay.ParseSeekTime(value)
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid duration")
	}
	return n, nil
}
