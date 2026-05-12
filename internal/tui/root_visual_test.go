package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// makeNTracks returns n tracks with sequential hashes h0..h(n-1) in a stable
// order. Assigned directly to sourceTracks so no index-based sort reorders them.
func makeNTracks(n int) []library.Track {
	tracks := make([]library.Track, n)
	for i := 0; i < n; i++ {
		tracks[i] = library.Track{
			Hash:   fmt.Sprintf("h%d", i),
			Title:  fmt.Sprintf("Track %d", i),
			Artist: "Artist",
		}
	}
	return tracks
}

// newLibraryModel returns a model with n library tracks at known positions
// (h0 at index 0, h1 at index 1, …) and the library view active.
func newLibraryModel(n int) Model {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.library.sourceTracks = makeNTracks(n)
	return m
}

// doVisual calls handleNavigationAction(actionVisualMode) and returns the model.
func doVisual(m Model) Model {
	m, _, _ = m.handleNavigationAction(actionVisualMode)
	return m
}

// doNav calls handleNavigationAction(a) and returns the model.
func doNav(m Model, a action) Model {
	m, _, _ = m.handleNavigationAction(a)
	return m
}

// TestVisualModeEnterSelectsAnchor verifies that entering visual mode selects
// only the item at the cursor.
func TestVisualModeEnterSelectsAnchor(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 2

	m = doVisual(m)

	if !m.visual.active {
		t.Fatal("expected visual mode to be active")
	}
	if m.visual.anchor != 2 {
		t.Errorf("anchor: expected 2, got %d", m.visual.anchor)
	}
	if _, ok := m.library.selectedHashes["h2"]; !ok {
		t.Error("expected h2 to be selected at anchor")
	}
	if len(m.library.selectedHashes) != 1 {
		t.Errorf("expected 1 selected hash, got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModeExtendDownward verifies that j expands the visual range downward.
func TestVisualModeExtendDownward(t *testing.T) {
	m := newLibraryModel(6)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // cursor 1
	m = doNav(m, actionCursorDown) // cursor 2
	m = doNav(m, actionCursorDown) // cursor 3

	if m.library.cursor != 3 {
		t.Fatalf("expected cursor at 3, got %d", m.library.cursor)
	}
	for i := 0; i <= 3; i++ {
		h := fmt.Sprintf("h%d", i)
		if _, ok := m.library.selectedHashes[h]; !ok {
			t.Errorf("expected %s to be selected", h)
		}
	}
	if len(m.library.selectedHashes) != 4 {
		t.Errorf("expected 4 selected, got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModeRangeShrinks verifies that moving the cursor back up narrows
// the visual range.
func TestVisualModeRangeShrinks(t *testing.T) {
	m := newLibraryModel(6)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // cursor 1
	m = doNav(m, actionCursorDown) // cursor 2
	m = doNav(m, actionCursorDown) // cursor 3
	m = doNav(m, actionCursorUp)   // cursor 2

	for i := 0; i <= 2; i++ {
		h := fmt.Sprintf("h%d", i)
		if _, ok := m.library.selectedHashes[h]; !ok {
			t.Errorf("expected %s to be selected after shrink", h)
		}
	}
	if _, ok := m.library.selectedHashes["h3"]; ok {
		t.Error("h3 should not be selected after shrinking range")
	}
	if len(m.library.selectedHashes) != 3 {
		t.Errorf("expected 3 selected after shrink, got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModeExitPreservesSelections verifies that pressing v again exits
// visual mode while keeping the selections.
func TestVisualModeExitPreservesSelections(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // cursor 1
	m = doNav(m, actionCursorDown) // cursor 2 — range: h0..h2
	m = doVisual(m)                // exit

	if m.visual.active {
		t.Fatal("expected visual mode to be inactive after second v")
	}
	for i := 0; i <= 2; i++ {
		h := fmt.Sprintf("h%d", i)
		if _, ok := m.library.selectedHashes[h]; !ok {
			t.Errorf("expected %s to persist after visual exit", h)
		}
	}
	if len(m.library.selectedHashes) != 3 {
		t.Errorf("expected 3 selections after exit, got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModeClearSelectionExitsMode verifies that X clears all selections
// and exits visual mode.
func TestVisualModeClearSelectionExitsMode(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // range: h0..h1
	m, _, _ = m.handleNavigationAction(actionClearSelection)

	if m.visual.active {
		t.Fatal("expected visual mode to exit on X")
	}
	if len(m.library.selectedHashes) != 0 {
		t.Errorf("expected 0 selected after X, got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModeViewSwitchExits verifies that switching views exits visual
// mode without discarding selections made in the original view.
func TestVisualModeViewSwitchExits(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // range: h0..h1

	m = m.switchToView(viewQueue)

	if m.visual.active {
		t.Fatal("expected visual mode to exit on view switch")
	}
	if len(m.library.selectedHashes) != 2 {
		t.Errorf("expected 2 library selections preserved after switch, got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModePreexistingSelectionsPreserved verifies that items selected
// before entering visual mode and outside the range remain selected.
func TestVisualModePreexistingSelectionsPreserved(t *testing.T) {
	m := newLibraryModel(6)
	m.library.selectedHashes = map[string]struct{}{"h5": {}}
	m.library.cursor = 1

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // range: h1..h2

	if _, ok := m.library.selectedHashes["h5"]; !ok {
		t.Error("h5 (pre-existing) should be preserved during visual range")
	}
	if _, ok := m.library.selectedHashes["h1"]; !ok {
		t.Error("h1 should be in the visual range")
	}
	if _, ok := m.library.selectedHashes["h2"]; !ok {
		t.Error("h2 should be in the visual range")
	}
	if len(m.library.selectedHashes) != 3 {
		t.Errorf("expected 3 selected (h1+h2+h5), got %d", len(m.library.selectedHashes))
	}
}

// TestVisualModeQueueExtend verifies visual mode in the queue view selects the
// correct entries when moving the cursor downward.
func TestVisualModeQueueExtend(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // 4 entries, indices 0..3
	m.queue.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // cursor 1
	m = doNav(m, actionCursorDown) // cursor 2

	for i := 0; i <= 2; i++ {
		if !m.queue.entries[i].Selected {
			t.Errorf("queue entry %d should be selected", i)
		}
	}
	if m.queue.entries[3].Selected {
		t.Error("queue entry 3 should not be selected")
	}
}

// TestVisualModeQueueExitPreservesSelections verifies queue selections persist
// after exiting visual mode.
func TestVisualModeQueueExitPreservesSelections(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown) // range: entries 0..1
	m = doVisual(m)                // exit

	if m.visual.active {
		t.Fatal("expected visual mode inactive after exit")
	}
	for i := 0; i <= 1; i++ {
		if !m.queue.entries[i].Selected {
			t.Errorf("queue entry %d should remain selected after exit", i)
		}
	}
	if m.queue.entries[2].Selected || m.queue.entries[3].Selected {
		t.Error("queue entries 2 and 3 should not be selected")
	}
}

// TestVisualModeHierarchicalViewGuard verifies that pressing v at artist list
// level is a no-op (visual mode is only valid at track level).
func TestVisualModeHierarchicalViewGuard(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	// artists.level zero-value is ArtistLevel, not TrackLevel.

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at artist list level")
	}
}

// TestVisualModeAnchorClamp verifies that when the visible list shrinks the
// anchor is clamped so updateVisualSelection does not panic or over-select.
func TestVisualModeAnchorClamp(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 4

	m = doVisual(m) // anchor = 4

	if m.visual.anchor != 4 {
		t.Fatalf("expected anchor 4, got %d", m.visual.anchor)
	}

	// Shrink list to 3 items and clamp cursor.
	m.library.sourceTracks = makeNTracks(3)
	m.library.cursor = 2

	m = doNav(m, actionCursorUp) // triggers updateVisualSelection with anchor=4, len=3

	if m.visual.anchor > 2 {
		t.Errorf("anchor should be clamped to ≤2 after list shrink, got %d", m.visual.anchor)
	}
}

// TestVisualModeFilterActivationExits verifies that pressing / (activating the
// filter bar) exits visual mode because the visible list changes.
func TestVisualModeFilterActivationExits(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to be active before filter")
	}

	m, _ = m.handleFilterBarMsg()

	if m.visual.active {
		t.Fatal("expected visual mode to exit when filter bar is activated")
	}
}

// TestSortedIntSetKeysOrder verifies that sortedIntSetKeys returns keys in
// ascending order.
func TestSortedIntSetKeysOrder(t *testing.T) {
	s := map[int]struct{}{5: {}, 1: {}, 3: {}, 0: {}, 9: {}}
	keys := sortedIntSetKeys(s)
	want := []int{0, 1, 3, 5, 9}
	if len(keys) != len(want) {
		t.Fatalf("len = %d, want %d", len(keys), len(want))
	}
	for i, k := range keys {
		if k != want[i] {
			t.Errorf("keys[%d] = %d, want %d", i, k, want[i])
		}
	}
}

// TestSortedIntSetKeysEmpty verifies the helper handles an empty set.
func TestSortedIntSetKeysEmpty(t *testing.T) {
	keys := sortedIntSetKeys(nil)
	if len(keys) != 0 {
		t.Fatalf("expected empty slice, got %v", keys)
	}
}

// TestBulkFavoriteTracksReturnsDeterministicOrder verifies that
// bulkFavoriteTracks returns tracks in display order (ascending index),
// not in non-deterministic map iteration order.
func TestBulkFavoriteTracksReturnsDeterministicOrder(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
		{Hash: "h2", Title: "T2", Artist: "A"},
		{Hash: "h3", Title: "T3", Artist: "A"},
		{Hash: "h4", Title: "T4", Artist: "A"},
	}
	m.favorites.selectedItems = map[int]struct{}{4: {}, 1: {}, 3: {}}

	// Run many times to catch non-determinism.
	for i := 0; i < 50; i++ {
		tracks := m.bulkFavoriteTracks()
		if len(tracks) != 3 {
			t.Fatalf("iter %d: expected 3 tracks, got %d", i, len(tracks))
		}
		if tracks[0].Hash != "h1" || tracks[1].Hash != "h3" || tracks[2].Hash != "h4" {
			t.Fatalf("iter %d: wrong order: %s, %s, %s (want h1, h3, h4)",
				i, tracks[0].Hash, tracks[1].Hash, tracks[2].Hash)
		}
	}
}

// TestPlaylistBulkDeleteExitsVisualMode verifies that bulk-deleting tracks in
// the playlists view exits visual mode so the stale snapshot does not
// re-select removed items on the next cursor move.
func TestPlaylistBulkDeleteExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewPlaylists
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Title: "T1", Artist: "A", DurationMs: 1000},
		{ID: 2, Title: "T2", Artist: "A", DurationMs: 1000},
		{ID: 3, Title: "T3", Artist: "A", DurationMs: 1000},
	}
	m.playlists.cursor = 0

	// Enter visual mode and extend the selection to two tracks.
	m = doVisual(m)
	m = doNav(m, actionCursorDown) // visual range: 0..1
	if !m.visual.active {
		t.Fatal("expected visual mode active before delete")
	}

	// Manually select the tracks covered by visual range.
	m.playlists.selectedTrackIDs = map[int64]struct{}{1: {}, 2: {}}

	// Delete via the playlist delete handler (bulk path).
	m, _ = m.handlePlaylistDelete()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after playlist bulk delete")
	}
	if len(m.playlists.tracks) != 1 {
		t.Fatalf("expected 1 track remaining, got %d", len(m.playlists.tracks))
	}
}

// TestFavoritesBulkActionExitsVisualMode verifies that the favorites bulk
// remove action exits visual mode so the stale index-based snapshot does
// not re-select removed items on the next cursor move.
func TestFavoritesBulkActionExitsVisualMode(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h0", Title: "T0", Artist: "A"},
		{Hash: "h1", Title: "T1", Artist: "A"},
		{Hash: "h2", Title: "T2", Artist: "A"},
	}
	m.favorites.cursor = 0

	// Enter visual mode and extend selection.
	m = doVisual(m)
	m = doNav(m, actionCursorDown) // visual range: 0..1
	if !m.visual.active {
		t.Fatal("expected visual mode active before bulk action")
	}

	// Provide a no-op writer so handleFavoritesBulkAction does not panic.
	m.favoriteWriter = &stubFavoriteWriter{}

	m, _, _ = m.handleFavoritesBulkAction()

	if m.visual.active {
		t.Fatal("expected visual mode to exit after favorites bulk action")
	}
}
