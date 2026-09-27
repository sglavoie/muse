package playlists

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
)

// --- 'c' create flow ---

func TestStartCreateActivatesInput(t *testing.T) {
	v := View{}
	v2 := v.StartCreate()

	if !v2.Creating {
		t.Error("StartCreate: expected Creating=true")
	}
	if v2.CreateInput.Value() != "" {
		t.Errorf("StartCreate: expected empty CreateInput, got %q", v2.CreateInput.Value())
	}
}

// TestInlineInputsShowTagsStylePrompt guards that the playlist create and
// rename rows carry the same "+ " / "r " prompt as the Tags rows, rather than
// an unlabelled input indistinguishable from a playlist name.
func TestInlineInputsShowTagsStylePrompt(t *testing.T) {
	if got := ansi.Strip((View{}).StartCreate().CreateInput.View()); !strings.HasPrefix(got, "+ ") {
		t.Errorf("create input should start with %q, got %q", "+ ", got)
	}
	if got := ansi.Strip((View{}).StartRename("Old").RenameInput.View()); !strings.HasPrefix(got, "r Old") {
		t.Errorf("rename input should start with %q, got %q", "r Old", got)
	}
}

func TestStartCreateCancelsRename(t *testing.T) {
	v := View{}.StartRename("OldName")
	v = v.StartCreate()

	if v.Renaming {
		t.Error("StartCreate: expected Renaming=false")
	}
	if v.RenameInput.Value() != "" {
		t.Errorf("StartCreate: expected empty RenameInput, got %q", v.RenameInput.Value())
	}
	if !v.Creating {
		t.Error("StartCreate: expected Creating=true")
	}
}

func TestCreateInputEmptyNameIsRejected(t *testing.T) {
	v := View{}.StartCreate()

	if strings.TrimSpace(v.CreateInput.Value()) != "" {
		t.Error("fresh CreateInput must be empty")
	}

	v.CreateInput.SetValue("  ")
	if strings.TrimSpace(v.CreateInput.Value()) != "" {
		t.Error("whitespace-only CreateInput must be considered empty")
	}
}

func TestCreateInputTypingAccumulatesBuffer(t *testing.T) {
	v := View{}.StartCreate()
	v.CreateInput.SetValue("MyList")
	if v.CreateInput.Value() != "MyList" {
		t.Errorf("expected buffer 'MyList', got %q", v.CreateInput.Value())
	}
	if strings.TrimSpace(v.CreateInput.Value()) == "" {
		t.Error("non-empty input must not be empty")
	}
}

func TestCancelCreateDismissesInput(t *testing.T) {
	v := View{}.StartCreate()
	v.CreateInput.SetValue("Partial")
	v2 := v.CancelCreate()

	if v2.Creating {
		t.Error("CancelCreate: expected Creating=false")
	}
	if v2.CreateInput.Value() != "" {
		t.Errorf("CancelCreate: expected empty CreateInput, got %q", v2.CreateInput.Value())
	}
}

func TestCreateFlowPersistSimulated(t *testing.T) {
	v := View{}.StartCreate()
	v.CreateInput.SetValue("NewPlaylist")

	name := v.CreateInput.Value()
	if strings.TrimSpace(v.CreateInput.Value()) == "" {
		t.Fatal("input must not be empty before persist")
	}

	v.Playlists = append(v.Playlists, db.PlaylistRow{ID: 99, Name: name})
	v = v.CancelCreate()

	if v.Creating {
		t.Error("expected Creating=false after persist")
	}
	if len(v.Playlists) != 1 || v.Playlists[0].Name != "NewPlaylist" {
		t.Errorf("expected one playlist 'NewPlaylist', got %v", v.Playlists)
	}
}
