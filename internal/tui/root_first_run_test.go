package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
)

type noTracksExtractor struct{}

func (noTracksExtractor) Extract(context.Context, []library.HashedFile) ([]library.Track, error) {
	return nil, nil
}

func unconfiguredModel(t *testing.T, w, h int, opts ...Option) Model {
	t.Helper()
	cfg := config.Defaults()
	cfg.MusicDirectories, cfg.MusicDirectory = nil, ""
	m := New(t.Context(), append([]Option{WithConfig(cfg)}, opts...)...)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func TestFirstRunWelcomeOffersSuggestedFolder(t *testing.T) {
	for _, size := range [][2]int{{60, 15}, {80, 24}, {120, 36}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := unconfiguredModel(t, size[0], size[1], WithMusicDirSuggestion(fixtureMusicDir))
			screen := ansi.Strip(viewContent(m))
			for _, want := range []string{"┌┬┐ ┬ ┬ ┌─┐ ┌─┐", "A terminal player", "enter   use ~/Music", "ctrl+,  choose folders", "?       help"} {
				if !strings.Contains(screen, want) {
					t.Fatalf("missing %q:\n%s", want, screen)
				}
			}
			if strings.Contains(screen, "No music found") || strings.Contains(screen, "Welcome to Muse") {
				t.Fatalf("first-run screen shows the old card or the welcome overlay:\n%s", screen)
			}
			if !strings.Contains(ansi.Strip(m.renderFooterHint()), "enter use ~/Music") {
				t.Fatal("footer does not offer the suggested folder")
			}
		})
	}
}

func TestFirstRunWelcomeWithoutSuggestion(t *testing.T) {
	m := unconfiguredModel(t, 80, 24)
	screen := ansi.Strip(viewContent(m))
	if strings.Contains(screen, "enter") || !strings.Contains(screen, "ctrl+,  add music folders") {
		t.Fatalf("no suggested folder: only folder setup is offered:\n%s", screen)
	}
	if next, _ := updateModel(m, tea.KeyPressMsg{Code: tea.KeyEnter}); len(next.cfg.MusicDirectories) != 0 {
		t.Fatal("Enter added a folder nobody suggested")
	}
}

// Short screens give up the drawn wordmark for a one-line name before they
// cut any action.
func TestFirstRunWelcomeDegradesToPlainName(t *testing.T) {
	m := unconfiguredModel(t, 60, 15, WithMusicDirSuggestion(fixtureMusicDir))
	m.library.Height = 6
	body := ansi.Strip(m.renderPrimaryBody())
	if strings.Contains(body, "┌┬┐") || !strings.Contains(body, "muse") || !strings.Contains(body, "?       help") {
		t.Fatalf("short screen:\n%s", body)
	}
	if got := strings.Count(body, "\n") + 1; got != 6 {
		t.Fatalf("body is %d rows, want the library height 6", got)
	}
}

func TestFirstRunSuggestionNotOfferedOnceConfigured(t *testing.T) {
	m := New(t.Context(), WithMusicDirSuggestion(fixtureMusicDir))
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if _, ok := m.offeredMusicDir(); ok {
		t.Fatal("suggestion offered although music folders are configured")
	}
	screen := ansi.Strip(viewContent(m))
	if !strings.Contains(screen, "No music found in your folders") || !strings.Contains(screen, "check folders and exclusions") {
		t.Fatalf("configured but empty keeps its guidance:\n%s", screen)
	}
}

func TestFirstRunEnterAdoptsFolderAndScans(t *testing.T) {
	dir := t.TempDir()
	m := unconfiguredModel(t, 80, 24,
		WithMusicDirSuggestion(config.MusicDirSuggestion{Path: dir, Label: "~/Music"}),
		WithExtractor(noTracksExtractor{}))
	m, cmd := updateModel(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.cfg.MusicDirectories) != 1 || m.cfg.MusicDirectories[0] != dir || m.libraryRoots[0] != dir {
		t.Fatalf("folder not adopted: %v / %v", m.cfg.MusicDirectories, m.libraryRoots)
	}
	if !m.scanning || !m.firstRun.scanning || cmd == nil {
		t.Fatal("adopting the folder did not start the first scan")
	}
	screen := ansi.Strip(viewContent(m))
	if !strings.Contains(screen, "Scanning your music…") || !strings.Contains(screen, "Looking in …") {
		t.Fatalf("progress screen missing:\n%s", screen)
	}
	m, _ = updateModel(m, library.ScanDoneMsg{})
	if m.firstRun.scanning || m.scanning {
		t.Fatal("first scan state outlived the scan")
	}
}

// Without a config path (fixtures) the save is a no-op that still reports
// success, and without an extractor nothing is scanned.
func TestFirstRunAdoptInFixtureTouchesNothing(t *testing.T) {
	m, err := BuildFixture(FixtureEmpty)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := updateModel(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.scanning || cmd == nil {
		t.Fatal("fixture adopt should save in memory only")
	}
	if msg, ok := cmd().(settingsSavedMsg); !ok || msg.err != nil {
		t.Fatalf("save result %#v", msg)
	}
	if m.cfg.MusicDirectories[0] != fixtureMusicDir.Path {
		t.Fatal(m.cfg.MusicDirectories)
	}
}

func firstScanModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(t.Context(), WithLibraryRoots([]string{"/music"}), WithArtwork(artwork.SyntheticLoader{}, config.ArtworkBlocks))
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: w, Height: h})
	m.scanning = true
	m.firstRun = m.firstRun.begin()
	return m
}

func scanBatch(albums, perAlbum int) library.ScanBatchMsg {
	var tracks []library.Track
	for a := range albums {
		for n := range perAlbum {
			tracks = append(tracks, library.Track{
				Hash:   fmt.Sprintf("h%d-%d", a, n),
				Path:   fmt.Sprintf("/music/Artist %d/Album %d/%02d.flac", a, a, n),
				Title:  fmt.Sprintf("Song %d", n),
				Artist: fmt.Sprintf("Artist %d", a),
				Album:  fmt.Sprintf("Album %d", a),
			})
		}
	}
	return library.ScanBatchMsg{Tracks: tracks}
}

func TestFirstScanShowsLiveProgressAndCovers(t *testing.T) {
	m := firstScanModel(t, 80, 24)
	m, _ = updateModel(m, library.ScanTotalMsg{Total: 40})
	m, _ = updateModel(m, scanBatch(6, 3))
	screen := ansi.Strip(viewContent(m))
	for _, want := range []string{"Scanning your music…", "18 tracks found · 45%", "music/Artist 5/Album 5", "esc    cancel scan"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q:\n%s", want, screen)
		}
	}
	if len(m.firstRun.recent) != firstRunCovers || m.firstRun.recent[0].Album != "Album 5" {
		t.Fatalf("recent albums: %+v", m.firstRun.recent)
	}
	if !strings.ContainsAny(m.firstRunCoverRow(), "▀▄") {
		t.Fatal("covers did not pop in")
	}
	if strings.Contains(screen, "Song 0") {
		t.Fatalf("track list shown under the progress screen:\n%s", screen)
	}
	// Enter leaves the progress for the tracks found so far.
	m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if screen := ansi.Strip(viewContent(m)); strings.Contains(screen, "Scanning your music…") || !strings.Contains(screen, "Song") {
		t.Fatalf("browse now did not show the library:\n%s", screen)
	}
}

func TestFirstScanAtMinimumSizeDropsCovers(t *testing.T) {
	m := firstScanModel(t, 60, 15)
	m, _ = updateModel(m, scanBatch(2, 2))
	body := ansi.Strip(m.renderPrimaryBody())
	if strings.ContainsAny(body, "▀▄") || !strings.Contains(body, "4 tracks found") || !strings.Contains(body, "cancel scan") {
		t.Fatalf("minimum size:\n%s", body)
	}
}

func TestFirstScanEndsWithFoundNotice(t *testing.T) {
	m := firstScanModel(t, 120, 30)
	m, _ = updateModel(m, scanBatch(3, 4))
	m, _ = updateModel(m, library.ScanDoneMsg{})
	if m.firstRun.scanning {
		t.Fatal("first scan still flagged")
	}
	status := ansi.Strip(m.RenderStatusBar())
	if !strings.Contains(status, "Found 12 tracks across 3 albums") {
		t.Fatalf("status: %s", status)
	}
	if screen := ansi.Strip(viewContent(m)); !strings.Contains(screen, "Song") {
		t.Fatalf("did not land on the library:\n%s", screen)
	}
}

// A scan into a library that already has tracks is an ordinary rescan.
func TestRescanOfFilledLibraryIsNotFirstRun(t *testing.T) {
	m := newRichFixture()
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.libraryRoots = []string{t.TempDir()}
	m.extractor = noTracksExtractor{}
	next, _, _ := m.handleLibraryStateAction(keys.ActionRescan)
	if next.(Model).firstRun.scanning {
		t.Fatal("rescan of a filled library showed the first-run progress")
	}
}

func TestFirstRunSuppressesWelcomeOverlay(t *testing.T) {
	m := unconfiguredModel(t, 80, 24, WithConfigPath(testConfigPath, false),
		WithMusicDirHint(config.ConfigHint(testConfigPath), false))
	if m.welcomeVisible() {
		t.Fatal("first-run screen and welcome overlay both shown")
	}
	ch := make(chan library.Track)
	m = New(context.Background(), WithScanChannel(ch), WithConfigPath(testConfigPath, true))
	if m.welcomeVisible() || !m.firstRun.scanning {
		t.Fatal("first scan should show progress, not the overlay")
	}
}
