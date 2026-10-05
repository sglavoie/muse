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

func TestWelcomeOpensLibrarySettings(t *testing.T) {
	m := sizedWelcomeModel(t, WithConfigPath(testConfigPath, true))
	if !strings.Contains(ansi.Strip(viewContent(m)), "Set music directories") {
		t.Fatal("welcome does not advertise directory setup")
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	if m.welcomeVisible() || m.overlay.settings == nil {
		t.Fatal("settings shortcut only dismissed welcome")
	}
	// 'a' opens the directory input only on the Library tab.
	m = pressKey(m, "a")
	if m.overlay.settings.LibDirInput == nil {
		t.Fatal("settings did not open on Library")
	}
	m = pressEsc(pressEsc(m))
	if m.welcomeVisible() || m.overlay.settings != nil {
		t.Fatal("canceling settings should return to the library")
	}
}

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
		WithMusicDirHint(config.ConfigHint(testConfigPath), false),
	)
	if !m.welcomeVisible() {
		t.Fatal("welcome overlay should be visible when no music directories are valid")
	}
	if m.ActiveOverlayName() != "welcome" {
		t.Fatalf("activeOverlayName = %q, want welcome", m.ActiveOverlayName())
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
		WithMusicDirHint(config.ConfigHint(testConfigPath), false),
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

// With some directories still usable the overlay must not claim that none were
// found — the valid roots are being scanned while it is on screen — but it
// still has to carry the actionable fix for the broken ones.
func TestWelcomeOverlay_PartialValidityCopy(t *testing.T) {
	m := sizedWelcomeModel(t,
		WithConfigPath(testConfigPath, true),
		WithMusicDirHint(config.ConfigHint(testConfigPath), true),
	)
	view := ansi.Strip(viewContent(m))

	if strings.Contains(view, "No music directories found.") {
		t.Errorf("overlay falsely claims no directories were found:\n%s", view)
	}
	for _, want := range []string{
		"Some music directories could not be read.",
		"The remaining directories are being scanned.",
		testConfigPath,
		"music_directories = [",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("welcome overlay missing %q:\n%s", want, view)
		}
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

// Ordinary keys dismiss the overlay even when a missing-directory hint persists.
func TestWelcomeOverlay_DismissedByAnyKey(t *testing.T) {
	m := sizedWelcomeModel(t,
		WithConfigPath(testConfigPath, false),
		WithMusicDirHint(config.ConfigHint(testConfigPath), false),
	)
	m, _ = updateModel(m, tea.KeyPressMsg{Code: 'x', Text: "x"})

	if m.welcomeVisible() {
		t.Fatal("welcome overlay should be dismissed after a key press")
	}
	if got := ansi.Strip(viewContent(m)); strings.Contains(got, "No music directories found.") {
		t.Errorf("dismissed overlay still rendered:\n%s", got)
	}
}

func TestWelcomeOverlayQuitAndDismissal(t *testing.T) {
	for _, created := range []bool{false, true} {
		for _, quit := range []bool{false, true} {
			opts := []Option{WithConfigPath(testConfigPath, created)}
			if !created {
				opts = append(opts, WithMusicDirHint(config.ConfigHint(testConfigPath), false))
			}
			m := sizedWelcomeModel(t, opts...)
			key := tea.KeyPressMsg{Code: '2', Text: "2"}
			if quit {
				key = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
			}
			m, cmd := updateModel(m, key)
			if quit {
				if !m.quitting || cmd == nil {
					t.Fatal("welcome must quit on the first Ctrl+C")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("welcome did not return the quit command")
				}
			} else if m.welcomeVisible() || m.ActiveView != viewLibrary {
				t.Fatal("dismissal key leaked through to the underlying view")
			}
		}
	}
}
