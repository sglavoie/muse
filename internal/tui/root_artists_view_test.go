package tui

import (
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestArtistsViewWrapperHandleAction verifies that the ArtistsView HandleAction
// properly delegates cursor navigation.
func TestArtistsViewWrapperHandleAction(t *testing.T) {
	v := browse.ArtistsView{
		Artists: []string{"Alice", "Bob", "Charlie"},
		Cursor:  0,
		Height:  20,
	}

	v = v.HandleAction(keys.ActionCursorDown)
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.Cursor)
	}

	v = v.HandleAction(keys.ActionCursorUp)
	if v.Cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.Cursor)
	}

	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 2 {
		t.Errorf("GoToBottom: expected 2, got %d", v.Cursor)
	}

	v = v.HandleAction(keys.ActionGoToTop)
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.Cursor)
	}
}

// TestArtistsViewFocusedMethods verifies focusedArtist, focusedAlbum, and
// focusedTrack return the correct values and false when out of bounds.
func TestArtistsViewFocusedMethods(t *testing.T) {
	v := browse.ArtistsView{
		Level:   arttui.ArtistLevel,
		Artists: []string{"Alice", "Bob"},
		Cursor:  1,
	}
	artist, ok := v.FocusedArtist()
	if !ok || artist != "Bob" {
		t.Errorf("focusedArtist: expected (Bob, true), got (%q, %v)", artist, ok)
	}

	v.Cursor = 5
	_, ok = v.FocusedArtist()
	if ok {
		t.Error("focusedArtist: expected false when cursor out of bounds")
	}

	v.Level = arttui.AlbumLevel
	v.Cursor = 0
	_, ok = v.FocusedArtist()
	if ok {
		t.Error("focusedArtist: expected false at AlbumLevel")
	}

	v.Level = arttui.AlbumLevel
	v.Albums = []string{"Alpha", "Beta"}
	v.Cursor = 0
	album, ok := v.FocusedAlbum()
	if !ok || album != "Alpha" {
		t.Errorf("focusedAlbum: expected (Alpha, true), got (%q, %v)", album, ok)
	}

	v.Level = arttui.TrackLevel
	v.Tracks = []library.Track{{Title: "Song A"}}
	v.Cursor = 0
	track, ok := v.FocusedTrack()
	if !ok || track.Title != "Song A" {
		t.Errorf("focusedTrack: expected (Song A, true), got (%q, %v)", track.Title, ok)
	}
}

// TestArtistsViewClampCursor verifies clampCursor clamps at each level.
func TestArtistsViewClampCursor(t *testing.T) {
	t.Run("empty artist list", func(t *testing.T) {
		v := browse.ArtistsView{Cursor: 5, ScrollOffset: 3}
		v = v.ClampCursor()
		if v.Cursor != 0 || v.ScrollOffset != 0 {
			t.Errorf("expected cursor=0 scrollOffset=0, got cursor=%d scrollOffset=%d", v.Cursor, v.ScrollOffset)
		}
	})

	t.Run("cursor beyond list end", func(t *testing.T) {
		v := browse.ArtistsView{Artists: []string{"Alice", "Bob"}, Cursor: 5}
		v = v.ClampCursor()
		if v.Cursor != 1 {
			t.Errorf("expected cursor=1, got %d", v.Cursor)
		}
	})

	t.Run("cursor within bounds is unchanged", func(t *testing.T) {
		v := browse.ArtistsView{Artists: []string{"Alice", "Bob", "Charlie"}, Cursor: 1}
		v = v.ClampCursor()
		if v.Cursor != 1 {
			t.Errorf("expected cursor=1 (unchanged), got %d", v.Cursor)
		}
	})

	t.Run("album level", func(t *testing.T) {
		v := browse.ArtistsView{Level: arttui.AlbumLevel, Albums: []string{"Alpha"}, Cursor: 3}
		v = v.ClampCursor()
		if v.Cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.Cursor)
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := browse.ArtistsView{Level: arttui.TrackLevel, Tracks: []library.Track{{Title: "T1"}}, Cursor: 5}
		v = v.ClampCursor()
		if v.Cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.Cursor)
		}
	})
}

// TestArtistsViewEnqueueFocusedTrack verifies the enqueue helper.
func TestArtistsViewEnqueueFocusedTrack(t *testing.T) {
	v := browse.ArtistsView{Level: arttui.TrackLevel, Tracks: []library.Track{{Title: "Song A", Hash: "h1"}}, Cursor: 0}
	qa := &stubQueueAppender{}
	cmd := v.EnqueueFocusedTrack(qa, 0)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	enqueued, ok := msg.(libtui.TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", msg)
	}
	if enqueued.Track.Title != "Song A" {
		t.Errorf("expected 'Song A', got %q", enqueued.Track.Title)
	}
	if !enqueued.WasEmpty {
		t.Error("expected WasEmpty=true when prevQueueLen=0")
	}
}

// TestArtistsViewEnqueueFocusedTrackNotAtTrackLevel returns nil.
func TestArtistsViewEnqueueFocusedTrackNotAtTrackLevel(t *testing.T) {
	v := browse.ArtistsView{Level: arttui.ArtistLevel, Artists: []string{"Alice"}, Cursor: 0}
	qa := &stubQueueAppender{}
	cmd := v.EnqueueFocusedTrack(qa, 0)
	if cmd != nil {
		t.Fatal("expected nil cmd when not at TrackLevel")
	}
}

// TestArtistsDrillDownPreservesWidthHeight verifies that openAlbums and
// openTracks do not reset width/height.
func TestArtistsDrillDownPreservesWidthHeight(t *testing.T) {
	v := browse.ArtistsView{Level: arttui.ArtistLevel, Artists: []string{"Alice"}, Width: 120, Height: 40}

	v = v.OpenAlbums([]string{"Alpha"})
	if v.Width != 120 || v.Height != 40 {
		t.Errorf("openAlbums changed width/height: %d/%d", v.Width, v.Height)
	}

	v = v.OpenTracks([]library.Track{{Title: "T1"}})
	if v.Width != 120 || v.Height != 40 {
		t.Errorf("openTracks changed width/height: %d/%d", v.Width, v.Height)
	}
}

// TestArtistsClampCursor_WithFilter verifies that clampCursor accounts for
// the active filter rather than the raw list length.
func TestArtistsClampCursor_WithFilter(t *testing.T) {
	m := newModelWithTracks()
	m.browse.Artists.Artists = arttui.DeriveArtists(m.library.Index.Tracks())
	m.browse.Artists.Level = arttui.ArtistLevel
	m.browse.Artists.Filter.Text = "ali"
	m.browse.Artists.Cursor = 2
	m.browse.Artists.ScrollOffset = 2

	m.browse.Artists = m.browse.Artists.ClampCursor()

	if m.browse.Artists.Cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered match), got %d", m.browse.Artists.Cursor)
	}
	if m.browse.Artists.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.browse.Artists.ScrollOffset)
	}
}

// TestArtistsClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down when it ends up above the cursor.
func TestArtistsClampCursor_ScrollOffsetClamped(t *testing.T) {
	m := newModelWithTracks()
	m.browse.Artists.Artists = []string{"Alice"}
	m.browse.Artists.Level = arttui.ArtistLevel
	m.browse.Artists.Cursor = 5
	m.browse.Artists.ScrollOffset = 10

	m.browse.Artists = m.browse.Artists.ClampCursor()

	if m.browse.Artists.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.browse.Artists.Cursor)
	}
	if m.browse.Artists.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.browse.Artists.ScrollOffset)
	}
}

// TestArtistsClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestArtistsClampCursor_TrackLevelWithFilter(t *testing.T) {
	m := newModelWithTracks()
	m.browse.Artists.Level = arttui.TrackLevel
	m.browse.Artists.Tracks = []library.Track{
		{Hash: "h1", Title: "Foo", Artist: "A"},
		{Hash: "h2", Title: "Bar", Artist: "B"},
		{Hash: "h3", Title: "Baz", Artist: "C"},
	}
	m.browse.Artists.Filter.Text = "ba"
	m.browse.Artists.Cursor = 3
	m.browse.Artists.ScrollOffset = 3

	m.browse.Artists = m.browse.Artists.ClampCursor()

	if m.browse.Artists.Cursor != 1 {
		t.Errorf("expected cursor=1 (last filtered index), got %d", m.browse.Artists.Cursor)
	}
	if m.browse.Artists.ScrollOffset > m.browse.Artists.Cursor {
		t.Errorf("scrollOffset %d should not be above cursor %d", m.browse.Artists.ScrollOffset, m.browse.Artists.Cursor)
	}
}

// TestArtistsGoToBottomScrollOffsetIndependentOfSort verifies that GoToBottom
// produces the same scrollOffset regardless of sort state, because the state
// strip absorbs the sort chunk without reserving an extra row.
func TestArtistsGoToBottomScrollOffsetIndependentOfSort(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist_%02d", i)
	}

	v := browse.ArtistsView{Level: arttui.ArtistLevel, Cursor: 0, Artists: artists, Height: 12}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.Cursor)
	}
	scrollNoSort := v.ScrollOffset

	v2 := browse.ArtistsView{
		Level:     arttui.ArtistLevel,
		Cursor:    0,
		Artists:   artists,
		Height:    12,
		SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v2 = v2.HandleAction(keys.ActionGoToBottom)
	if v2.Cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset = %d (strip absorbs sort), got %d", scrollNoSort, v2.ScrollOffset)
	}
}
