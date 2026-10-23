package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Drain only finite navigation commands; periodic Init commands are not run.
func browsingTestDrain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	pending := []tea.Cmd{cmd}
	for n := 0; len(pending) > 0; n++ {
		if n > 100 {
			t.Fatal("navigation did not finish")
		}
		cmd, pending = pending[0], pending[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			pending = append(pending, batch...)
			continue
		}
		next, follow := m.Update(msg)
		m = next.(Model)
		pending = append(pending, follow)
	}
	return m
}

func browsingTestEnter(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return browsingTestDrain(t, next.(Model), cmd)
}

func browsingTestRoundTrip(t *testing.T, m Model) Model {
	t.Helper()
	before := m.captureBrowsingSession()
	m.dataDir = t.TempDir()
	if err := m.SaveBrowsingSession(); err != nil {
		t.Fatal(err)
	}
	if after := m.captureBrowsingSession(); !reflect.DeepEqual(before, after) {
		t.Fatal("saving mutated navigation")
	}
	fresh := newRichFixture()
	fresh.dataDir = m.dataDir
	fresh = fresh.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30}).loadBrowsingSession()
	if fresh.browsingRestore == nil {
		t.Fatal("saved session not loaded")
	}
	n, cmd := fresh.Update(browsingResumeMsg{})
	restored := browsingTestDrain(t, n.(Model), cmd)
	if restored.browsingRestore != nil {
		t.Fatal("restore did not finish")
	}
	after := restored.captureBrowsingSession()
	if !reflect.DeepEqual(before, after) {
		a, _ := json.MarshalIndent(before, "", "  ")
		b, _ := json.MarshalIndent(after, "", "  ")
		t.Fatalf("round trip changed context:\n%s\ninto:\n%s", a, b)
	}
	if len(restored.queue.Entries) != len(fresh.queue.Entries) || restored.playback.state != playback.StateIdle {
		t.Fatal("navigation changed playback")
	}
	return restored
}

func TestBrowsingSessionRoundTripEveryViewAndCollection(t *testing.T) {
	cases := []struct {
		name  string
		view  viewID
		mode  browse.BrowseMode
		depth int
	}{
		{"library", viewLibrary, 0, 0}, {"queue", viewQueue, 0, 0},
		{"playlists", viewPlaylists, 0, 0}, {"playlist tracks", viewPlaylists, 0, 1},
		{"artists", viewBrowse, browse.BrowseModeArtists, 0}, {"artist albums", viewBrowse, browse.BrowseModeArtists, 1}, {"artist tracks", viewBrowse, browse.BrowseModeArtists, 2},
		{"albums", viewBrowse, browse.BrowseModeAlbums, 0}, {"album tracks", viewBrowse, browse.BrowseModeAlbums, 1},
		{"genres", viewBrowse, browse.BrowseModeGenres, 0}, {"genre artists", viewBrowse, browse.BrowseModeGenres, 1}, {"genre tracks", viewBrowse, browse.BrowseModeGenres, 2},
		{"favorites", viewFavorites, 0, 0}, {"favorite artists", viewFavorites, 0, 1}, {"favorite tracks", viewFavorites, 0, 2},
		{"tags", viewTags, 0, 0}, {"tag tracks", viewTags, 0, 1},
		{"mixes", viewMixes, 0, 0}, {"mix tracks", viewMixes, 0, 1},
		{"insights", viewInsights, 0, 0}, {"insights tracks", viewInsights, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newRichFixture()
			m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
			m.browse.Mode = tc.mode
			m = m.switchToView(tc.view)
			cmd := m.viewLazyLoadCmd(tc.view)
			m = browsingTestDrain(t, m, cmd)
			for range tc.depth {
				m = browsingTestEnter(t, m)
			}
			if f := m.browsingFilter(); f != nil && len(m.browsingKeys()) > 0 {
				// Preserve a committed query as well as focus.
				if tc.view == viewLibrary || strings.Contains(tc.name, "tracks") {
					f.Text = "a:"
				}
				m = m.recomputePreferredSort()
			}
			if len(m.browsingKeys()) > 1 {
				p := m.browsingPosition()
				p.Key = m.browsingKeys()[1]
				p.Cursor = 1
				m.restoreBrowsingPosition(p)
			}
			browsingTestRoundTrip(t, m)
		})
	}
}

func TestBrowsingSessionRestoresNestedFiltersAfterRestart(t *testing.T) {
	m := newRichFixture().handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.switchToView(viewBrowse)
	m = pressEnter(typeText(pressSlash(m), "Alice"))
	m = browsingTestEnter(t, m)
	m = pressEnter(typeText(pressSlash(m), "Alpha"))
	m = browsingTestEnter(t, m)
	m = pressEnter(typeText(pressSlash(m), "Track A2"))
	restored := browsingTestRoundTrip(t, m)
	if !strings.Contains(restored.View().Content, "filtered") {
		t.Fatal("restored filter is not visible")
	}
	restored = pressEsc(restored) // clear child filter
	restored = pressEsc(restored) // child -> parent
	if restored.browse.Artists.Filter.Text != "Alpha" {
		t.Fatal("album query lost")
	}
	restored = pressEsc(restored) // clear
	restored = pressEsc(restored) // parent -> root
	if restored.browse.Artists.Filter.Text != "Alice" {
		t.Fatal("artist query lost")
	}
}

func TestBrowsingSessionDeletedCollectionStopsAtParent(t *testing.T) {
	m := newRichFixture().switchToView(viewBrowse)
	m = browsingTestEnter(t, browsingTestEnter(t, m))
	saved := m.captureBrowsingSession()
	fresh := newRichFixture()
	fresh.library.Index.BulkLoad([]library.Track{{Hash: "new", Artist: "Zoe", Album: "New", Title: "Other"}})
	fresh.browsingRestore = &browsingRestore{Session: saved}
	n, cmd := fresh.Update(browsingResumeMsg{})
	fresh = browsingTestDrain(t, n.(Model), cmd)
	if fresh.browsingRestore != nil || fresh.browse.Artists.Level != 0 || len(fresh.queue.Entries) != 3 {
		t.Fatal("opened a replacement collection")
	}
}

func TestBrowsingSessionLateLoadCannotOverrideUserNavigation(t *testing.T) {
	m := newRichFixture().switchToView(viewPlaylists)
	m = browsingTestEnter(t, m)
	fresh := newRichFixture()
	fresh.browsingRestore = &browsingRestore{Session: m.captureBrowsingSession()}
	n, cmd := fresh.Update(browsingResumeMsg{})
	fresh = n.(Model)
	if cmd == nil {
		t.Fatal("playlist did not load asynchronously")
	}
	n, _ = fresh.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	fresh = n.(Model)
	fresh = browsingTestDrain(t, fresh, cmd)
	if fresh.ActiveView != viewLibrary || !fresh.playlists.AtListLevel() {
		t.Fatal("late load changed user navigation")
	}
	if fresh.playlists.PendingLoadID != 0 || fresh.playlists.SmartQuery != nil {
		t.Fatal("canceled playlist retained pending load state")
	}
}

func TestBrowsingSessionCanceledInsightsCanReload(t *testing.T) {
	m := newRichFixture().switchToView(viewInsights)
	fresh := newRichFixture()
	fresh.browsingRestore = &browsingRestore{Session: m.captureBrowsingSession()}
	n, pending := fresh.Update(browsingResumeMsg{})
	fresh = n.(Model)
	if pending == nil || !fresh.insights.Loading {
		t.Fatal("Insights did not load asynchronously")
	}
	n, _ = fresh.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	fresh = browsingTestDrain(t, n.(Model), pending)
	if fresh.ActiveView != viewLibrary || fresh.insights.Loading {
		t.Fatal("canceled restoration retained loading state")
	}
	fresh = fresh.switchToView(viewInsights)
	cmd := fresh.insightsEnterCmd()
	if cmd == nil {
		t.Fatal("Insights could not reload after cancellation")
	}
	fresh = browsingTestDrain(t, fresh, cmd)
	if !fresh.insights.Loaded || fresh.insights.Loading {
		t.Fatal("Insights reload did not complete")
	}
}

func TestBrowsingSessionIgnoresInvalidFileAndCannotEnqueue(t *testing.T) {
	m := newRichFixture()
	m.dataDir = t.TempDir()
	for _, data := range []string{"broken", `{"version":9}`, `{"version":1,"view":99,"steps":[{}]}`} {
		if err := os.WriteFile(filepath.Join(m.dataDir, browsingSessionFile), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if m.loadBrowsingSession().browsingRestore != nil {
			t.Fatal("invalid session accepted")
		}
	}
	saved := m.captureBrowsingSession()
	saved.Steps = append(saved.Steps, saved.Steps[0])
	m.browsingRestore = &browsingRestore{Session: saved}
	n, cmd := m.Update(browsingResumeMsg{})
	m = browsingTestDrain(t, n.(Model), cmd)
	if len(m.queue.Entries) != 3 || m.playback.state != playback.StateIdle {
		t.Fatal("session replay enqueued a track")
	}
}

func TestBrowsingSessionInsightsSessions(t *testing.T) {
	m := newRichFixture().handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.switchToView(viewInsights)
	m.insights.Panel = instui.PanelSessions
	cmd := m.insightsEnterCmd()
	m = browsingTestDrain(t, m, cmd)
	for i, row := range m.insights.Rows() {
		if row.Drill.OpensSessions() {
			m.insights.Cursor = i
			break
		}
	}
	m = browsingTestEnter(t, m)
	if m.insights.Level != instui.LevelSessions {
		t.Fatal("fixture did not open sessions")
	}
	browsingTestRoundTrip(t, m)
	m = browsingTestEnter(t, m)
	if !m.insights.AtTracks() {
		t.Fatal("fixture did not open session tracks")
	}
	browsingTestRoundTrip(t, m)
}
