package tui

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"strings"
	"testing"
)

func TestEmptyLibraryRecoveryAgreesAcrossScreen(t *testing.T) {
	for _, tc := range []struct {
		name       string
		setup      func(*Model)
		title, key string
	}{
		{"unconfigured", func(m *Model) { m.cfg.MusicDirectories = nil; m.libraryRoots = nil }, "Add a music folder", "ctrl+,"},
		{"configured empty", func(m *Model) { m.cfg.MusicDirectories = []string{"/music"} }, "No music found", "ctrl+,"},
		{"unavailable", func(m *Model) {
			m.cfg.MusicDirectories = []string{"/offline"}
			m.musicDirHint = "unavailable"
			m.welcomeDismissed = true
		}, "Music folders need attention", "ctrl+,"},
		{"scanning", func(m *Model) { m.scanning = true; m.longOp = statusbar.LongOpState{Active: true, Label: "Scan"} }, "Scanning music folders", "esc"},
		{"scan error", func(m *Model) {
			m.cfg.MusicDirectories = []string{"/music"}
			next, _ := m.Update(library.ScanErrorMsg{Err: errors.New("permission denied")})
			*m = next.(Model)
		}, "Music folders need attention", "ctrl+,"},
	} {
		for _, size := range [][2]int{{60, 15}, {100, 30}} {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size[0], size[1]), func(t *testing.T) {
				m := New(t.Context())
				m.cfg.MusicDirectories = nil
				m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				tc.setup(&m)
				body := ansi.Strip(m.renderActiveBody())
				footer := ansi.Strip(m.renderFooterHint())
				status := ansi.Strip(m.RenderStatusBar())
				for _, out := range []string{body, status} {
					if !strings.Contains(out, tc.title) || !strings.Contains(out, tc.key) {
						t.Fatalf("inconsistent guidance: %s", out)
					}
				}
				if !strings.Contains(footer, tc.key) || strings.Contains(status, "press 1 to browse Library") {
					t.Fatalf("footer/status: %s / %s", footer, status)
				}
				if tc.name == "scanning" && !strings.Contains(status, "esc cancel") {
					t.Fatal("scan progress hidden by empty queue", status)
				}
			})
		}
	}
}

func TestMixKeysSelectWithoutDismissing(t *testing.T) {
	m := newRichFixture()
	m = m.switchToView(viewMixes)
	next, load := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if load != nil {
		next, _ = m.Update(load())
		m = next.(Model)
	}
	if m.mixes.Level != 1 {
		t.Fatal("Enter did not open mix")
	}
	m = pressKey(m, "x")
	if m.activeViewSelectionCount() != 1 || len(m.dismissedHashes) != 0 {
		t.Fatal("x must select, not dismiss")
	}
	m = pressKey(m, "X")
	if m.activeViewSelectionCount() != 0 {
		t.Fatal("X must clear")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || next.(Model).mixes.Level != 1 {
		t.Fatal("Enter should enqueue a focused mix track")
	}
}

func TestMixPaletteLeadsWithListeningAndKeepsRemovalSearchable(t *testing.T) {
	m := setupMixesAtTrackLevel()
	cp, _ := palette.NewCommandPalette(m.BuildPaletteCommands(), nil)
	cp = cp.WithHeight(12)
	if cp.Filtered[0].ID != "queue.enqueue.focused" {
		t.Fatalf("first action: %s", cp.Filtered[0].ID)
	}
	for _, c := range cp.Filtered[:6] {
		if c.Intent == palette.IntentRemoval {
			t.Fatalf("removal crowded out listening: %s", c.ID)
		}
	}
	cp.Filter.Text = "dismiss track from mix"
	cp.Refilter()
	if len(cp.Filtered) == 0 || cp.Filtered[0].ID != "mix.dismiss" {
		t.Fatal("dismiss search lost")
	}
}

func TestAutoDJSourceLabelPreservesNamedPlaylists(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 160, Height: 30})
	m.ActiveView = viewQueue
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0
	m.queue.Entries[0].SourcePlaylist = "Radio"
	m.statusBar.PlaylistSource = "Radio"
	for _, owned := range []bool{true, false} {
		m.queue.Entries[0].DJOwned = owned
		want := "Radio"
		if owned {
			want = "Auto-DJ"
		}
		if out := ansi.Strip(m.RenderStatusBar()); !strings.Contains(out, "["+want+"]") {
			t.Fatal(out)
		}
		if out := ansi.Strip(m.renderPrimaryBody()); !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
}
