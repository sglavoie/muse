package tui

import (
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestAlbumsViewWrapperHandleAction verifies that the AlbumsView HandleAction
// properly delegates cursor navigation.
func TestAlbumsViewWrapperHandleAction(t *testing.T) {
	v := browse.AlbumsView{
		Albums: []string{"Alpha", "Beta", "Delta", "Gamma"},
		Cursor: 0,
		Height: 20,
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
	if v.Cursor != 3 {
		t.Errorf("GoToBottom: expected 3, got %d", v.Cursor)
	}

	v = v.HandleAction(keys.ActionGoToTop)
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.Cursor)
	}
}

// TestAlbumsViewFocusedMethods verifies focusedAlbum and focusedTrack return
// the correct values and false when out of bounds or at the wrong level.
func TestAlbumsViewFocusedMethods(t *testing.T) {
	v := browse.AlbumsView{
		Level:  albtui.AlbumLevel,
		Albums: []string{"Alpha", "Beta"},
		Cursor: 1,
	}
	album, ok := v.FocusedAlbum()
	if !ok || album != "Beta" {
		t.Errorf("focusedAlbum: expected (Beta, true), got (%q, %v)", album, ok)
	}

	// Out of bounds.
	v.Cursor = 5
	_, ok = v.FocusedAlbum()
	if ok {
		t.Error("focusedAlbum: expected false when cursor out of bounds")
	}

	// Wrong level.
	v.Level = albtui.TrackLevel
	v.Cursor = 0
	_, ok = v.FocusedAlbum()
	if ok {
		t.Error("focusedAlbum: expected false at TrackLevel")
	}

	// Track.
	v.Level = albtui.TrackLevel
	v.Tracks = []library.Track{{Title: "Song A"}}
	v.Cursor = 0
	track, ok := v.FocusedTrack()
	if !ok || track.Title != "Song A" {
		t.Errorf("focusedTrack: expected (Song A, true), got (%q, %v)", track.Title, ok)
	}

	// Wrong level for track.
	v.Level = albtui.AlbumLevel
	_, ok = v.FocusedTrack()
	if ok {
		t.Error("focusedTrack: expected false at AlbumLevel")
	}
}

// TestAlbumsViewClampCursor verifies clampCursor clamps at each level.
func TestAlbumsViewClampCursor(t *testing.T) {
	t.Run("empty album list", func(t *testing.T) {
		v := browse.AlbumsView{Cursor: 5, ScrollOffset: 3}
		v = v.ClampCursor()
		if v.Cursor != 0 || v.ScrollOffset != 0 {
			t.Errorf("expected cursor=0 scrollOffset=0, got cursor=%d scrollOffset=%d", v.Cursor, v.ScrollOffset)
		}
	})

	t.Run("cursor beyond list end", func(t *testing.T) {
		v := browse.AlbumsView{
			Albums: []string{"Alpha", "Beta"},
			Cursor: 5,
		}
		v = v.ClampCursor()
		if v.Cursor != 1 {
			t.Errorf("expected cursor=1, got %d", v.Cursor)
		}
	})

	t.Run("cursor within bounds is unchanged", func(t *testing.T) {
		v := browse.AlbumsView{
			Albums: []string{"Alpha", "Beta", "Gamma"},
			Cursor: 1,
		}
		v = v.ClampCursor()
		if v.Cursor != 1 {
			t.Errorf("expected cursor=1 (unchanged), got %d", v.Cursor)
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := browse.AlbumsView{
			Level:  albtui.TrackLevel,
			Tracks: []library.Track{{Title: "T1"}},
			Cursor: 5,
		}
		v = v.ClampCursor()
		if v.Cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.Cursor)
		}
	})
}

// TestAlbumsViewEnqueueFocusedTrack verifies the enqueue helper.
func TestAlbumsViewEnqueueFocusedTrack(t *testing.T) {
	v := browse.AlbumsView{
		Level:  albtui.TrackLevel,
		Tracks: []library.Track{{Title: "Song A", Hash: "h1"}},
		Cursor: 0,
	}
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

// TestAlbumsViewEnqueueFocusedTrackNotAtTrackLevel returns nil.
func TestAlbumsViewEnqueueFocusedTrackNotAtTrackLevel(t *testing.T) {
	v := browse.AlbumsView{
		Level:  albtui.AlbumLevel,
		Albums: []string{"Alpha"},
		Cursor: 0,
	}
	qa := &stubQueueAppender{}
	cmd := v.EnqueueFocusedTrack(qa, 0)
	if cmd != nil {
		t.Fatal("expected nil cmd when not at TrackLevel")
	}
}

// TestAlbumsDrillDownPreservesWidthHeight verifies that openTracks does not
// reset width/height.
func TestAlbumsDrillDownPreservesWidthHeight(t *testing.T) {
	v := browse.AlbumsView{
		Level:  albtui.AlbumLevel,
		Albums: []string{"Alpha"},
		Width:  120,
		Height: 40,
	}

	v = v.OpenTracks([]library.Track{{Title: "T1"}})
	if v.Width != 120 || v.Height != 40 {
		t.Errorf("openTracks changed width/height: %d/%d", v.Width, v.Height)
	}
}

// TestAlbumsClampCursor_WithFilter verifies that clampCursor accounts for
// the active filter rather than the raw list length.
func TestAlbumsClampCursor_WithFilter(t *testing.T) {
	v := browse.AlbumsView{
		Level:  albtui.AlbumLevel,
		Albums: []string{"Alpha", "Beta", "Delta", "Gamma"},
	}
	v.Filter.Text = "al" // matches "Alpha" = 1 item
	v.Cursor = 3         // beyond filtered bounds
	v.ScrollOffset = 3

	v = v.ClampCursor()

	if v.Cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered match), got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", v.ScrollOffset)
	}
}

// TestAlbumsClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down when it ends up above the cursor.
func TestAlbumsClampCursor_ScrollOffsetClamped(t *testing.T) {
	v := browse.AlbumsView{
		Level:  albtui.AlbumLevel,
		Albums: []string{"Alpha"},
	}
	v.Cursor = 5
	v.ScrollOffset = 10

	v = v.ClampCursor()

	if v.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", v.ScrollOffset)
	}
}

// TestAlbumsClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestAlbumsClampCursor_TrackLevelWithFilter(t *testing.T) {
	v := browse.AlbumsView{
		Level: albtui.TrackLevel,
		Tracks: []library.Track{
			{Hash: "h1", Title: "Foo", Artist: "A"},
			{Hash: "h2", Title: "Bar", Artist: "B"},
			{Hash: "h3", Title: "Baz", Artist: "C"},
		},
	}
	v.Filter.Text = "ba" // matches "Bar", "Baz" = 2 items
	v.Cursor = 3         // beyond filtered bounds
	v.ScrollOffset = 3

	v = v.ClampCursor()

	if v.Cursor != 1 {
		t.Errorf("expected cursor=1 (last filtered index), got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("scrollOffset %d should not be above cursor %d", v.ScrollOffset, v.Cursor)
	}
}

// TestAlbumsGoToBottomScrollOffsetIndependentOfSort verifies that GoToBottom
// produces the same scrollOffset regardless of sort state, because the state
// strip absorbs the sort chunk without reserving an extra row.
func TestAlbumsGoToBottomScrollOffsetIndependentOfSort(t *testing.T) {
	albums := make([]string, 20)
	for i := range albums {
		albums[i] = fmt.Sprintf("Album_%02d", i)
	}

	v := browse.AlbumsView{
		Level:  albtui.AlbumLevel,
		Cursor: 0,
		Albums: albums,
		Height: 12,
	}
	v = v.HandleAction(keys.ActionGoToBottom)
	if v.Cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.Cursor)
	}
	scrollNoSort := v.ScrollOffset

	v2 := browse.AlbumsView{
		Level:     albtui.AlbumLevel,
		Cursor:    0,
		Albums:    albums,
		Height:    12,
		SortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v2 = v2.HandleAction(keys.ActionGoToBottom)
	if v2.Cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset = %d (strip absorbs sort), got %d",
			scrollNoSort, v2.ScrollOffset)
	}
}
