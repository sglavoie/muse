package smartrules

import "github.com/sglavoie/muse/internal/db"

// RemapTag keeps an open draft and its discard baseline aligned after a merge.
func (e SmartRuleEditor) RemapTag(sourceID int64, destination db.TagRow) SmartRuleEditor {
	query := e.BuildQuery()
	query.Root = db.RemapSmartRuleTag(query.Root, sourceID, destination)
	e.rules = query.Root.Rules
	if initial, err := db.ParseSmartPlaylistQuery(e.initialQuery); err == nil {
		initial.Root = db.RemapSmartRuleTag(initial.Root, sourceID, destination)
		e.initialQuery, _ = db.MarshalSmartPlaylistQuery(initial)
	}
	e.initialRule = db.RemapSmartRuleTag(e.initialRule, sourceID, destination)
	if e.ruleTagID == sourceID {
		e.ruleTagID = destination.ID
		e.ruleValueInput.SetValue(destination.Name)
	}
	return e
}

func (e SmartRuleEditor) missingTag(rule db.SmartRule) bool {
	for _, child := range rule.Rules {
		if e.missingTag(child) {
			return true
		}
	}
	if rule.Field != db.FieldTag {
		return false
	}
	for _, tag := range e.tags {
		if tag.ID == rule.TagID {
			return false
		}
	}
	return true
}

func (e SmartRuleEditor) ruleLabel(rule db.SmartRule) string {
	label := ruleDisplayLabel(rule)
	if e.missingTag(rule) {
		label += " [missing tag — choose replacement]"
	}
	return label
}
