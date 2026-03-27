package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestBuildQuery_DefaultSortOmitsSortDir(t *testing.T) {
	e := newSmartRuleEditor()
	// sortByIdx 0 is "(default)" → sortBy should be "" and sortDir should be "".
	q := e.buildQuery()
	if q.SortBy != "" {
		t.Errorf("SortBy: got %q, want empty", q.SortBy)
	}
	if q.SortDir != "" {
		t.Errorf("SortDir: got %q, want empty when SortBy is default", q.SortDir)
	}
}

func TestBuildQuery_ExplicitSortIncludesSortDir(t *testing.T) {
	e := newSmartRuleEditor()
	e.sortByIdx = 1 // "artist"
	e.sortDir = 1   // desc

	q := e.buildQuery()
	if q.SortBy != "artist" {
		t.Errorf("SortBy: got %q, want %q", q.SortBy, "artist")
	}
	if q.SortDir != "desc" {
		t.Errorf("SortDir: got %q, want %q", q.SortDir, "desc")
	}
}

func TestBuildQuery_AscSortDir(t *testing.T) {
	e := newSmartRuleEditor()
	e.sortByIdx = 2 // "title"
	e.sortDir = 0   // asc

	q := e.buildQuery()
	if q.SortBy != "title" {
		t.Errorf("SortBy: got %q, want %q", q.SortBy, "title")
	}
	if q.SortDir != "asc" {
		t.Errorf("SortDir: got %q, want %q", q.SortDir, "asc")
	}
}

func TestBuildQuery_EmptyRulesCreatesValidGroup(t *testing.T) {
	e := newSmartRuleEditor()
	q := e.buildQuery()

	if q.Root.Conjunction != db.ConjAnd {
		t.Errorf("Conjunction: got %q, want %q", q.Root.Conjunction, db.ConjAnd)
	}
	if len(q.Root.Rules) != 0 {
		t.Errorf("Rules: got %d rules, want 0", len(q.Root.Rules))
	}
}

func TestBuildQuery_MaxTracks(t *testing.T) {
	e := newSmartRuleEditor()
	for _, r := range "25" {
		e.maxTracksInput.insertRune(r)
	}
	q := e.buildQuery()
	if q.MaxTracks != 25 {
		t.Errorf("MaxTracks: got %d, want 25", q.MaxTracks)
	}
}

func TestBuildQuery_MaxTracksInvalid(t *testing.T) {
	e := newSmartRuleEditor()
	for _, r := range "abc" {
		e.maxTracksInput.insertRune(r)
	}
	q := e.buildQuery()
	if q.MaxTracks != 0 {
		t.Errorf("MaxTracks: got %d, want 0 for invalid input", q.MaxTracks)
	}
}
