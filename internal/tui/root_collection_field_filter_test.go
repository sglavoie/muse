package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func collectionFilterSetups(tracks []library.Track) map[string]func(*Model) {
	return map[string]func(*Model){
		"artists": func(m *Model) {
			m.ActiveView = viewBrowse
			m.browse.Mode = browse.BrowseModeArtists
			m.browse.Artists.Level, m.browse.Artists.Tracks = arttui.TrackLevel, tracks
		},
		"albums": func(m *Model) {
			m.ActiveView = viewBrowse
			m.browse.Mode = browse.BrowseModeAlbums
			m.browse.Albums.Level, m.browse.Albums.Tracks = albtui.TrackLevel, tracks
		},
		"genres": func(m *Model) {
			m.ActiveView = viewBrowse
			m.browse.Mode = browse.BrowseModeGenres
			m.browse.Genres.Level, m.browse.Genres.Tracks = gentui.TrackLevel, tracks
		},
		"favorite tracks": func(m *Model) {
			m.ActiveView = viewFavorites
			m.favorites.Level, m.favorites.Category = favtui.ListLevel, favtui.CategoryTracks
			m.favorites.Tracks = tracks
		},
		"favorite collection": func(m *Model) {
			m.ActiveView = viewFavorites
			m.favorites.Level, m.favorites.DrilldownTracks = favtui.TrackLevel, tracks
		},
		"tags": func(m *Model) {
			m.ActiveView = viewTags
			m.tagsView.Level = tagtui.TrackLevel
			m.tagsView.Tracks = nil
			for _, tr := range tracks {
				m.tagsView.Tracks = append(m.tagsView.Tracks, db.PlaylistTrackRow{
					Hash: tr.Hash, Path: tr.Path, Title: tr.Title, Artist: tr.Artist, Album: tr.Album, Genre: tr.Genre,
				})
			}
		},
	}
}

func TestCollectionTrackFiltersMatchAcrossViews(t *testing.T) {
	tracks := []library.Track{
		{Hash: "one", Title: "Live One", Artist: "Miles Davis", Album: "Kind of Blue", Genre: "Jazz"},
		{Hash: "two", Title: "Live Two", Artist: "Miles Davis", Album: "Other", Genre: "Fusion"},
		{Hash: "three", Title: "Miles Davis Tribute", Artist: "AC/DC", Album: "Kind of Blue", Genre: "Rock"},
	}
	for name, setup := range collectionFilterSetups(tracks) {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				query string
				want  []string
			}{
				{"a:Miles Davis", []string{"one", "two"}},
				{"ARTIST:MILES DAVIS genre:jazz", []string{"one"}},
				{"live a:Miles Davis l:Kind of Blue", []string{"one"}},
				{"title:Tribute album:Kind of Blue", []string{"three"}},
				{"a: g:Jazz", []string{"one"}},
				{"artist:", []string{"one", "two", "three"}},
				{"a:missing", nil},
				{"AC/DC", []string{"three"}},
			} {
				m := newRichFixture()
				setup(&m)
				m = typeText(pressSlash(m), tc.query)
				if got := m.filterRowKeys(); !slices.Equal(got, tc.want) {
					t.Fatalf("%q: got %v, want %v", tc.query, got, tc.want)
				}
				if library.ParseFilterQuery(tc.query).IsFieldSpecific() && m.filterQuerySummary() == "" {
					t.Fatalf("%q: missing field summary", tc.query)
				}
				m = pressEnter(m)
				if len(tc.want) > 0 && m.focusedTrackHash() != tc.want[0] {
					t.Fatalf("%q: action target %q, want %q", tc.query, m.focusedTrackHash(), tc.want[0])
				}
				m = pressEsc(m)
				if got := m.filterRowKeys(); !slices.Equal(got, []string{"one", "two", "three"}) {
					t.Fatalf("clearing filter failed: %v", got)
				}
			}
		})
	}
}

func TestCollectionTrackFiltersRecallSharedHistory(t *testing.T) {
	tracks := []library.Track{{Hash: "one", Artist: "Miles Davis", Genre: "Jazz"}}
	for name, setup := range collectionFilterSetups(tracks) {
		t.Run(name, func(t *testing.T) {
			m := newRichFixture()
			m = pressEnter(typeText(pressSlash(m), "a:Miles Davis g:Jazz"))
			setup(&m)
			m = pressKey(pressSlash(m), "up")
			if got := m.filterRowKeys(); !slices.Equal(got, []string{"one"}) {
				t.Fatalf("recalled query did not match: %v", got)
			}
			m.termWidth = 100
			m.showFooterHints = true
			if out := ansi.Strip(m.renderFooterHint()); !strings.Contains(out, "Artist: Miles Davis") || !strings.Contains(out, "Genre: Jazz") {
				t.Fatalf("missing recalled query summary: %q", out)
			}
		})
	}
}

func TestCollectionNameFiltersStayLiteral(t *testing.T) {
	m := newRichFixture()
	for _, view := range []viewID{viewBrowse, viewFavorites, viewTags} {
		m.ActiveView = view
		m = typeText(pressSlash(m), "a:Alice")
		if m.filterQuerySummary() != "" {
			t.Fatal("collection names must not advertise field matching")
		}
		m = pressEsc(m)
	}
}

func TestCollectionFilenameAccentFilters(t *testing.T) {
	tracks := []library.Track{{Hash: "accent", Path: "/private/Cafe\u0301.flac", Artist: "Björk"}}
	for name, setup := range collectionFilterSetups(tracks) {
		t.Run(name, func(t *testing.T) {
			for _, query := range []string{"cafe", "cafe a:bjork", "a:BJORK"} {
				m := newRichFixture()
				setup(&m)
				m = typeText(pressSlash(m), query)
				if got := m.filterRowKeys(); !slices.Equal(got, []string{"accent"}) {
					t.Fatalf("%q: %v", query, got)
				}
			}
		})
	}
}
