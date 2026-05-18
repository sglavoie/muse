package tui

import (
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestAlbumsViewWrapperHandleAction verifies that the AlbumsView HandleAction
// properly delegates cursor navigation.
func TestAlbumsViewWrapperHandleAction(t *testing.T) {
	v := AlbumsView{
		albums: []string{"Alpha", "Beta", "Delta", "Gamma"},
		cursor: 0,
		height: 20,
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
	if v.cursor != 3 {
		t.Errorf("GoToBottom: expected 3, got %d", v.cursor)
	}

	v = v.HandleAction(actionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

// TestAlbumsViewFocusedMethods verifies focusedAlbum and focusedTrack return
// the correct values and false when out of bounds or at the wrong level.
func TestAlbumsViewFocusedMethods(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha", "Beta"},
		cursor: 1,
	}
	album, ok := v.focusedAlbum()
	if !ok || album != "Beta" {
		t.Errorf("focusedAlbum: expected (Beta, true), got (%q, %v)", album, ok)
	}

	// Out of bounds.
	v.cursor = 5
	_, ok = v.focusedAlbum()
	if ok {
		t.Error("focusedAlbum: expected false when cursor out of bounds")
	}

	// Wrong level.
	v.level = albtui.TrackLevel
	v.cursor = 0
	_, ok = v.focusedAlbum()
	if ok {
		t.Error("focusedAlbum: expected false at TrackLevel")
	}

	// Track.
	v.level = albtui.TrackLevel
	v.tracks = []library.Track{{Title: "Song A"}}
	v.cursor = 0
	track, ok := v.focusedTrack()
	if !ok || track.Title != "Song A" {
		t.Errorf("focusedTrack: expected (Song A, true), got (%q, %v)", track.Title, ok)
	}

	// Wrong level for track.
	v.level = albtui.AlbumLevel
	_, ok = v.focusedTrack()
	if ok {
		t.Error("focusedTrack: expected false at AlbumLevel")
	}
}

// TestAlbumsViewClampCursor verifies clampCursor clamps at each level.
func TestAlbumsViewClampCursor(t *testing.T) {
	t.Run("empty album list", func(t *testing.T) {
		v := AlbumsView{cursor: 5, scrollOffset: 3}
		v = v.clampCursor()
		if v.cursor != 0 || v.scrollOffset != 0 {
			t.Errorf("expected cursor=0 scrollOffset=0, got cursor=%d scrollOffset=%d", v.cursor, v.scrollOffset)
		}
	})

	t.Run("cursor beyond list end", func(t *testing.T) {
		v := AlbumsView{
			albums: []string{"Alpha", "Beta"},
			cursor: 5,
		}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1, got %d", v.cursor)
		}
	})

	t.Run("cursor within bounds is unchanged", func(t *testing.T) {
		v := AlbumsView{
			albums: []string{"Alpha", "Beta", "Gamma"},
			cursor: 1,
		}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1 (unchanged), got %d", v.cursor)
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := AlbumsView{
			level:  albtui.TrackLevel,
			tracks: []library.Track{{Title: "T1"}},
			cursor: 5,
		}
		v = v.clampCursor()
		if v.cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.cursor)
		}
	})
}

// TestAlbumsViewEnqueueFocusedTrack verifies the enqueue helper.
func TestAlbumsViewEnqueueFocusedTrack(t *testing.T) {
	v := AlbumsView{
		level:  albtui.TrackLevel,
		tracks: []library.Track{{Title: "Song A", Hash: "h1"}},
		cursor: 0,
	}
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

// TestAlbumsViewEnqueueFocusedTrackNotAtTrackLevel returns nil.
func TestAlbumsViewEnqueueFocusedTrackNotAtTrackLevel(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha"},
		cursor: 0,
	}
	qa := &stubQueueAppender{}
	cmd := v.enqueueFocusedTrack(qa, 0)
	if cmd != nil {
		t.Fatal("expected nil cmd when not at TrackLevel")
	}
}

// TestAlbumsDrillDownPreservesWidthHeight verifies that openTracks does not
// reset width/height.
func TestAlbumsDrillDownPreservesWidthHeight(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha"},
		width:  120,
		height: 40,
	}

	v = v.openTracks([]library.Track{{Title: "T1"}})
	if v.width != 120 || v.height != 40 {
		t.Errorf("openTracks changed width/height: %d/%d", v.width, v.height)
	}
}

// TestAlbumsClampCursor_WithFilter verifies that clampCursor accounts for
// the active filter rather than the raw list length.
func TestAlbumsClampCursor_WithFilter(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha", "Beta", "Delta", "Gamma"},
	}
	v.filter.Text = "al" // matches "Alpha" = 1 item
	v.cursor = 3         // beyond filtered bounds
	v.scrollOffset = 3

	v = v.clampCursor()

	if v.cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered match), got %d", v.cursor)
	}
	if v.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", v.scrollOffset)
	}
}

// TestAlbumsClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down when it ends up above the cursor.
func TestAlbumsClampCursor_ScrollOffsetClamped(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha"},
	}
	v.cursor = 5
	v.scrollOffset = 10

	v = v.clampCursor()

	if v.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", v.cursor)
	}
	if v.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", v.scrollOffset)
	}
}

// TestAlbumsClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestAlbumsClampCursor_TrackLevelWithFilter(t *testing.T) {
	v := AlbumsView{
		level: albtui.TrackLevel,
		tracks: []library.Track{
			{Hash: "h1", Title: "Foo", Artist: "A"},
			{Hash: "h2", Title: "Bar", Artist: "B"},
			{Hash: "h3", Title: "Baz", Artist: "C"},
		},
	}
	v.filter.Text = "ba" // matches "Bar", "Baz" = 2 items
	v.cursor = 3         // beyond filtered bounds
	v.scrollOffset = 3

	v = v.clampCursor()

	if v.cursor != 1 {
		t.Errorf("expected cursor=1 (last filtered index), got %d", v.cursor)
	}
	if v.scrollOffset > v.cursor {
		t.Errorf("scrollOffset %d should not be above cursor %d", v.scrollOffset, v.cursor)
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

	v := AlbumsView{
		level:  albtui.AlbumLevel,
		cursor: 0,
		albums: albums,
		height: 12,
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.cursor)
	}
	scrollNoSort := v.scrollOffset

	v2 := AlbumsView{
		level:     albtui.AlbumLevel,
		cursor:    0,
		albums:    albums,
		height:    12,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	v2 = v2.HandleAction(actionGoToBottom)
	if v2.cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.cursor)
	}
	if v2.scrollOffset != scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset = %d (strip absorbs sort), got %d",
			scrollNoSort, v2.scrollOffset)
	}
}
