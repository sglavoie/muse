package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestHandleActionReservesFilterBarLine_Artists verifies that ArtistsView.HandleAction
// accounts for the filter bar line when computing visible rows.
func TestHandleActionReservesFilterBarLine_Artists(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist %02d", i)
	}
	v := ArtistsView{height: 12, artists: artists, filter: filter.State{Active: true}}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.cursor)
	}
	if v.scrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.scrollOffset)
	}
}

// TestHandleActionReservesFilterBarLine_Albums verifies the same fix for AlbumsView.
func TestHandleActionReservesFilterBarLine_Albums(t *testing.T) {
	albums := make([]string, 20)
	for i := range albums {
		albums[i] = fmt.Sprintf("Album %02d", i)
	}
	v := AlbumsView{height: 12, albums: albums, filter: filter.State{Active: true}}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.cursor)
	}
	if v.scrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.scrollOffset)
	}
}

// TestHandleActionReservesFilterBarLine_Genres verifies the same fix for GenresView.
func TestHandleActionReservesFilterBarLine_Genres(t *testing.T) {
	genres := make([]string, 20)
	for i := range genres {
		genres[i] = fmt.Sprintf("Genre %02d", i)
	}
	v := GenresView{height: 12, genres: genres, filter: filter.State{Active: true}}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.cursor)
	}
	if v.scrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.scrollOffset)
	}
}

// TestFavoritesViewPerCellArtistFavoriteStyle verifies that in the favorites
// track list, an artist-favorited row applies FavArtist to the artist cell only.
func TestFavoritesViewPerCellArtistFavoriteStyle(t *testing.T) {
	tracks := []library.Track{{Title: "Song", Artist: "FavArtist", Album: "SomeAlbum", Hash: "h1", Duration: 60}}
	s := styles.NewStyles(true)
	fv := favtui.View{
		Level:           favtui.ListLevel,
		Category:        favtui.CategoryTracks,
		Tracks:          tracks,
		Width:           120,
		Height:          40,
		Styles:          s,
		FavoriteHashes:  map[string]struct{}{},
		FavoriteArtists: map[string]struct{}{"FavArtist": {}},
		FavoriteAlbums:  map[string]struct{}{},
	}
	out := fv.Render()

	artistRef := s.FavArtistAccent.Render("REF")
	if ansi.Strip(artistRef) == artistRef {
		return
	}
	artistPrefix, _, _ := strings.Cut(artistRef, "REF")
	titleRef := s.FavoriteAccent.Render("REF")
	titlePrefix, _, _ := strings.Cut(titleRef, "REF")

	if !strings.Contains(out, artistPrefix) {
		t.Errorf("favorites view artist-only fav: artist cell must use FavArtistAccent style; got:\n%s", ansi.Strip(out))
	}
	if strings.Contains(out, titlePrefix) {
		t.Errorf("favorites view artist-only fav: title cell must NOT use FavoriteAccent style; got:\n%s", ansi.Strip(out))
	}
}

// TestHandleActionReservesFilterBarLine_Favorites verifies the same fix for
// FavoritesView at ListLevel with artists.
func TestHandleActionReservesFilterBarLine_Favorites(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("FavArtist %02d", i)
	}
	v := FavoritesView{
		height:   12,
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
		artists:  artists,
		filter:   filter.State{Active: true},
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.cursor)
	}
	if v.scrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.scrollOffset)
	}
}
