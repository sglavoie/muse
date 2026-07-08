package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// --- cursor navigation ---

func TestGoToCursorDown(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
	})
	m2, cmd := m.HandleKey("j")
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %T", cmd)
	}
	if m2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", m2.cursor)
	}
}

func TestGoToCursorUp(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
	})
	m.cursor = 1
	m2, cmd := m.HandleKey("k")
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %T", cmd)
	}
	if m2.cursor != 0 {
		t.Errorf("cursor: want 0, got %d", m2.cursor)
	}
}

func TestGoToCursorClampsBottom(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
	})
	m2, _ := m.HandleKey("j")
	if m2.cursor != 0 {
		t.Errorf("cursor: want 0 (clamped), got %d", m2.cursor)
	}
}

func TestGoToCursorClampsTop(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
	})
	m2, _ := m.HandleKey("k")
	if m2.cursor != 0 {
		t.Errorf("cursor: want 0 (clamped), got %d", m2.cursor)
	}
}

func TestGoToDownArrow(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
	})
	m2, _ := m.HandleKey("down")
	if m2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", m2.cursor)
	}
}

func TestGoToUpArrow(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
	})
	m.cursor = 1
	m2, _ := m.HandleKey("up")
	if m2.cursor != 0 {
		t.Errorf("cursor: want 0, got %d", m2.cursor)
	}
}

// --- confirm via enter ---

func TestGoToEnterConfirms(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
	})
	m.cursor = 1
	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(GoToPickedMsg)
	if !ok {
		t.Fatalf("expected GoToPickedMsg, got %T", msg)
	}
	if picked.Kind != goToArtist || picked.Value != "Bob" {
		t.Errorf("expected goToArtist/Bob, got %v/%q", picked.Kind, picked.Value)
	}
}

// --- dismiss via esc ---

func TestGoToEscDismisses(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
	})
	_, cmd := m.HandleKey("esc")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(GoToDismissedMsg); !ok {
		t.Fatalf("expected GoToDismissedMsg, got %T", msg)
	}
}

// --- shortcut keys ---

func TestGoToShortcutArtist(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
		{Kind: goToGenre, Value: "Rock"},
	})
	_, cmd := m.HandleKey("a")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	picked := cmd().(GoToPickedMsg)
	if picked.Kind != goToArtist || picked.Value != "Bob" {
		t.Errorf("expected goToArtist/Bob, got %v/%q", picked.Kind, picked.Value)
	}
}

func TestGoToShortcutAlbum(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Bob"},
	})
	_, cmd := m.HandleKey("b")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	picked := cmd().(GoToPickedMsg)
	if picked.Kind != goToAlbum || picked.Value != "Alpha" {
		t.Errorf("expected goToAlbum/Alpha, got %v/%q", picked.Kind, picked.Value)
	}
}

func TestGoToShortcutGenre(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToGenre, Value: "Rock"},
	})
	_, cmd := m.HandleKey("g")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	picked := cmd().(GoToPickedMsg)
	if picked.Kind != goToGenre || picked.Value != "Rock" {
		t.Errorf("expected goToGenre/Rock, got %v/%q", picked.Kind, picked.Value)
	}
}

func TestGoToShortcutPlaylist(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToPlaylist, Value: "My Playlist"},
	})
	_, cmd := m.HandleKey("p")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	picked := cmd().(GoToPickedMsg)
	if picked.Kind != goToPlaylist || picked.Value != "My Playlist" {
		t.Errorf("expected goToPlaylist/My Playlist, got %v/%q", picked.Kind, picked.Value)
	}
}

func TestGoToShortcutNoMatch(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
	})
	_, cmd := m.HandleKey("g") // no genre option
	if cmd != nil {
		t.Fatalf("expected nil cmd when no genre option, got %T", cmd)
	}
}

func TestGoToShortcutPrefersFocused(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToGenre, Value: "Rock"},
		{Kind: goToGenre, Value: "Jazz"},
	})
	m.cursor = 1 // focused on Jazz
	_, cmd := m.HandleKey("g")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	picked := cmd().(GoToPickedMsg)
	if picked.Value != "Jazz" {
		t.Errorf("expected Jazz (focused), got %q", picked.Value)
	}
}

func TestGoToShortcutFallsBackToFirst(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToGenre, Value: "Rock"},
		{Kind: goToGenre, Value: "Jazz"},
	})
	m.cursor = 0 // focused on Album, not a genre
	_, cmd := m.HandleKey("g")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	picked := cmd().(GoToPickedMsg)
	if picked.Value != "Rock" {
		t.Errorf("expected Rock (first genre), got %q", picked.Value)
	}
}

// --- display labels ---

func TestGoToDisplayLabel(t *testing.T) {
	tests := []struct {
		opt  GoToOption
		want string
	}{
		{GoToOption{Kind: goToArtist, Value: "Bob"}, "a  Artist: Bob"},
		{GoToOption{Kind: goToAlbum, Value: "Alpha"}, "b  Album: Alpha"},
		{GoToOption{Kind: goToGenre, Value: "Rock"}, "g  Genre: Rock"},
		{GoToOption{Kind: goToPlaylist, Value: "My PL"}, "p  Source: My PL"},
	}
	for _, tt := range tests {
		got := tt.opt.displayLabel()
		if got != tt.want {
			t.Errorf("displayLabel(%v): want %q, got %q", tt.opt, tt.want, got)
		}
	}
}

// --- render ---

func TestGoToRenderContainsTitle(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToArtist, Value: "Bob"},
	})
	s := styles.NewStyles(true)
	out := m.Render(80, 24, s)
	if !strings.Contains(ansi.Strip(out), "Go To") {
		t.Error("rendered output should contain 'Go To' title")
	}
}

func TestGoToRenderContainsOptions(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToArtist, Value: "Bob"},
		{Kind: goToAlbum, Value: "Alpha"},
	})
	s := styles.NewStyles(true)
	out := m.Render(80, 24, s)
	if !strings.Contains(out, "Artist: Bob") {
		t.Error("rendered output should contain 'Artist: Bob'")
	}
	if !strings.Contains(out, "Album: Alpha") {
		t.Error("rendered output should contain 'Album: Alpha'")
	}
}

func TestGoToRenderContainsHint(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToArtist, Value: "Bob"},
	})
	s := styles.NewStyles(true)
	out := m.Render(80, 24, s)
	if !strings.Contains(out, "a/b/g/p shortcut") {
		t.Error("rendered output should contain shortcut hint")
	}
}

func TestGoToRenderHintFitsMinimumSupportedSize(t *testing.T) {
	m := NewGoToModal([]GoToOption{
		{Kind: goToAlbum, Value: "Alpha"},
		{Kind: goToArtist, Value: "Alice"},
		{Kind: goToGenre, Value: "Ambient"},
	})
	s := styles.NewStyles(true)

	raw := m.Render(minTerminalWidth, minTerminalHeight, s)
	if issues := DetectLayoutIssues(raw, PlainScreen(raw), minTerminalWidth, minTerminalHeight); len(issues) != 0 {
		t.Fatalf("go-to overlay should fit at minimum supported size, got %+v:\n%s", issues, PlainScreen(raw))
	}
}
