package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestInsertStringAt verifies insertStringAt for various insertion positions.
func TestInsertStringAt(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		idx   int
		s     string
		want  []string
	}{
		{"insert at start", []string{"b", "c"}, 0, "a", []string{"a", "b", "c"}},
		{"insert in middle", []string{"a", "c"}, 1, "b", []string{"a", "b", "c"}},
		{"insert at end", []string{"a", "b"}, 2, "c", []string{"a", "b", "c"}},
		{"clamp negative idx", []string{"a", "b"}, -1, "z", []string{"z", "a", "b"}},
		{"clamp idx beyond len", []string{"a", "b"}, 10, "z", []string{"a", "b", "z"}},
		{"empty slice", nil, 0, "x", []string{"x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertStringAt(tc.items, tc.idx, tc.s)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestInsertTrackAt verifies insertTrackAt for various insertion positions.
func TestInsertTrackAt(t *testing.T) {
	a := library.Track{Hash: "aaa", Title: "A"}
	b := library.Track{Hash: "bbb", Title: "B"}
	c := library.Track{Hash: "ccc", Title: "C"}

	tests := []struct {
		name   string
		tracks []library.Track
		idx    int
		t      library.Track
		want   []library.Track
	}{
		{"insert at start", []library.Track{b, c}, 0, a, []library.Track{a, b, c}},
		{"insert in middle", []library.Track{a, c}, 1, b, []library.Track{a, b, c}},
		{"insert at end", []library.Track{a, b}, 2, c, []library.Track{a, b, c}},
		{"clamp negative idx", []library.Track{a, b}, -1, c, []library.Track{c, a, b}},
		{"clamp idx beyond len", []library.Track{a, b}, 10, c, []library.Track{a, b, c}},
		{"empty slice", nil, 0, a, []library.Track{a}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertTrackAt(tc.tracks, tc.idx, tc.t)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i].Hash != tc.want[i].Hash {
					t.Fatalf("index %d: got hash %q, want %q", i, got[i].Hash, tc.want[i].Hash)
				}
			}
		})
	}
}

// TestUndoRemoveFavorite_SingleFromFavoritesView verifies that undoing a single
// artist removal from the Favorites view re-inserts the artist at its original
// index and calls AddFavorite on the writer.
func TestUndoRemoveFavorite_SingleFromFavoritesView(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach", "Coltrane", "Davis"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	// Simulate state after "Bach" (index 0) was removed.
	m.favorites.artists = []string{"Coltrane", "Davis"}
	m.favorites.artistCount = 2

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:      "artist",
			value:    "Bach",
			idx:      0,
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.artists) != 3 {
		t.Fatalf("expected 3 artists after undo, got %d: %v", len(nm.favorites.artists), nm.favorites.artists)
	}
	if nm.favorites.artists[0] != "Bach" {
		t.Fatalf("expected 'Bach' at index 0, got %q", nm.favorites.artists[0])
	}
	if len(w.added) != 1 || w.added[0].typ != "artist" || w.added[0].value != "Bach" {
		t.Fatalf("expected AddFavorite(artist, Bach), got %v", w.added)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "artist") {
		t.Errorf("expected notification mentioning 'artist', got %q", nm.statusBar.notification.msg)
	}
}

// TestUndoRemoveFavorite_TrackHashRestored verifies that undoing a track
// removal restores the hash in the in-memory favoriteHashes set.
func TestUndoRemoveFavorite_TrackHashRestored(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"hash-1", "hash-2"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	// Simulate that hash-1 was removed from the in-memory set.
	m.favoriteHashes = map[string]struct{}{"hash-2": {}}

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "track",
			value: "hash-1",
			idx:   -1, // not from Favorites view
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if _, ok := nm.favoriteHashes["hash-1"]; !ok {
		t.Fatal("expected hash-1 to be restored in favoriteHashes")
	}
	if len(w.added) != 1 || w.added[0].value != "hash-1" {
		t.Fatalf("expected AddFavorite called with hash-1, got %v", w.added)
	}
}

// TestUndoRemoveFavorite_FromNonFavoritesView verifies that undo from a
// non-Favorites view (idx == -1) still calls AddFavorite on the DB
// but does not modify any in-view lists.
func TestUndoRemoveFavorite_FromNonFavoritesView(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.activeView = viewLibrary // not the Favorites view

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "artist",
			value: "Miles Davis",
			idx:   -1,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 1 || w.added[0].value != "Miles Davis" {
		t.Fatalf("expected AddFavorite(artist, Miles Davis), got %v", w.added)
	}
	// No in-view list manipulation should have happened.
	if len(nm.favorites.artists) != 0 {
		t.Fatalf("expected no artists to be modified, got %v", nm.favorites.artists)
	}
}

// TestUndoBulkRemoveFavorites verifies that undoing a bulk removal of 3 tracks
// (at original indices 1, 2, 4) re-inserts them at their correct positions.
func TestUndoBulkRemoveFavorites(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h0", "h3"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks

	// After removing 3 tracks (original indices 1, 2, 4), only h0 and h3 remain.
	t0 := library.Track{Hash: "h0", Title: "T0"}
	t3 := library.Track{Hash: "h3", Title: "T3"}
	m.favorites.tracks = []library.Track{t0, t3}
	m.favorites.trackCount = 2

	t1 := library.Track{Hash: "h1", Title: "T1"}
	t2 := library.Track{Hash: "h2", Title: "T2"}
	t4 := library.Track{Hash: "h4", Title: "T4"}

	m.undo.slot = undoSlot{
		action: undoBulkRemoveFavoritesAction{
			typ:      "track",
			values:   []string{"h1", "h2", "h4"},
			tracks:   []library.Track{t1, t2, t4},
			indices:  []int{1, 2, 4},
			level:    favtui.ListLevel,
			category: favtui.CategoryTracks,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.tracks) != 5 {
		t.Fatalf("expected 5 tracks after undo, got %d: %v", len(nm.favorites.tracks), nm.favorites.tracks)
	}
	// Verify each position.
	expected := []string{"h0", "h1", "h2", "h3", "h4"}
	for i, h := range expected {
		if nm.favorites.tracks[i].Hash != h {
			t.Errorf("index %d: got hash %q, want %q", i, nm.favorites.tracks[i].Hash, h)
		}
	}
	if len(w.added) != 3 {
		t.Fatalf("expected 3 AddFavorite calls, got %d: %v", len(w.added), w.added)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}

// TestUndoRemoveFavorite_Expired verifies that UndoMsg after the window has
// expired shows "Nothing to undo" and does not call AddFavorite.
func TestUndoRemoveFavorite_Expired(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Coltrane"}

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "artist",
			value: "Bach",
			idx:   0,
		},
		expiresAt: time.Now().Add(-1 * time.Second), // already expired
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 0 {
		t.Fatalf("AddFavorite must not be called after expiry, got %v", w.added)
	}
	if len(nm.favorites.artists) != 1 {
		t.Fatalf("artists list must be unchanged, got %v", nm.favorites.artists)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Nothing to undo") {
		t.Errorf("expected 'Nothing to undo' notification, got %q", nm.statusBar.notification.msg)
	}
}

// TestBulkFavoriteRemoval_UpdatesInViewState verifies that bulk removal of
// favorites from the Favorites view correctly removes all items from the
// in-memory view lists, not just the single msg.value (which is empty for
// bulk operations).
func TestBulkFavoriteRemoval_UpdatesInViewState(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach", "Davis"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Bach", "Coltrane", "Davis", "Ellington"}
	m.favorites.artistCount = 4

	// Simulate a bulk removal result: msg.value is empty, msg.removedValues
	// has the actual values.
	msg := favoriteToggledMsg{
		typ:           "artist",
		value:         "",
		count:         2,
		added:         false,
		removedValues: []string{"Bach", "Davis"},
		favLevel:      favtui.ListLevel,
		favCategory:   favtui.CategoryArtists,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.artists) != 2 {
		t.Fatalf("expected 2 artists after bulk removal, got %d: %v", len(nm.favorites.artists), nm.favorites.artists)
	}
	if nm.favorites.artists[0] != "Coltrane" || nm.favorites.artists[1] != "Ellington" {
		t.Fatalf("expected [Coltrane, Ellington], got %v", nm.favorites.artists)
	}
}

// TestBulkFavoriteRemoval_TracksAtTrackLevel verifies that bulk removal of
// track favorites from the TrackLevel correctly removes tracks from
// drilldownTracks.
func TestBulkFavoriteRemoval_TracksAtTrackLevel(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h1", "h4"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel

	t1 := library.Track{Hash: "h1", Title: "T1"}
	t2 := library.Track{Hash: "h2", Title: "T2"}
	t3 := library.Track{Hash: "h3", Title: "T3"}
	t4 := library.Track{Hash: "h4", Title: "T4"}
	m.favorites.drilldownTracks = []library.Track{t1, t2, t3, t4}

	msg := favoriteToggledMsg{
		typ:           "track",
		count:         2,
		added:         false,
		removedValues: []string{"h1", "h3"},
		favLevel:      favtui.TrackLevel,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.drilldownTracks) != 2 {
		t.Fatalf("expected 2 tracks after bulk removal, got %d", len(nm.favorites.drilldownTracks))
	}
	if nm.favorites.drilldownTracks[0].Hash != "h2" || nm.favorites.drilldownTracks[1].Hash != "h4" {
		t.Fatalf("expected [h2, h4], got [%s, %s]",
			nm.favorites.drilldownTracks[0].Hash, nm.favorites.drilldownTracks[1].Hash)
	}
}

// TestSingleFavoriteRemoval_StillWorksWithValue verifies that single removal
// (msg.value populated, msg.removedValues empty) still updates the in-view state.
func TestSingleFavoriteRemoval_StillWorksWithValue(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"album": {"Jazz", "Rock"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Jazz", "Rock", "Blues"}
	m.favorites.albumCount = 3

	msg := favoriteToggledMsg{
		typ:         "album",
		value:       "Rock",
		count:       1,
		added:       false,
		favLevel:    favtui.ListLevel,
		favCategory: favtui.CategoryAlbums,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.albums) != 2 {
		t.Fatalf("expected 2 albums after single removal, got %d: %v", len(nm.favorites.albums), nm.favorites.albums)
	}
	if nm.favorites.albums[0] != "Jazz" || nm.favorites.albums[1] != "Blues" {
		t.Fatalf("expected [Jazz, Blues], got %v", nm.favorites.albums)
	}
}

// TestUndoRemoveFavorite_Superseded verifies that performing two consecutive
// removals only allows undoing the second (most recent) one.
func TestUndoRemoveFavorite_Superseded(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"artist": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists

	// Set up state: two artists were removed; only second slot remains.
	m.favorites.artists = []string{}
	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:      "artist",
			value:    "Coltrane", // second removal (supersedes first)
			idx:      0,
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// Only "Coltrane" should have been restored.
	if len(w.added) != 1 {
		t.Fatalf("expected exactly 1 AddFavorite call, got %d: %v", len(w.added), w.added)
	}
	if w.added[0].value != "Coltrane" {
		t.Fatalf("expected AddFavorite for 'Coltrane', got %q", w.added[0].value)
	}
	// Slot must be cleared — "Bach" cannot be undone.
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}

// TestFavoritesClampCursor_WithActiveFilter verifies that clampCursor accounts
// for the active filter rather than the raw backing-store length. Without the
// fix, a cursor at index 2 in a filtered list of 2 items goes out of bounds.
func TestFavoritesClampCursor_WithActiveFilter(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alice", "Bob", "Charlie", "David"}
	m.favorites.filter.Text = "a" // matches "Alice", "Charlie", "David" = 3 items
	m.favorites.cursor = 3        // beyond filtered list bounds

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 2 {
		t.Errorf("expected cursor=2 (last filtered index), got %d", m.favorites.cursor)
	}
}

// TestFavoritesClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down to cursor when scrollOffset > cursor after clamping.
func TestFavoritesClampCursor_ScrollOffsetClamped(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Alpha"}
	m.favorites.cursor = 5        // beyond list
	m.favorites.scrollOffset = 10 // stale high offset

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.favorites.cursor)
	}
	if m.favorites.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.scrollOffset)
	}
}

// TestFavoritesClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestFavoritesClampCursor_TrackLevelWithFilter(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Title: "Alpha", Artist: "A"},
		{Hash: "h2", Title: "Beta", Artist: "B"},
		{Hash: "h3", Title: "Gamma", Artist: "G"},
	}
	m.favorites.filter.Text = "beta" // matches only "Beta" = 1 item
	m.favorites.cursor = 2           // beyond filtered bounds
	m.favorites.scrollOffset = 2

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered item), got %d", m.favorites.cursor)
	}
	if m.favorites.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.scrollOffset)
	}
}

// TestFavoritesClampCursor_EmptyFilteredList verifies clampCursor resets both
// cursor and scrollOffset when the filter eliminates all items.
func TestFavoritesClampCursor_EmptyFilteredList(t *testing.T) {
	m := New(context.Background())
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
	}
	m.favorites.filter.Text = "zzz" // matches nothing
	m.favorites.cursor = 1
	m.favorites.scrollOffset = 1

	m.favorites = m.favorites.clampCursor()

	if m.favorites.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.favorites.cursor)
	}
	if m.favorites.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.favorites.scrollOffset)
	}
}

// TestSortBulkRemovalByIndex verifies that sortBulkRemovalByIndex sorts
// parallel arrays by the index values in ascending order.
func TestSortBulkRemovalByIndex(t *testing.T) {
	t.Run("sorts values and indices", func(t *testing.T) {
		values := []string{"c", "a", "b"}
		indices := []int{5, 1, 3}
		sortBulkRemovalByIndex(values, nil, indices)
		wantValues := []string{"a", "b", "c"}
		wantIndices := []int{1, 3, 5}
		for i := range values {
			if values[i] != wantValues[i] {
				t.Errorf("values[%d] = %q, want %q", i, values[i], wantValues[i])
			}
			if indices[i] != wantIndices[i] {
				t.Errorf("indices[%d] = %d, want %d", i, indices[i], wantIndices[i])
			}
		}
	})

	t.Run("sorts tracks in parallel", func(t *testing.T) {
		values := []string{"h3", "h1", "h2"}
		tracks := []library.Track{
			{Hash: "h3", Title: "C"},
			{Hash: "h1", Title: "A"},
			{Hash: "h2", Title: "B"},
		}
		indices := []int{5, 1, 3}
		sortBulkRemovalByIndex(values, tracks, indices)
		wantHashes := []string{"h1", "h2", "h3"}
		for i := range tracks {
			if tracks[i].Hash != wantHashes[i] {
				t.Errorf("tracks[%d].Hash = %q, want %q", i, tracks[i].Hash, wantHashes[i])
			}
		}
	})

	t.Run("single element is a no-op", func(t *testing.T) {
		values := []string{"a"}
		indices := []int{5}
		sortBulkRemovalByIndex(values, nil, indices)
		if values[0] != "a" || indices[0] != 5 {
			t.Errorf("unexpected mutation of single-element slices")
		}
	})

	t.Run("already sorted", func(t *testing.T) {
		values := []string{"a", "b", "c"}
		indices := []int{1, 3, 5}
		sortBulkRemovalByIndex(values, nil, indices)
		for i, want := range []string{"a", "b", "c"} {
			if values[i] != want {
				t.Errorf("values[%d] = %q, want %q", i, values[i], want)
			}
		}
	})
}

// TestBulkFavoritesUndoReinsertionOrder verifies that bulk undo correctly
// re-inserts items at their original positions when indices are sorted.
func TestBulkFavoritesUndoReinsertionOrder(t *testing.T) {
	// Start with artists [A, B, C, D, E].
	// Remove B (idx 1) and D (idx 3).
	// After removal: [A, C, E].
	// Undo should restore to [A, B, C, D, E].
	m := Model{
		appCtx:     context.Background(),
		activeView: viewFavorites,
		favorites: FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
			artists:  []string{"A", "C", "E"},
		},
	}

	action := undoBulkRemoveFavoritesAction{
		typ:      "artist",
		values:   []string{"B", "D"}, // sorted by sortBulkRemovalByIndex
		indices:  []int{1, 3},        // original positions, ascending
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
	}
	m, _ = action.perform(m)

	want := []string{"A", "B", "C", "D", "E"}
	if len(m.favorites.artists) != len(want) {
		t.Fatalf("got %d artists, want %d", len(m.favorites.artists), len(want))
	}
	for i, w := range want {
		if m.favorites.artists[i] != w {
			t.Errorf("artists[%d] = %q, want %q", i, m.favorites.artists[i], w)
		}
	}
}

// TestBulkFavoritesUndoReinsertionOrder_Unsorted verifies that without sorting,
// the undo produces incorrect results (validating the fix is necessary).
func TestBulkFavoritesUndoReinsertionOrder_Unsorted(t *testing.T) {
	// Simulate what would happen if indices were in reverse order
	// (as might come from map iteration): [3, 1]
	// Start: [A, C, E] (after removing B at 1, D at 3)
	// Insert D at min(3, 3)=3 -> [A, C, E, D]
	// Insert B at min(1, 4)=1 -> [A, B, C, E, D] (wrong! D is at end)
	m := Model{
		appCtx:     context.Background(),
		activeView: viewFavorites,
		favorites: FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
			artists:  []string{"A", "C", "E"},
		},
	}

	// Reverse order (simulating bad map iteration).
	action := undoBulkRemoveFavoritesAction{
		typ:      "artist",
		values:   []string{"D", "B"},
		indices:  []int{3, 1},
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
	}
	m, _ = action.perform(m)

	// With unsorted indices, the result is NOT [A, B, C, D, E].
	// This test documents that the undo algorithm requires sorted indices.
	correct := []string{"A", "B", "C", "D", "E"}
	isCorrect := len(m.favorites.artists) == len(correct)
	if isCorrect {
		for i, w := range correct {
			if m.favorites.artists[i] != w {
				isCorrect = false
				break
			}
		}
	}
	if isCorrect {
		t.Error("expected incorrect result with unsorted indices, but got correct result")
	}
}

// TestFavoritesLoadedMsg_SavesCategoryCursor verifies that transitioning from
// CategoryLevel to ListLevel via handleFavoritesLoadedMsg saves the category
// cursor so that back() restores it correctly. Without the fix, categoryCursor
// remained 0 and back() always returned to the first category regardless of
// which one was opened.
func TestFavoritesLoadedMsg_SavesCategoryCursor(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.CategoryLevel
	// Position cursor on Albums (index 1).
	m.favorites.cursor = 1

	msg := favoritesLoadedMsg{
		category: int(favtui.CategoryAlbums),
		albums:   []string{"Jazz", "Rock"},
	}
	m = m.handleFavoritesLoadedMsg(msg)

	if m.favorites.level != favtui.ListLevel {
		t.Fatalf("expected ListLevel, got %d", m.favorites.level)
	}
	if m.favorites.categoryCursor != 1 {
		t.Fatalf("expected categoryCursor=1 (Albums), got %d", m.favorites.categoryCursor)
	}

	// Now go back and verify the cursor is restored to the Albums category.
	m.favorites = m.favorites.back()

	if m.favorites.level != favtui.CategoryLevel {
		t.Fatalf("expected CategoryLevel after back(), got %d", m.favorites.level)
	}
	if m.favorites.cursor != 1 {
		t.Fatalf("expected cursor=1 (Albums) after back(), got %d", m.favorites.cursor)
	}
}

// TestFavoritesLoadedMsg_SavesCategoryCursor_Tracks verifies the fix for
// cursor=2 (Tracks category).
func TestFavoritesLoadedMsg_SavesCategoryCursor_Tracks(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.CategoryLevel
	m.favorites.cursor = 2 // Tracks

	msg := favoritesLoadedMsg{
		category: int(favtui.CategoryTracks),
		tracks:   []library.Track{{Hash: "h1", Title: "Song"}},
	}
	m = m.handleFavoritesLoadedMsg(msg)

	if m.favorites.categoryCursor != 2 {
		t.Fatalf("expected categoryCursor=2 (Tracks), got %d", m.favorites.categoryCursor)
	}

	m.favorites = m.favorites.back()
	if m.favorites.cursor != 2 {
		t.Fatalf("expected cursor=2 after back(), got %d", m.favorites.cursor)
	}
}

// TestFavoritesGoToBottomWithSortAccountsForReserve verifies that GoToBottom
// calculates the correct scroll offset when a sort is active. The sort
// indicator takes one extra row, so visibleRows is one fewer.
func TestFavoritesGoToBottomWithSortAccountsForReserve(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist_%02d", i)
	}

	// Without sort: Height=12, ListLevel with CategoryArtists no filter/sort ->
	// reserve=0, visible=12.
	v := FavoritesView{
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
		cursor:   0,
		artists:  artists,
		height:   12,
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.cursor)
	}
	scrollNoSort := v.scrollOffset

	// With sort: reserve=1 (sort indicator), visible=11.
	v2 := FavoritesView{
		level:     favtui.ListLevel,
		category:  favtui.CategoryArtists,
		cursor:    0,
		artists:   artists,
		height:    12,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	v2 = v2.HandleAction(actionGoToBottom)
	if v2.cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.cursor)
	}
	if v2.scrollOffset <= scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset > %d (no-sort), got %d",
			scrollNoSort, v2.scrollOffset)
	}
}
