package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestTagCreateInputWithoutWriterDoesNotPanic(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView = m.tagsView.StartCreate()
	m.tagsView.CreateInput.SetValue("New Tag")

	m, cmd, consumed := m.handleTagCreateInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed {
		t.Fatal("expected enter to be consumed")
	}
	if cmd != nil {
		t.Fatal("expected no create command without tag writer")
	}
	if m.tagsView.IsCreating() {
		t.Fatal("expected create input to be cancelled")
	}
}

func TestTagRenameInputWithoutWriterDoesNotPanic(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = []db.TagRow{{ID: 1, Name: "Old"}}
	m.tagsView.TagCounts = map[int64]int{1: 1}
	m.tagsView = m.tagsView.StartRename("Old")
	m.tagsView.RenameInput.SetValue("New")

	m, cmd, consumed := m.handleTagRenameInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed {
		t.Fatal("expected enter to be consumed")
	}
	if cmd != nil {
		t.Fatal("expected no rename command without tag writer")
	}
	if m.tagsView.IsRenaming() {
		t.Fatal("expected rename input to be cancelled")
	}
}
