package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

func TestUpdateCtrlCQuits(t *testing.T) {
	m := New(context.Background())
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c: expected a quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c: expected tea.QuitMsg, got %T", msg)
	}
}

// TestCtrlCQuitsWhileFilterBarActive verifies that ctrl+c dispatches tea.Quit
// even when the library filter bar is active and intercepting all keypresses.
func TestCtrlCQuitsWhileFilterBarActive(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.library.filter.Active = true
	m.library.filter.Text = "test"

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c while filter bar active: expected non-nil Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while filter bar active: expected QuitMsg, got %T", msg)
	}
}

// TestCtrlCQuitsWhilePlaylistCreateActive verifies that ctrl+c dispatches
// tea.Quit even when the inline playlist create input is active.
func TestCtrlCQuitsWhilePlaylistCreateActive(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists

	// Activate create mode.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if !m.playlists.isCreating() {
		t.Fatal("precondition: expected isCreating=true")
	}

	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c while create input active: expected non-nil Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while create input active: expected QuitMsg, got %T", msg)
	}
}

// TestCtrlCQuitsWhilePlaylistRenameActive verifies that ctrl+c dispatches
// tea.Quit even when the inline playlist rename input is active.
func TestCtrlCQuitsWhilePlaylistRenameActive(t *testing.T) {
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{{ID: 1, Name: "Test"}}))
	m.activeView = viewPlaylists

	// Activate rename mode via PlaylistRenameMsg.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if !m.playlists.isRenaming() {
		t.Fatal("precondition: expected isRenaming=true")
	}

	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c while rename input active: expected non-nil Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while rename input active: expected QuitMsg, got %T", msg)
	}
}