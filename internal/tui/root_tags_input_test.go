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
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel
	m.tagsView = m.tagsView.startCreate()
	m.tagsView.createInput.SetValue("New Tag")

	m, cmd, consumed := m.handleTagCreateInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed {
		t.Fatal("expected enter to be consumed")
	}
	if cmd != nil {
		t.Fatal("expected no create command without tag writer")
	}
	if m.tagsView.isCreating() {
		t.Fatal("expected create input to be cancelled")
	}
}

func TestTagRenameInputWithoutWriterDoesNotPanic(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel
	m.tagsView.tags = []db.TagRow{{ID: 1, Name: "Old"}}
	m.tagsView.tagCounts = map[int64]int{1: 1}
	m.tagsView = m.tagsView.startRename("Old")
	m.tagsView.renameInput.SetValue("New")

	m, cmd, consumed := m.handleTagRenameInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed {
		t.Fatal("expected enter to be consumed")
	}
	if cmd != nil {
		t.Fatal("expected no rename command without tag writer")
	}
	if m.tagsView.isRenaming() {
		t.Fatal("expected rename input to be cancelled")
	}
}
