package playlists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- 'r' rename flow ---

func TestStartRenamePrePopulatesInput(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{{ID: 1, Name: "OldName"}},
	}
	v2 := v.StartRename("OldName")

	if !v2.Renaming {
		t.Error("StartRename: expected Renaming=true")
	}
	if v2.RenameInput.Value() != "OldName" {
		t.Errorf("StartRename: expected RenameInput='OldName', got %q", v2.RenameInput.Value())
	}
}

func TestRenameInputEmptyNameIsRejected(t *testing.T) {
	v := View{}.StartRename("SomeName")
	v.RenameInput.SetValue("")

	if strings.TrimSpace(v.RenameInput.Value()) != "" {
		t.Errorf("fully-erased RenameInput must be empty; got %q", v.RenameInput.Value())
	}
}

func TestRenameDuplicateNameRejected(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
		},
		Cursor:   0,
		Renaming: true,
	}
	v.RenameInput = NewInlineInput("Beta")

	isDuplicate := false
	for _, p := range v.Playlists {
		if p.Name == v.RenameInput.Value() {
			isDuplicate = true
			break
		}
	}
	if !isDuplicate {
		t.Error("entering an existing name must be detected as duplicate")
	}
	if !v.Renaming {
		t.Error("Renaming must stay active when a duplicate is detected")
	}
}

func TestCancelRenameDismissesInput(t *testing.T) {
	v := View{}.StartRename("CurrentName")
	v2 := v.CancelRename()

	if v2.Renaming {
		t.Error("CancelRename: expected Renaming=false")
	}
	if v2.RenameInput.Value() != "" {
		t.Errorf("CancelRename: expected empty RenameInput, got %q", v2.RenameInput.Value())
	}
}

func TestRenameFlowPersistSimulated(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{{ID: 1, Name: "OldName"}},
		Cursor:    0,
	}
	v = v.StartRename("OldName")
	v.RenameInput.SetValue("NewName")

	newName := v.RenameInput.Value()
	if strings.TrimSpace(v.RenameInput.Value()) == "" {
		t.Fatal("RenameInput must not be empty before persist")
	}

	isDuplicate := false
	for _, p := range v.Playlists {
		if p.Name == newName {
			isDuplicate = true
			break
		}
	}
	if isDuplicate {
		t.Fatal("test setup error: new name must not be a duplicate")
	}

	v.Playlists[v.Cursor].Name = newName
	v = v.CancelRename()

	if v.Renaming {
		t.Error("expected Renaming=false after persist")
	}
	if v.Playlists[0].Name != "NewName" {
		t.Errorf("expected Playlists[0].Name='NewName', got %q", v.Playlists[0].Name)
	}
}
