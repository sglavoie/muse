package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestCollectionBackRestoresFilteredParent(t *testing.T) {
	cases := []struct {
		name, query string
		setup       func(Model) Model
	}{
		{"artists", "Bob", func(m Model) Model { m.ActiveView = viewBrowse; m.browse.Mode = browse.BrowseModeArtists; return m }},
		{"albums", "Gamma", func(m Model) Model { m.ActiveView = viewBrowse; m.browse.Mode = browse.BrowseModeAlbums; return m }},
		{"genres", "Techno", func(m Model) Model { m.ActiveView = viewBrowse; m.browse.Mode = browse.BrowseModeGenres; return m }},
		{"favorites", "Bob", func(m Model) Model {
			m.ActiveView = viewFavorites
			next, _ := m.Update(favoritesLoadedMsg{category: 0, artists: []string{"Alice", "Bob"}})
			return next.(Model)
		}},
		{"playlists", "Evening", func(m Model) Model { m.ActiveView = viewPlaylists; return m }},
		{"tags", "focus", func(m Model) Model { m.ActiveView = viewTags; return m }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.setup(newRichFixture())
			m = m.switchToView(m.ActiveView)
			if m.ActiveView == viewBrowse {
				m = m.deriveCurrentBrowseData()
			}
			m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
			m = pressEnter(typeText(pressSlash(m), tc.query))
			before := m.filterRowKeys()
			if len(before) == 0 {
				t.Fatal("fixture query has no matches")
			}
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(Model)
			// Playlist/tag reads complete asynchronously; apply their real fixture result.
			if tc.name == "playlists" || tc.name == "tags" {
				if cmd == nil {
					t.Fatal("missing collection load")
				}
				next, _ = m.Update(cmd())
				m = next.(Model)
			}
			m = pressEsc(m)
			if m.navigationFilter().Text != tc.query {
				t.Fatalf("lost parent filter: %+v", m.navigationFilter())
			}
			_, cursor, _ := m.sortPreferenceTarget()
			keys := m.filterRowKeys()
			if *cursor >= len(keys) || keys[*cursor] != before[0] {
				t.Fatalf("lost parent identity: cursor %d, rows %v, want %q", *cursor, keys, before[0])
			}
			// Reopening must target the same collection, and clearing the restored
			// filter must still use its original stable position.
			m = pressEsc(m)
			_, cursor, _ = m.sortPreferenceTarget()
			keys = m.filterRowKeys()
			if keys[*cursor] != before[0] {
				t.Fatalf("clear lost identity: %v at %d", keys, *cursor)
			}
		})
	}
}

func TestCollectionBackRestoresNestedContextAfterMutationAndResize(t *testing.T) {
	tracks := make([]library.Track, 40)
	for i := range tracks {
		tracks[i] = library.Track{Hash: fmt.Sprint(i), Artist: fmt.Sprintf("Artist %02d", i), Album: "Album", Title: "Song"}
	}
	m := newModelWithTracks()
	m.library.Index.BulkLoad(tracks)
	m = m.switchToView(viewBrowse)
	m.browse.Mode = browse.BrowseModeArtists
	m = m.rederiveArtistsData()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = pressEnter(typeText(pressSlash(m), "Artist"))
	m.browse.Artists.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortDesc}
	m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
	m.browse.Artists.Cursor = 20
	m.browse.Artists.ScrollOffset = 15
	want, _ := m.browse.Artists.FocusedArtist()
	m = pressEnter(m)
	m = pressEnter(typeText(pressSlash(m), "Album"))
	m = pressEnter(m)
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	m = pressEsc(m)
	if m.browse.Artists.Filter.Text != "Album" {
		t.Fatal("nested album filter lost")
	}
	m = pressEsc(m) // clear album filter
	m = pressEsc(m) // back to artists
	if m.browse.Artists.Filter.Text != "Artist" || m.browse.Artists.SortState.Dir != sorting.SortDesc {
		t.Fatal("parent search/sort lost")
	}
	got, _ := m.browse.Artists.FocusedArtist()
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), want) {
		t.Fatal("restored focus is offscreen after resize")
	}
	// A missing parent falls back to a valid neighboring row.
	m = pressEnter(m)
	m.browse.Artists.Artists = nil
	m.library.Index.BulkLoad(nil)
	m = pressEsc(m)
	if m.browse.Artists.Cursor != 0 || m.browse.Artists.ScrollOffset != 0 {
		t.Fatal("empty parent not clamped")
	}
}

func TestCollectionBackFollowsIdentityAfterParentInsertion(t *testing.T) {
	m := newRichFixture().switchToView(viewBrowse)
	m = pressEnter(typeText(pressSlash(m), "o"))
	if name, ok := m.browse.Artists.FocusedArtist(); !ok || name != "Bob" {
		t.Fatalf("fixture focus: %q", name)
	}
	m = pressEnter(m)
	tracks := append([]library.Track(nil), m.library.Index.Tracks()...)
	tracks = append(tracks, library.Track{Hash: "new", Artist: "Aaron", Album: "New album", Title: "New song"})
	m.library.Index.BulkLoad(tracks)
	m = pressEsc(m)
	if name, _ := m.browse.Artists.FocusedArtist(); name != "Bob" || m.browse.Artists.Cursor != 1 {
		t.Fatalf("parent insertion changed focus to %q at %d", name, m.browse.Artists.Cursor)
	}
}
