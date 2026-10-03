package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestPaletteGatedCommandsExplainAvailability(t *testing.T) {
	for _, m := range []Model{New(context.Background()), newRichFixture(), paletteTriageModel()} {
		for _, cmd := range m.BuildPaletteCommands() {
			if cmd.Enabled != nil && strings.TrimSpace(cmd.DisabledReason) == "" {
				t.Errorf("command %s has an availability gate but no explanation", cmd.ID)
			}
		}
	}
}

func TestFileManagerFailuresReachMessageHistory(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	next, _ := m.Update(desktop.FileManagerResultMsg{
		Action: "Reveal file", Path: "/music/song.flac", Err: errors.New("permission denied"),
	})
	m = next.(Model)
	history := m.statusBar.RecentMessages()
	if len(history) == 0 {
		t.Fatal("failure not recorded")
	}
	for _, want := range []string{"Reveal file failed", "/music/song.flac", "permission denied"} {
		if !strings.Contains(history[len(history)-1].Text, want) {
			t.Errorf("history missing %q: %v", want, history)
		}
	}
	m = m.showRecentMessages()
	if !strings.Contains(ansi.Strip(m.View().Content), "permission denied") {
		t.Fatal("failure cannot be recovered in recent messages")
	}
}

func TestFilterRecoveryHintsAndEmptyTagFilter(t *testing.T) {
	for _, view := range []viewID{viewLibrary, viewQueue} {
		for _, tag := range []bool{false, true} {
			for _, query := range []string{"", "zzzz-no-track"} {
				m := newRichFixture()
				m.ActiveView = view
				m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
				if tag {
					id := int64(999)
					m.activeTagFilter = &id
					m = m.applyTagFilterToViews()
				}
				if view == viewLibrary {
					m.library.Filter = filter.State{Text: query}
					m.library = m.library.RebuildFilterCache()
				} else {
					m.queue.Filter = filter.State{Text: query}
				}
				screen := ansi.Strip(m.View().Content)
				if tag || query != "" {
					if !strings.Contains(screen, "No matches") {
						t.Fatalf("view=%v tag=%v query=%q: missing no-matches hint: %s", view, tag, query, screen)
					}
					if strings.Contains(screen, "is empty") {
						t.Fatalf("filtered data described as empty: %s", screen)
					}
				}
				if tag && !strings.Contains(screen, "ctrl+t clears tag filter") {
					t.Fatalf("missing tag recovery: %s", screen)
				}
				if query != "" && !strings.Contains(screen, "Esc clears text filter") {
					t.Fatalf("missing text recovery: %s", screen)
				}
				for _, line := range strings.Split(screen, "\n") {
					if ansi.StringWidth(line) > 60 {
						t.Fatalf("hint overflow: %q", line)
					}
				}
				if query != "" {
					m = pressEsc(m)
				}
				if tag {
					if len(m.library.Panel().VisibleTracks()) != 0 || len(m.queue.FilteredEntries()) != 0 {
						t.Fatal("empty active tag filter leaked tracks after clearing text")
					}
					m = pressKey(m, "ctrl+t")
				}
				if len(m.library.Panel().VisibleTracks()) == 0 || len(m.queue.FilteredEntries()) == 0 {
					t.Fatal("clearing filters failed to restore tracks")
				}
			}
		}
	}
}

func TestTagFilterAppliesToLibrarySourcesAndSorts(t *testing.T) {
	for _, mode := range []libtui.LibraryMode{libtui.LibraryModeAll, libtui.LibraryModeRecent, libtui.LibraryModeHistory} {
		for _, field := range []sorting.SortField{sorting.SortDefault, sorting.SortTitle} {
			m := newRichFixture()
			tracks := m.library.Index.Tracks()
			m.library = m.library.SwitchMode(mode)
			if mode == libtui.LibraryModeHistory {
				m.library.SourceTracks = tracks[:2]
			}
			m.library.SortState.Field = field
			for _, hashes := range []map[string]struct{}{{}, {tracks[0].Hash: {}}} {
				m.library.TagFilterHashes = hashes
				m.library = m.library.RefreshView()
				got := m.library.Panel().VisibleTracks()
				if len(got) != len(hashes) {
					t.Fatalf("mode=%v sort=%v tags=%v: got %d tracks", mode, field, hashes, len(got))
				}
			}
		}
	}
}

func TestRevealFilePaletteNameMatchesPlatform(t *testing.T) {
	for platform, want := range map[string]string{"darwin": "Reveal in Finder", "linux": "Open containing folder"} {
		if got := revealFilePaletteName(platform); got != want {
			t.Errorf("%s: got %q, want %q", platform, got, want)
		}
	}
}
