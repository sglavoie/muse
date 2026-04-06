package db

import (
	"encoding/json"
	"fmt"
)

// RuleField identifies a track attribute that can be filtered.
type RuleField string

const (
	FieldArtist     RuleField = "artist"
	FieldTitle      RuleField = "title"
	FieldAlbum      RuleField = "album"
	FieldGenre      RuleField = "genre"
	FieldDuration   RuleField = "duration"
	FieldDateAdded  RuleField = "date_added"
	FieldPlayCount  RuleField = "play_count"
	FieldPath       RuleField = "path"
	FieldSize       RuleField = "size"
	FieldFavorite   RuleField = "is_favorite"
	FieldLastPlayed RuleField = "last_played"
	FieldTag        RuleField = "has_tag"
)

// RuleOperator identifies the comparison operation for a leaf rule.
type RuleOperator string

const (
	OpEquals      RuleOperator = "equals"
	OpNotEquals   RuleOperator = "not_equals"
	OpContains    RuleOperator = "contains"
	OpNotContains RuleOperator = "not_contains"
	OpStartsWith  RuleOperator = "starts_with"
	OpEndsWith    RuleOperator = "ends_with"
	OpGreaterThan RuleOperator = "greater_than"
	OpLessThan    RuleOperator = "less_than"
	OpInLast      RuleOperator = "in_last"
)

// RuleConjunction combines child rules in a group.
type RuleConjunction string

const (
	ConjAnd RuleConjunction = "and"
	ConjOr  RuleConjunction = "or"
)

// SmartRule is a recursive rule node. It is either:
//   - a leaf: Field + Operator + Value are set, Rules and Conjunction are empty.
//   - a group: Conjunction + Rules are set, Field/Operator/Value are empty.
type SmartRule struct {
	Field       RuleField       `json:"field,omitempty"`
	Operator    RuleOperator    `json:"operator,omitempty"`
	Value       string          `json:"value,omitempty"`
	Conjunction RuleConjunction `json:"conjunction,omitempty"`
	Rules       []SmartRule     `json:"rules,omitempty"`
}

// SmartPlaylistQuery is the top-level document stored in the playlists.query column.
type SmartPlaylistQuery struct {
	Root      SmartRule `json:"root"`
	SortBy    string    `json:"sort_by,omitempty"`  // "artist", "title", "date_added", "duration"
	SortDir   string    `json:"sort_dir,omitempty"` // "asc", "desc"
	MaxTracks int       `json:"max_tracks,omitempty"`
}

// validFields is the set of recognised RuleField values.
var validFields = map[RuleField]bool{
	FieldArtist: true, FieldTitle: true, FieldAlbum: true, FieldGenre: true,
	FieldDuration: true, FieldDateAdded: true, FieldPlayCount: true,
	FieldPath: true, FieldSize: true, FieldFavorite: true,
	FieldLastPlayed: true, FieldTag: true,
}

// validOperators is the set of recognised RuleOperator values.
var validOperators = map[RuleOperator]bool{
	OpEquals: true, OpNotEquals: true, OpContains: true, OpNotContains: true,
	OpStartsWith: true, OpEndsWith: true, OpGreaterThan: true, OpLessThan: true,
	OpInLast: true,
}

// ValidateSmartRule returns an error if the rule tree contains unknown
// fields/operators or violates leaf-vs-group invariants.
func ValidateSmartRule(r SmartRule) error {
	isLeaf := r.Field != "" || r.Operator != "" || r.Value != ""
	isGroup := r.Conjunction != "" || len(r.Rules) > 0

	if isLeaf && isGroup {
		return fmt.Errorf("smart rule: node cannot be both a leaf and a group")
	}
	if !isLeaf && !isGroup {
		return fmt.Errorf("smart rule: node must be either a leaf or a group")
	}

	if isLeaf {
		if !validFields[r.Field] {
			return fmt.Errorf("smart rule: unknown field %q", r.Field)
		}
		if !validOperators[r.Operator] {
			return fmt.Errorf("smart rule: unknown operator %q", r.Operator)
		}
		return nil
	}

	// group node
	if r.Conjunction != ConjAnd && r.Conjunction != ConjOr {
		return fmt.Errorf("smart rule: unknown conjunction %q", r.Conjunction)
	}
	for i, child := range r.Rules {
		if err := ValidateSmartRule(child); err != nil {
			return fmt.Errorf("smart rule: rules[%d]: %w", i, err)
		}
	}
	return nil
}

// ParseSmartPlaylistQuery unmarshals JSON from the playlists.query column into a
// SmartPlaylistQuery and validates the rule tree.
func ParseSmartPlaylistQuery(raw string) (SmartPlaylistQuery, error) {
	var q SmartPlaylistQuery
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return SmartPlaylistQuery{}, fmt.Errorf("smart playlist query: unmarshal: %w", err)
	}
	if err := ValidateSmartRule(q.Root); err != nil {
		return SmartPlaylistQuery{}, fmt.Errorf("smart playlist query: %w", err)
	}
	return q, nil
}

// MarshalSmartPlaylistQuery serialises q to JSON for storage in the database.
func MarshalSmartPlaylistQuery(q SmartPlaylistQuery) (string, error) {
	b, err := json.Marshal(q)
	if err != nil {
		return "", fmt.Errorf("smart playlist query: marshal: %w", err)
	}
	return string(b), nil
}
