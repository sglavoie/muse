package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/smartrules"
)

func TestTagLifecycleUpdatesCachedSmartPlaylistAndOpenDraft(t *testing.T) {
	m := newRichFixture()
	source := db.TagRow{ID: 1, Name: "Before"}
	destination := db.TagRow{ID: 2, Name: "Destination"}
	m = m.rebuildTagCache([]db.TagRow{source, destination})
	query := db.SmartPlaylistQuery{Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpEquals, Value: source.Name, TagID: source.ID}}
	raw, err := db.MarshalSmartPlaylistQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	m.playlists.Playlists = []db.PlaylistRow{{ID: 42, Name: "Saved", IsSmart: true, Query: raw}}
	m.playlists.SmartQuery = &query
	editor := smartrules.NewSmartRuleEditorEdit(42, "Saved", query).WithTags(m.tags)
	m.overlay.smartRuleEditor = &editor
	m = m.handleTagRenamed(tagRenamedMsg{id: 1, name: "After"})
	if got := m.overlay.smartRuleEditor.BuildQuery().Root.Rules[0]; got.Value != "After" || got.TagID != 1 {
		t.Fatal(got)
	}
	m.tagReader = nil
	m, _ = m.handleTagMerged(tagMergedMsg{src: source, dest: destination})
	stored, err := db.ParseSmartPlaylistQuery(m.playlists.Playlists[0].Query)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []db.SmartRule{stored.Root, m.playlists.SmartQuery.Root, m.overlay.smartRuleEditor.BuildQuery().Root.Rules[0]} {
		if rule.TagID != 2 || rule.Value != "Destination" {
			t.Fatalf("stale snapshot: %+v", rule)
		}
	}
	// Catalog-only updates do not create edits that require discard confirmation.
	m = pressEsc(m)
	if m.overlay.smartRuleEditor != nil {
		t.Fatal("rename/merge dirtied an otherwise untouched draft")
	}
}
