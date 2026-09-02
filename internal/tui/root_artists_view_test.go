package tui

import (
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestArtistsViewWrapperHandleAction verifies that the ArtistsView HandleAction
// properly delegates cursor navigation.
func TestArtistsViewWrapperHandleAction(t *testing.T) {
	v := ArtistsView{
		artists: []string{"Alice", "Bob", "Charlie"},
		cursor:  0,
		height:  20,
	}

	v = v.HandleAction(actionCursorDown)
	if v.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.cursor)
	}

	v = v.HandleAction(actionCursorUp)
	if v.cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.cursor)
	}

	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 2 {
		t.Errorf("GoToBottom: expected 2, got %d", v.cursor)
	}

	v = v.HandleAction(actionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

// TestArtistsViewFocusedMethods verifies focusedArtist, focusedAlbum, and
// focusedTrack return the correct values and false when out of bounds.
func TestArtistsViewFocusedMethods(t *testing.T) {
	v := ArtistsView{
		level:   arttui.ArtistLevel,
		artists: []string{"Alice", "Bob"},
		cursor:  1,
	}
	artist, ok := v.focusedArtist()
	if !ok || artist != "Bob" {
		t.Errorf("focusedArtist: expected (Bob, true), got (%q, %v)", artist, ok)
	}

	v.cursor = 5
	_, ok = v.focusedArtist()
	if ok {
		t.Error("focusedArtist: expected false when cursor out of bounds")
	}

	v.level = arttui.AlbumLevel
	v.cursor = 0
	_, ok = v.focusedArtist()
	if ok {
		t.Error("focusedArtist: expected false at AlbumLevel")
	}

	v.level = arttui.AlbumLevel
	v.albums = []string{"Alpha", "Beta"}
	v.cursor = 0
	album, ok := v.focusedAlbum()
	if !ok || album != "Alpha" {
		t.Errorf("focusedAlbum: expected (Alpha, true), got (%q, %v)", album, ok)
	}

	v.level = arttui.TrackLevel
	v.tracks = []library.Track{{Title: "Song A"}}
	v.cursor = 0
	track, ok := v.focusedTrack()
	if !ok || track.Title != "Song A" {
		t.Errorf("focusedTrack: expected (Song A, true), got (%q, %v)", track.Title, ok)
	}
}

// TestArtistsViewClampCursor verifies clampCursor clamps at each level.
func TestArtistsViewClampCursor(t *testing.T) {
	t.Run("empty artist list", func(t *testing.T) {
		v := ArtistsView{cursor: 5, scrollOffset: 3}
		v = v.clampCursor()
		if v.cursor != 0 || v.scrollOffset != 0 {
			t.Errorf("expected cursor=0 scrollOffset=0, got cursor=%d scrollOffset=%d", v.cursor, v.scrollOffset)
		}
	})

	t.Run("cursor beyond list end", func(t *testing.T) {
		v := ArtistsView{artists: []string{"Alice", "Bob"}, cursor: 5}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1, got %d", v.cursor)
		}
	})

	t.Run("cursor within bounds is unchanged", func(t *testing.T) {
		v := ArtistsView{artists: []string{"Alice", "Bob", "Charlie"}, cursor: 1}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1 (unchanged), got %d", v.cursor)
		}
	})

	t.Run("album level", func(t *testing.T) {
		v := ArtistsView{level: arttui.AlbumLevel, albums: []string{"Alpha"}, cursor: 3}
		v = v.clampCursor()
		if v.cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.cursor)
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := ArtistsView{level: arttui.TrackLevel, tracks: []library.Track{{Title: "T1"}}, cursor: 5}
		v = v.clampCursor()
		if v.cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.cursor)
		}
	})
}

// TestArtistsViewEnqueueFocusedTrack verifies the enqueue helper.
func TestArtistsViewEnqueueFocusedTrack(t *testing.T) {
	v := ArtistsView{level: arttui.TrackLevel, tracks: []library.Track{{Title: "Song A", Hash: "h1"}}, cursor: 0}
	qa := &stubQueueAppender{}
	cmd := v.enqueueFocusedTrack(qa, 0)
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
	v := ArtistsView{level: arttui.ArtistLevel, artists: []string{"Alice"}, cursor: 0}
	qa := &stubQueueAppender{}
	cmd := v.enqueueFocusedTrack(qa, 0)
	if cmd != nil {
		t.Fatal("expected nil cmd when not at TrackLevel")
	}
}

// TestArtistsDrillDownPreservesWidthHeight verifies that openAlbums and
// openTracks do not reset width/height.
func TestArtistsDrillDownPreservesWidthHeight(t *testing.T) {
	v := ArtistsView{level: arttui.ArtistLevel, artists: []string{"Alice"}, width: 120, height: 40}

	v = v.openAlbums([]string{"Alpha"})
	if v.width != 120 || v.height != 40 {
		t.Errorf("openAlbums changed width/height: %d/%d", v.width, v.height)
	}

	v = v.openTracks([]library.Track{{Title: "T1"}})
	if v.width != 120 || v.height != 40 {
		t.Errorf("openTracks changed width/height: %d/%d", v.width, v.height)
	}
}

// TestArtistsClampCursor_WithFilter verifies that clampCursor accounts for
// the active filter rather than the raw list length.
func TestArtistsClampCursor_WithFilter(t *testing.T) {
	m := newModelWithTracks()
	m.browse.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	m.browse.artists.level = arttui.ArtistLevel
	m.browse.artists.filter.Text = "ali"
	m.browse.artists.cursor = 2
	m.browse.artists.scrollOffset = 2

	m.browse.artists = m.browse.artists.clampCursor()

	if m.browse.artists.cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered match), got %d", m.browse.artists.cursor)
	}
	if m.browse.artists.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.browse.artists.scrollOffset)
	}
}

// TestArtistsClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down when it ends up above the cursor.
func TestArtistsClampCursor_ScrollOffsetClamped(t *testing.T) {
	m := newModelWithTracks()
	m.browse.artists.artists = []string{"Alice"}
	m.browse.artists.level = arttui.ArtistLevel
	m.browse.artists.cursor = 5
	m.browse.artists.scrollOffset = 10

	m.browse.artists = m.browse.artists.clampCursor()

	if m.browse.artists.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.browse.artists.cursor)
	}
	if m.browse.artists.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.browse.artists.scrollOffset)
	}
}

// TestArtistsClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestArtistsClampCursor_TrackLevelWithFilter(t *testing.T) {
	m := newModelWithTracks()
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.tracks = []library.Track{
		{Hash: "h1", Title: "Foo", Artist: "A"},
		{Hash: "h2", Title: "Bar", Artist: "B"},
		{Hash: "h3", Title: "Baz", Artist: "C"},
	}
	m.browse.artists.filter.Text = "ba"
	m.browse.artists.cursor = 3
	m.browse.artists.scrollOffset = 3

	m.browse.artists = m.browse.artists.clampCursor()

	if m.browse.artists.cursor != 1 {
		t.Errorf("expected cursor=1 (last filtered index), got %d", m.browse.artists.cursor)
	}
	if m.browse.artists.scrollOffset > m.browse.artists.cursor {
		t.Errorf("scrollOffset %d should not be above cursor %d", m.browse.artists.scrollOffset, m.browse.artists.cursor)
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

	v := ArtistsView{level: arttui.ArtistLevel, cursor: 0, artists: artists, height: 12}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.cursor)
	}
	scrollNoSort := v.scrollOffset

	v2 := ArtistsView{
		level:     arttui.ArtistLevel,
		cursor:    0,
		artists:   artists,
		height:    12,
		sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v2 = v2.HandleAction(actionGoToBottom)
	if v2.cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.cursor)
	}
	if v2.scrollOffset != scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset = %d (strip absorbs sort), got %d", scrollNoSort, v2.scrollOffset)
	}
}
