package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestBuildPaletteCommandsIncludesStableNavigationTargets(t *testing.T) {
	m := New(context.Background())
	commands := m.buildPaletteCommands()

	want := map[string]struct {
		name     string
		shortcut string
	}{
		"nav.library.all":     {name: "Go to Library: All Tracks"},
		"nav.library.recent":  {name: "Go to Library: Recently Added"},
		"nav.library.history": {name: "Go to Library: History"},
		"nav.playlists":       {name: "Go to Playlists", shortcut: "2"},
		"nav.queue":           {name: "Go to Queue", shortcut: "3"},
		"nav.browse.artists":  {name: "Go to Browse: Artists"},
		"nav.browse.albums":   {name: "Go to Browse: Albums"},
		"nav.browse.genres":   {name: "Go to Browse: Genres"},
		"nav.favorites":       {name: "Go to Favorites", shortcut: "5"},
		"nav.tags":            {name: "Go to Tags", shortcut: "6"},
		"nav.mixes":           {name: "Go to Mixes", shortcut: "7"},
	}

	seen := make(map[string]palette.PaletteCommand, len(commands))
	for _, command := range commands {
		seen[command.Key()] = command
	}
	for id, expected := range want {
		command, ok := seen[id]
		if !ok {
			t.Errorf("missing navigation command %q", id)
			continue
		}
		if command.Name != expected.name {
			t.Errorf("command %q name = %q, want %q", id, command.Name, expected.name)
		}
		if command.Category != "nav" {
			t.Errorf("command %q category = %q, want nav", id, command.Category)
		}
		if command.Shortcut != expected.shortcut {
			t.Errorf("command %q shortcut = %q, want %q", id, command.Shortcut, expected.shortcut)
		}
		if !isSafeDirectCommand(id) {
			t.Errorf("navigation command %q is not fixture-safe", id)
		}
	}
}

func TestNavigationPaletteSearchFindsRecentAndHistory(t *testing.T) {
	m := New(context.Background())

	for _, tc := range []struct {
		query string
		want  string
	}{
		{query: "recent", want: "Go to Library: Recently Added"},
		{query: "history", want: "Go to Library: History"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			cp, _ := palette.NewCommandPalette(m.buildPaletteCommands(), nil)
			cp.Filter.Text = tc.query
			cp.Refilter()

			var body []palette.PaletteCommand
			for _, command := range cp.Filtered {
				if command.Pinned == palette.PinNone {
					body = append(body, command)
				}
			}
			if len(body) != 1 {
				t.Fatalf("query %q returned %d body commands: %+v", tc.query, len(body), body)
			}
			if body[0].Name != tc.want {
				t.Fatalf("query %q returned %q, want %q", tc.query, body[0].Name, tc.want)
			}
		})
	}
}

func TestPaletteLibraryNavigationTargetsExactMode(t *testing.T) {
	reader := &stubHistoryReader{tracks: []library.Track{
		{Hash: "played", Path: "/played.flac", LastPlayedAt: 500},
	}}
	m := New(context.Background(), WithHistoryReader(reader))
	m.library.index.Insert(library.Track{Hash: "old", Path: "/old.flac", Artist: "A", CreatedAt: 100})
	m.library.index.Insert(library.Track{Hash: "new", Path: "/new.flac", Artist: "B", CreatedAt: 300})
	m.activeView = viewQueue

	m = runPaletteCommandForTest(t, m, "Go to Library: Recently Added")
	if m.activeView != viewLibrary || m.library.mode != LibraryModeRecent {
		t.Fatalf("recent command landed on view=%v mode=%v", m.activeView, m.library.mode)
	}
	if !m.library.showDateAdded || m.library.showPlayedAt {
		t.Fatalf("recent columns: showDateAdded=%v showPlayedAt=%v", m.library.showDateAdded, m.library.showPlayedAt)
	}
	if m.library.sortState != (sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortDesc}) {
		t.Fatalf("recent sort = %+v", m.library.sortState)
	}
	if focused, ok := m.library.view().FocusedTrack(); !ok || focused.Hash != "new" {
		t.Fatalf("recent focus = %+v, ok=%v; want newest track", focused, ok)
	}

	// Each Library mode owns its cursor/filter snapshot. Leaving and returning
	// through exact palette targets must restore that state.
	m.library.cursor = 1
	m.library.filter = filter.State{Text: "flac"}
	m = runPaletteCommandForTest(t, m, "Go to Library: All Tracks")
	if m.library.mode != LibraryModeAll || m.library.showDateAdded || m.library.showPlayedAt {
		t.Fatalf("all-tracks command left incorrect mode/columns: mode=%v added=%v played=%v",
			m.library.mode, m.library.showDateAdded, m.library.showPlayedAt)
	}
	m = runPaletteCommandForTest(t, m, "Go to Library: Recently Added")
	if m.library.cursor != 1 || m.library.filter.Text != "flac" {
		t.Fatalf("recent snapshot not restored: cursor=%d filter=%q", m.library.cursor, m.library.filter.Text)
	}

	m = runPaletteCommandForTest(t, m, "Go to Library: History")
	if m.library.mode != LibraryModeHistory || !m.library.showPlayedAt || m.library.showDateAdded {
		t.Fatalf("history command left incorrect mode/columns: mode=%v added=%v played=%v",
			m.library.mode, m.library.showDateAdded, m.library.showPlayedAt)
	}
	if reader.calls != 1 || len(m.library.sourceTracks) != 1 || m.library.sourceTracks[0].Hash != "played" {
		t.Fatalf("history was not loaded: calls=%d tracks=%+v", reader.calls, m.library.sourceTracks)
	}
	if m.library.sortState != (sorting.SortState{Field: sorting.SortLastPlayed, Dir: sorting.SortDesc}) {
		t.Fatalf("history sort = %+v", m.library.sortState)
	}
}

func TestPaletteBrowseNavigationTargetsExactModeAndPreservesState(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.browse.albums.albums = []string{"Alpha", "Beta", "Gamma"}
	m.browse.albums.cursor = 2
	m.browse.albums.filter = filter.State{Text: "a"}

	m = runPaletteCommandForTest(t, m, "Go to Browse: Albums")
	if m.activeView != viewBrowse || m.browse.mode != BrowseModeAlbums {
		t.Fatalf("albums command landed on view=%v mode=%v", m.activeView, m.browse.mode)
	}
	if m.browse.albums.cursor != 2 || m.browse.albums.filter.Text != "a" {
		t.Fatalf("albums state changed: cursor=%d filter=%q", m.browse.albums.cursor, m.browse.albums.filter.Text)
	}

	for _, tc := range []struct {
		name string
		mode BrowseMode
	}{
		{name: "Go to Browse: Artists", mode: BrowseModeArtists},
		{name: "Go to Browse: Genres", mode: BrowseModeGenres},
	} {
		m = runPaletteCommandForTest(t, m, tc.name)
		if m.activeView != viewBrowse || m.browse.mode != tc.mode {
			t.Errorf("%q landed on view=%v mode=%v", tc.name, m.activeView, m.browse.mode)
		}
	}
}

func TestPalettePrimaryNavigationTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		view viewID
	}{
		{name: "Go to Playlists", view: viewPlaylists},
		{name: "Go to Queue", view: viewQueue},
		{name: "Go to Favorites", view: viewFavorites},
		{name: "Go to Tags", view: viewTags},
		{name: "Go to Mixes", view: viewMixes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background())
			m = runPaletteCommandForTest(t, m, tc.name)
			if m.activeView != tc.view {
				t.Fatalf("active view = %v, want %v", m.activeView, tc.view)
			}
		})
	}
}

func TestFixtureDriverRunsExactNavigationByName(t *testing.T) {
	driver := newTestDriver(t, FixtureRich, 100, 30)
	for _, tc := range []struct {
		name string
		mode string
	}{
		{name: "Go to Library: Recently Added", mode: "Recent"},
		{name: "Go to Library: History", mode: "History"},
	} {
		if err := driver.runCommand(tc.name); err != nil {
			t.Fatalf("runCommand(%q): %v", tc.name, err)
		}
		got := driver.model.Snapshot()
		if got.PrimaryView != "Library" || got.Mode != tc.mode {
			t.Fatalf("runCommand(%q) snapshot = view %q mode %q", tc.name, got.PrimaryView, got.Mode)
		}
	}
}

func runPaletteCommandForTest(t *testing.T, m Model, name string) Model {
	t.Helper()
	for _, command := range m.buildPaletteCommands() {
		if command.Name != name {
			continue
		}
		if command.Exec == nil {
			t.Fatalf("palette command %q has no Exec", name)
		}
		msg := command.Exec()()
		next, _ := m.Update(msg)
		result, ok := next.(Model)
		if !ok {
			t.Fatalf("palette command %q returned model %T", name, next)
		}
		return result
	}
	t.Fatalf("palette command %q not found", name)
	return m
}
