package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
)

const testConfigPath = "/home/user/.config/muse/config.toml"

func sizedWelcomeModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	m := New(context.Background(), opts...)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

// The overlay must fire for a user whose config.toml already exists but whose
// music directory has gone away — otherwise the instructions never reach the
// person who needs them.
func TestWelcomeOverlay_FiresOnMusicDirHintWithoutConfigCreated(t *testing.T) {
	m := New(context.Background(),
		WithConfigPath(testConfigPath, false),
		WithMusicDirHint(config.ConfigHint(testConfigPath)),
	)
	if !m.welcomeVisible() {
		t.Fatal("welcome overlay should be visible when no music directories are valid")
	}
	if m.activeOverlayName() != "welcome" {
		t.Fatalf("activeOverlayName = %q, want welcome", m.activeOverlayName())
	}
}

func TestWelcomeOverlay_HiddenWhenDirsValidAndConfigNotCreated(t *testing.T) {
	m := New(context.Background(), WithConfigPath(testConfigPath, false))
	if m.welcomeVisible() {
		t.Fatal("welcome overlay should stay hidden with a valid config and music dirs")
	}
}

// The overlay reuses config.ConfigHint verbatim, so it names the config file and
// the exact TOML key the user has to set.
func TestWelcomeOverlay_NamesConfigPathAndMusicDirectoriesKey(t *testing.T) {
	m := sizedWelcomeModel(t,
		WithConfigPath(testConfigPath, true),
		WithMusicDirHint(config.ConfigHint(testConfigPath)),
	)
	view := ansi.Strip(viewContent(m))

	for _, want := range []string{
		"No music directories found.",
		testConfigPath,
		"music_directories = [",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("welcome overlay missing %q:\n%s", want, view)
		}
	}
	// The keybinding rows survive alongside the new copy.
	if !strings.Contains(view, "Show all keybindings") {
		t.Errorf("welcome overlay lost its keybinding rows:\n%s", view)
	}
}

func TestWelcomeOverlay_NoMusicDirCopyWhenHintEmpty(t *testing.T) {
	m := sizedWelcomeModel(t, WithConfigPath(testConfigPath, true))
	view := ansi.Strip(viewContent(m))

	if strings.Contains(view, "music_directories") {
		t.Errorf("welcome overlay should not mention music_directories when dirs are valid:\n%s", view)
	}
	if !strings.Contains(view, "Welcome to Muse") {
		t.Errorf("welcome overlay missing its title:\n%s", view)
	}
}

// Any key dismisses the overlay even though configCreated is cleared first by
// handleKeyPressMsg; the hint alone would otherwise keep it on screen forever.
func TestWelcomeOverlay_DismissedByAnyKey(t *testing.T) {
	m := sizedWelcomeModel(t,
		WithConfigPath(testConfigPath, false),
		WithMusicDirHint(config.ConfigHint(testConfigPath)),
	)
	m, _ = updateModel(m, tea.KeyPressMsg{Code: 'x', Text: "x"})

	if m.welcomeVisible() {
		t.Fatal("welcome overlay should be dismissed after a key press")
	}
	if got := ansi.Strip(viewContent(m)); strings.Contains(got, "No music directories found.") {
		t.Errorf("dismissed overlay still rendered:\n%s", got)
	}
}

func TestHintRows_DropsBlankLines(t *testing.T) {
	rows := hintRows(config.ConfigHint(testConfigPath))
	if len(rows) != 2 {
		t.Fatalf("hintRows = %d rows, want 2: %+v", len(rows), rows)
	}
	if !strings.Contains(rows[0].Text, testConfigPath) {
		t.Errorf("row 0 = %q, want the config path", rows[0].Text)
	}
	if !strings.Contains(rows[1].Text, "music_directories = [") {
		t.Errorf("row 1 = %q, want the music_directories key", rows[1].Text)
	}
}
