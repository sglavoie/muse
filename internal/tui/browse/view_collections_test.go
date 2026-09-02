package browse

import (
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestHandleActionReservesFilterBarLine_Artists verifies that ArtistsView.HandleAction
// accounts for the filter bar line when computing visible rows.
func TestHandleActionReservesFilterBarLine_Artists(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist %02d", i)
	}
	v := ArtistsView{Height: 12, Artists: artists, Filter: filter.State{Active: true}}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.Cursor)
	}
	if v.ScrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.ScrollOffset)
	}
}

// TestHandleActionReservesFilterBarLine_Albums verifies the same fix for AlbumsView.
func TestHandleActionReservesFilterBarLine_Albums(t *testing.T) {
	albums := make([]string, 20)
	for i := range albums {
		albums[i] = fmt.Sprintf("Album %02d", i)
	}
	v := AlbumsView{Height: 12, Albums: albums, Filter: filter.State{Active: true}}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.Cursor)
	}
	if v.ScrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.ScrollOffset)
	}
}

// TestHandleActionReservesFilterBarLine_Genres verifies the same fix for GenresView.
func TestHandleActionReservesFilterBarLine_Genres(t *testing.T) {
	genres := make([]string, 20)
	for i := range genres {
		genres[i] = fmt.Sprintf("Genre %02d", i)
	}
	v := GenresView{Height: 12, Genres: genres, Filter: filter.State{Active: true}}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.Cursor)
	}
	if v.ScrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.ScrollOffset)
	}
}
