package smartrules

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestTagRuleIdentitySurvivesRenameEditAndMerge(t *testing.T) {
	rule := db.SmartRule{Field: db.FieldTag, Operator: db.OpNotEquals, Value: "Old", TagID: 4}
	e := NewSmartRuleEditorEdit(1, "Test", db.SmartPlaylistQuery{Root: rule}).WithTags([]db.TagRow{{ID: 4, Name: "Renamed"}})
	if e.hasChanges() || e.rules[0].Value != "Renamed" {
		t.Fatal("catalog refresh dirtied form or failed to update label")
	}
	e = e.startEditRule(0).confirmEditRule()
	if e.rules[0].TagID != 4 {
		t.Fatal("editing dropped identity")
	}
	e = e.RemapTag(4, db.TagRow{ID: 5, Name: "Merged"}).WithTags([]db.TagRow{{ID: 5, Name: "Merged"}})
	if e.hasChanges() || e.rules[0].TagID != 5 {
		t.Fatal("merge changed draft ownership", e.BuildQuery())
	}
	e = e.WithTags(nil)
	if !strings.Contains(e.ruleLabel(e.rules[0]), "missing tag") {
		t.Fatal("deleted tag was not marked for repair")
	}
	e = e.WithTags([]db.TagRow{{ID: 6, Name: "Merged"}})
	if e.rules[0].TagID != 5 {
		t.Fatal("new namesake silently replaced missing identity")
	}
}
