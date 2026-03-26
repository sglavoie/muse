package db_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- JSON roundtrip ---

func TestSmartPlaylistQuery_JSONRoundtrip(t *testing.T) {
	original := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjAnd,
			Rules: []db.SmartRule{
				{Field: db.FieldArtist, Operator: db.OpContains, Value: "Radiohead"},
				{Field: db.FieldDuration, Operator: db.OpLessThan, Value: "300"},
			},
		},
		SortBy:    "artist",
		SortDir:   "asc",
		MaxTracks: 50,
	}

	raw, err := db.MarshalSmartPlaylistQuery(original)
	if err != nil {
		t.Fatalf("MarshalSmartPlaylistQuery: %v", err)
	}

	got, err := db.ParseSmartPlaylistQuery(raw)
	if err != nil {
		t.Fatalf("ParseSmartPlaylistQuery: %v", err)
	}

	if got.SortBy != original.SortBy {
		t.Errorf("SortBy = %q, want %q", got.SortBy, original.SortBy)
	}
	if got.SortDir != original.SortDir {
		t.Errorf("SortDir = %q, want %q", got.SortDir, original.SortDir)
	}
	if got.MaxTracks != original.MaxTracks {
		t.Errorf("MaxTracks = %d, want %d", got.MaxTracks, original.MaxTracks)
	}
	if got.Root.Conjunction != db.ConjAnd {
		t.Errorf("Root.Conjunction = %q, want %q", got.Root.Conjunction, db.ConjAnd)
	}
	if len(got.Root.Rules) != 2 {
		t.Fatalf("Root.Rules len = %d, want 2", len(got.Root.Rules))
	}
	if got.Root.Rules[0].Field != db.FieldArtist {
		t.Errorf("Rules[0].Field = %q, want %q", got.Root.Rules[0].Field, db.FieldArtist)
	}
	if got.Root.Rules[0].Value != "Radiohead" {
		t.Errorf("Rules[0].Value = %q, want %q", got.Root.Rules[0].Value, "Radiohead")
	}
}

func TestSmartPlaylistQuery_OmitEmpty(t *testing.T) {
	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Field:    db.FieldTitle,
			Operator: db.OpEquals,
			Value:    "Creep",
		},
	}

	raw, err := db.MarshalSmartPlaylistQuery(q)
	if err != nil {
		t.Fatalf("MarshalSmartPlaylistQuery: %v", err)
	}

	// sort_by, sort_dir, max_tracks should be absent due to omitempty.
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, absent := range []string{"sort_by", "sort_dir", "max_tracks"} {
		if _, ok := m[absent]; ok {
			t.Errorf("key %q should be absent with zero value, but was present", absent)
		}
	}
}

// --- Validation ---

func TestValidateSmartRule_LeafOK(t *testing.T) {
	r := db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "test"}
	if err := db.ValidateSmartRule(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSmartRule_GroupOK(t *testing.T) {
	r := db.SmartRule{
		Conjunction: db.ConjOr,
		Rules: []db.SmartRule{
			{Field: db.FieldArtist, Operator: db.OpEquals, Value: "A"},
			{Field: db.FieldArtist, Operator: db.OpEquals, Value: "B"},
		},
	}
	if err := db.ValidateSmartRule(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSmartRule_EmptyNodeError(t *testing.T) {
	r := db.SmartRule{}
	if err := db.ValidateSmartRule(r); err == nil {
		t.Error("expected error for empty node, got nil")
	}
}

func TestValidateSmartRule_BothLeafAndGroupError(t *testing.T) {
	r := db.SmartRule{
		Field:       db.FieldArtist,
		Operator:    db.OpEquals,
		Value:       "A",
		Conjunction: db.ConjAnd,
		Rules:       []db.SmartRule{{Field: db.FieldTitle, Operator: db.OpEquals, Value: "T"}},
	}
	if err := db.ValidateSmartRule(r); err == nil {
		t.Error("expected error for node that is both leaf and group, got nil")
	}
}

func TestValidateSmartRule_UnknownField(t *testing.T) {
	r := db.SmartRule{Field: "nonexistent", Operator: db.OpEquals, Value: "x"}
	if err := db.ValidateSmartRule(r); err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestValidateSmartRule_UnknownOperator(t *testing.T) {
	r := db.SmartRule{Field: db.FieldArtist, Operator: "fuzzy", Value: "x"}
	if err := db.ValidateSmartRule(r); err == nil {
		t.Error("expected error for unknown operator, got nil")
	}
}

func TestValidateSmartRule_UnknownConjunction(t *testing.T) {
	r := db.SmartRule{
		Conjunction: "xor",
		Rules:       []db.SmartRule{{Field: db.FieldArtist, Operator: db.OpEquals, Value: "A"}},
	}
	if err := db.ValidateSmartRule(r); err == nil {
		t.Error("expected error for unknown conjunction, got nil")
	}
}

func TestValidateSmartRule_NestedError(t *testing.T) {
	r := db.SmartRule{
		Conjunction: db.ConjAnd,
		Rules: []db.SmartRule{
			{Field: "bad_field", Operator: db.OpEquals, Value: "v"},
		},
	}
	err := db.ValidateSmartRule(r)
	if err == nil {
		t.Fatal("expected error for nested unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "rules[0]") {
		t.Errorf("error should mention rules[0], got: %v", err)
	}
}

func TestParseSmartPlaylistQuery_InvalidJSON(t *testing.T) {
	_, err := db.ParseSmartPlaylistQuery("{not json}")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
