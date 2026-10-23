package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestBuildPaletteCommandsIncludesStableNavigationTargets(t *testing.T) {
	m := New(context.Background())
	commands := m.BuildPaletteCommands()

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
		if !IsSafeDirectCommand(id) {
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
			cp, _ := palette.NewCommandPalette(m.BuildPaletteCommands(), nil)
			cp.Filter.Text = tc.query
			cp.Refilter()

			var body []palette.PaletteCommand
			for _, command := range cp.Filtered {
				if command.Pinned == palette.PinNone {
					body = append(body, command)
				}
			}
			wantNames := map[string]bool{tc.want: true}
			if tc.query == "recent" {
				wantNames["Show recent messages"] = true
			}
			for _, command := range body {
				if !wantNames[command.Name] {
					t.Fatalf("unexpected match for %q: %q", tc.query, command.Name)
				}
				delete(wantNames, command.Name)
			}
			if len(wantNames) != 0 {
				t.Fatalf("query %q missing commands: %v", tc.query, wantNames)
			}

		})
	}
}

func TestPaletteLibraryNavigationTargetsExactMode(t *testing.T) {
	reader := &stubHistoryReader{tracks: []library.Track{
		{Hash: "played", Path: "/played.flac", LastPlayedAt: 500},
	}}
	m := New(context.Background(), WithHistoryReader(reader))
	m.library.Index.Insert(library.Track{Hash: "old", Path: "/old.flac", Artist: "A", CreatedAt: 100})
	m.library.Index.Insert(library.Track{Hash: "new", Path: "/new.flac", Artist: "B", CreatedAt: 300})
	m.ActiveView = viewQueue

	m = runPaletteCommandForTest(t, m, "Go to Library: Recently Added")
	if m.ActiveView != viewLibrary || m.library.Mode != libtui.LibraryModeRecent {
		t.Fatalf("recent command landed on view=%v mode=%v", m.ActiveView, m.library.Mode)
	}
	if !m.library.ShowDateAdded || m.library.ShowPlayedAt {
		t.Fatalf("recent columns: showDateAdded=%v showPlayedAt=%v", m.library.ShowDateAdded, m.library.ShowPlayedAt)
	}
	if m.library.SortState != (sorting.SortState{Field: sorting.SortDateAdded, Dir: sorting.SortDesc}) {
		t.Fatalf("recent sort = %+v", m.library.SortState)
	}
	if focused, ok := m.library.Panel().FocusedTrack(); !ok || focused.Hash != "new" {
		t.Fatalf("recent focus = %+v, ok=%v; want newest track", focused, ok)
	}

	// Each Library mode owns its cursor/filter snapshot. Leaving and returning
	// through exact palette targets must restore that state.
	m.library.Cursor = 1
	m.library.Filter = filter.State{Text: "flac"}
	m = runPaletteCommandForTest(t, m, "Go to Library: All Tracks")
	if m.library.Mode != libtui.LibraryModeAll || m.library.ShowDateAdded || m.library.ShowPlayedAt {
		t.Fatalf("all-tracks command left incorrect mode/columns: mode=%v added=%v played=%v",
			m.library.Mode, m.library.ShowDateAdded, m.library.ShowPlayedAt)
	}
	m = runPaletteCommandForTest(t, m, "Go to Library: Recently Added")
	if m.library.Cursor != 1 || m.library.Filter.Text != "flac" {
		t.Fatalf("recent snapshot not restored: cursor=%d filter=%q", m.library.Cursor, m.library.Filter.Text)
	}

	m = runPaletteCommandForTest(t, m, "Go to Library: History")
	if m.library.Mode != libtui.LibraryModeHistory || !m.library.ShowPlayedAt || m.library.ShowDateAdded {
		t.Fatalf("history command left incorrect mode/columns: mode=%v added=%v played=%v",
			m.library.Mode, m.library.ShowDateAdded, m.library.ShowPlayedAt)
	}
	if reader.calls != 1 || len(m.library.SourceTracks) != 1 || m.library.SourceTracks[0].Hash != "played" {
		t.Fatalf("history was not loaded: calls=%d tracks=%+v", reader.calls, m.library.SourceTracks)
	}
	if m.library.SortState != (sorting.SortState{Field: sorting.SortLastPlayed, Dir: sorting.SortDesc}) {
		t.Fatalf("history sort = %+v", m.library.SortState)
	}
}

func TestPaletteBrowseNavigationTargetsExactModeAndPreservesState(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.browse.Albums.Albums = []string{"Alpha", "Beta", "Gamma"}
	m.browse.Albums.Cursor = 2
	m.browse.Albums.Filter = filter.State{Text: "a"}

	m = runPaletteCommandForTest(t, m, "Go to Browse: Albums")
	if m.ActiveView != viewBrowse || m.browse.Mode != browse.BrowseModeAlbums {
		t.Fatalf("albums command landed on view=%v mode=%v", m.ActiveView, m.browse.Mode)
	}
	if m.browse.Albums.Cursor != 2 || m.browse.Albums.Filter.Text != "a" {
		t.Fatalf("albums state changed: cursor=%d filter=%q", m.browse.Albums.Cursor, m.browse.Albums.Filter.Text)
	}

	for _, tc := range []struct {
		name string
		mode browse.BrowseMode
	}{
		{name: "Go to Browse: Artists", mode: browse.BrowseModeArtists},
		{name: "Go to Browse: Genres", mode: browse.BrowseModeGenres},
	} {
		m = runPaletteCommandForTest(t, m, tc.name)
		if m.ActiveView != viewBrowse || m.browse.Mode != tc.mode {
			t.Errorf("%q landed on view=%v mode=%v", tc.name, m.ActiveView, m.browse.Mode)
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
			if m.ActiveView != tc.view {
				t.Fatalf("active view = %v, want %v", m.ActiveView, tc.view)
			}
		})
	}
}

func runPaletteCommandForTest(t *testing.T, m Model, name string) Model {
	t.Helper()
	for _, command := range m.BuildPaletteCommands() {
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

func TestPaletteBroadSearchOpensQueueAndExplicitSearchClearsIt(t *testing.T) {
	m := newRichFixture()
	cp, _ := palette.NewCommandPalette(m.BuildPaletteCommands(), nil)
	for _, tc := range []struct{ query, id string }{
		{"queue", "nav.queue"},
		{"clear queue", "queue.clear"},
		{"playlist", "nav.playlists"},
		{"tags", "nav.tags"},
	} {
		cp.Filter.Text = tc.query
		cp.Refilter()
		if len(cp.Filtered) == 0 || cp.Filtered[0].ID != tc.id {
			t.Fatalf("query %q: %+v", tc.query, cp.Filtered)
		}
	}
}
