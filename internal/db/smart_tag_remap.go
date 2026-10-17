package db

import "slices"

// RemapSmartRuleTag updates a live query snapshot after a tag merge, preserving
// other predicates and leaving the caller's slices untouched.
func RemapSmartRuleTag(rule SmartRule, sourceID int64, destination TagRow) SmartRule {
	rule.Rules = slices.Clone(rule.Rules)
	for i, child := range rule.Rules {
		rule.Rules[i] = RemapSmartRuleTag(child, sourceID, destination)
	}
	if rule.Field == FieldTag && rule.TagID == sourceID {
		rule.TagID, rule.Value = destination.ID, destination.Name
	}
	return rule
}
