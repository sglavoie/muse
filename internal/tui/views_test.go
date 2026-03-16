package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// makeQueue returns a QueueView pre-loaded with the given entries.
func makeQueue(width int, entries []QueueEntry) QueueView {
	return QueueView{
		entries: entries,
		width:   width,
	}
}

// TestQueueRenderEmptyShowsPlaceholder verifies the empty-queue message.
func TestQueueRenderEmptyShowsPlaceholder(t *testing.T) {
	v := QueueView{}
	got := v.render()
	if !strings.Contains(got, "empty") {
		t.Fatalf("empty queue must show '(empty)'; got: %q", got)
	}
}

// TestQueueRenderFourColumns checks that all four columns are present in each
// rendered row.
func TestQueueRenderFourColumns(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Song One", Artist: "Band A", DurationMs: 200_000, Position: 1000},
		{Title: "Song Two", Artist: "Band B", DurationMs: 65_000, Position: 2000},
	})
	out := v.render()

	// Position column: index numbers should appear.
	if !strings.Contains(out, "1") {
		t.Error("position column must contain entry index 1")
	}
	if !strings.Contains(out, "2") {
		t.Error("position column must contain entry index 2")
	}
	// Title column
	if !strings.Contains(out, "Song One") {
		t.Error("title column must contain 'Song One'")
	}
	if !strings.Contains(out, "Song Two") {
		t.Error("title column must contain 'Song Two'")
	}
	// Artist column
	if !strings.Contains(out, "Band A") {
		t.Error("artist column must contain 'Band A'")
	}
	if !strings.Contains(out, "Band B") {
		t.Error("artist column must contain 'Band B'")
	}
	// Duration column: 200 000 ms = 3:20, 65 000 ms = 1:05
	if !strings.Contains(out, "3:20") {
		t.Error("duration column must contain '3:20' for 200 000 ms")
	}
	if !strings.Contains(out, "1:05") {
		t.Error("duration column must contain '1:05' for 65 000 ms")
	}
}

// TestQueueRenderAscendingOrder verifies that entries appear in the order
// supplied to QueueView (callers are responsible for ascending position order;
// render preserves slice order).
func TestQueueRenderAscendingOrder(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "First", Artist: "A", DurationMs: 60_000, Position: 1000},
		{Title: "Second", Artist: "B", DurationMs: 120_000, Position: 2000},
		{Title: "Third", Artist: "C", DurationMs: 180_000, Position: 3000},
	})
	out := v.render()
	firstIdx := strings.Index(out, "First")
	secondIdx := strings.Index(out, "Second")
	thirdIdx := strings.Index(out, "Third")
	if firstIdx < 0 || secondIdx < 0 || thirdIdx < 0 {
		t.Fatal("all three titles must appear in the output")
	}
	if !(firstIdx < secondIdx && secondIdx < thirdIdx) {
		t.Errorf("entries must appear in ascending order: First(%d) Second(%d) Third(%d)",
			firstIdx, secondIdx, thirdIdx)
	}
}

// TestQueueRenderMissingEntryDim verifies that a missing entry is wrapped in
// ANSI dim codes and shows the "[missing]" label.
func TestQueueRenderMissingEntryDim(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Gone", Artist: "Nobody", DurationMs: 0, Missing: true, Position: 1000},
	})
	out := v.render()
	if !strings.Contains(out, ansiDim) {
		t.Error("missing entry must start with ANSI dim code")
	}
	if !strings.Contains(out, ansiReset) {
		t.Error("missing entry must end with ANSI reset code")
	}
	if !strings.Contains(out, "[missing]") {
		t.Error("missing entry must contain '[missing]' label")
	}
}

// TestQueueRenderProportionalColumnWidths verifies that the title and artist
// column widths change when the terminal width changes via WindowSizeMsg.
func TestQueueRenderProportionalColumnWidths(t *testing.T) {
	entry := QueueEntry{
		Title:      strings.Repeat("T", 60), // long enough to be truncated
		Artist:     strings.Repeat("A", 60),
		DurationMs: 90_000,
		Position:   1000,
	}

	narrow := makeQueue(80, []QueueEntry{entry})
	wide := makeQueue(160, []QueueEntry{entry})

	narrowOut := narrow.render()
	wideOut := wide.render()

	// Wide terminal should accommodate more of the long title / artist strings.
	// Count how many T's appear in the title column.
	narrowTs := strings.Count(narrowOut, "T")
	wideTs := strings.Count(wideOut, "T")
	if wideTs <= narrowTs {
		t.Errorf("wide terminal must show more title chars: narrow=%d, wide=%d", narrowTs, wideTs)
	}
}

// TestQueueRenderWindowSizeMsg verifies that sending a WindowSizeMsg to the
// root model and then rendering does not panic and produces non-empty content.
func TestQueueRenderWindowSizeMsg(t *testing.T) {
	m := New("", false, nil, nil, "", nil, nil)
	m.SetQueueEntries([]QueueEntry{
		{Title: "Track", Artist: "Artist", DurationMs: 30_000, Position: 1000},
	})
	// Switch to queue view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	// Send a resize event.
	resized, _ := nm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := resized.(Model)

	v := rm.View()
	if v.Content == "" {
		t.Fatal("View() after WindowSizeMsg must produce non-empty content")
	}
	if !strings.Contains(v.Content, "Track") {
		t.Error("queue view must contain the enqueued track title after resize")
	}
}

// TestLibraryViewToggleBulkSelectionToggles verifies that calling
// ToggleBulkSelection twice on the same hash leaves the set empty (toggle
// semantics), and that a single call adds the hash.
func TestLibraryViewToggleBulkSelectionToggles(t *testing.T) {
	v := LibraryView{}

	v = v.ToggleBulkSelection("abc")
	if _, ok := v.selectedHashes["abc"]; !ok {
		t.Fatal("expected hash to be selected after first toggle")
	}

	v = v.ToggleBulkSelection("abc")
	if _, ok := v.selectedHashes["abc"]; ok {
		t.Fatal("expected hash to be deselected after second toggle")
	}
	if len(v.selectedHashes) != 0 {
		t.Fatalf("expected empty selection set, got %v", v.selectedHashes)
	}
}

// TestLibraryViewClearSelectionEmptiesSet verifies that ClearSelection removes
// all bulk-selected hashes.
func TestLibraryViewClearSelectionEmptiesSet(t *testing.T) {
	v := LibraryView{}
	v = v.ToggleBulkSelection("a")
	v = v.ToggleBulkSelection("b")
	v = v.ClearSelection()
	if len(v.selectedHashes) != 0 {
		t.Fatalf("expected empty selection after ClearSelection, got %v", v.selectedHashes)
	}
}

// TestFormatQueueDuration covers the duration formatting helper.
// TestMarkMissingFlagsAllDuplicateEntries verifies that markMissing marks
// every entry with the matching path, not just the first one.
func TestMarkMissingFlagsAllDuplicateEntries(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/music/a.flac", Display: "A"},
			{Path: "/music/b.flac", Display: "B"},
			{Path: "/music/a.flac", Display: "A (dup)"},
		},
	}
	q.markMissing("/music/a.flac")

	if !q.entries[0].Missing {
		t.Error("first entry with matching path must be marked missing")
	}
	if q.entries[1].Missing {
		t.Error("entry with different path must not be marked missing")
	}
	if !q.entries[2].Missing {
		t.Error("second entry with matching path must be marked missing")
	}
}

func TestFormatQueueDuration(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "--:--"},
		{-1, "--:--"},
		{1_000, "0:01"},
		{60_000, "1:00"},
		{65_000, "1:05"},
		{3_600_000, "1:00:00"},
		{3_661_000, "1:01:01"},
		{5_999_000, "1:39:59"},
	}
	for _, tc := range cases {
		got := formatQueueDuration(tc.ms)
		if got != tc.want {
			t.Errorf("formatQueueDuration(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

// TestQueueRenderSelectedEntryShowsMarker verifies that a bulk-selected queue
// entry is rendered with a "●" selection marker, matching the Library and
// Playlists view behaviour.
func TestQueueRenderSelectedEntryShowsMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false},
		{Title: "Selected", Artist: "B", DurationMs: 120_000, Selected: true},
	})
	out := v.render()
	if !strings.Contains(out, "●") {
		t.Fatal("selected queue entry must show a ● marker")
	}
}

// TestQueueRenderUnselectedEntryHasNoMarker verifies that unselected queue
// entries do not contain a selection marker glyph.
func TestQueueRenderUnselectedEntryHasNoMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false},
	})
	out := v.render()
	if strings.Contains(out, "●") {
		t.Fatal("unselected queue entry must not show a ● marker")
	}
}

// TestPlaylistsViewEnsureCursorVisibleAdjustsScroll verifies that
// ensureCursorVisible moves scrollOffset when the cursor is beyond the
// visible window (both above and below).
func TestPlaylistsViewEnsureCursorVisibleAdjustsScroll(t *testing.T) {
	t.Run("cursor below visible window", func(t *testing.T) {
		v := PlaylistsView{
			cursor:       9,
			scrollOffset: 0,
			height:       6, // visibleRows = max(1, 6-3) = 3
		}
		v = v.ensureCursorVisible()
		// cursor=9, visibleRows=3 → scrollOffset should be 9-3+1 = 7
		if v.scrollOffset != 7 {
			t.Errorf("expected scrollOffset=7, got %d", v.scrollOffset)
		}
	})

	t.Run("cursor above visible window", func(t *testing.T) {
		v := PlaylistsView{
			cursor:       1,
			scrollOffset: 5,
			height:       6,
		}
		v = v.ensureCursorVisible()
		if v.scrollOffset != 1 {
			t.Errorf("expected scrollOffset=1, got %d", v.scrollOffset)
		}
	})

	t.Run("cursor already visible", func(t *testing.T) {
		v := PlaylistsView{
			cursor:       2,
			scrollOffset: 1,
			height:       6, // visibleRows=3, visible range [1,3]
		}
		v = v.ensureCursorVisible()
		if v.scrollOffset != 1 {
			t.Errorf("expected scrollOffset=1 (unchanged), got %d", v.scrollOffset)
		}
	})
}

// TestPlaylistsViewBackToListRestoresScrollOffset verifies that navigating
// from TrackLevel back to ListLevel restores the scroll offset the user had
// at list level, so the cursor remains visible without jumping to the top.
func TestPlaylistsViewBackToListRestoresScrollOffset(t *testing.T) {
	v := PlaylistsView{
		level:        pltui.ListLevel,
		cursor:       15,
		scrollOffset: 10,
		height:       10, // visibleRows = max(1, 10-3) = 7
		playlists:    make([]db.PlaylistRow, 20),
	}

	// Drill into tracks — openTracks saves list scroll position.
	v = v.openTracks([]db.PlaylistTrackRow{{Title: "T1"}, {Title: "T2"}})
	if v.scrollOffset != 0 {
		t.Fatalf("expected track-level scrollOffset=0, got %d", v.scrollOffset)
	}

	// Navigate back to the playlist list.
	v = v.backToList()
	if v.scrollOffset != 10 {
		t.Errorf("expected restored scrollOffset=10 after backToList, got %d", v.scrollOffset)
	}
	if v.cursor != 15 {
		t.Errorf("expected restored cursor=15 after backToList, got %d", v.cursor)
	}
}

// TestPlaylistsViewOpenTracksClearsStaleSelection verifies that opening a new
// playlist clears track-level bulk selections from a previously viewed playlist,
// preventing stale IDs from accidentally matching rows in the new playlist.
func TestPlaylistsViewOpenTracksClearsStaleSelection(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.TrackLevel,
		playlists: make([]db.PlaylistRow, 5),
		tracks:    []db.PlaylistTrackRow{{ID: 100, Title: "Old"}},
		selectedTrackIDs: map[int64]struct{}{
			100: {},
		},
	}

	// Simulate going back to list and opening a different playlist.
	v = v.backToList()
	v = v.openTracks([]db.PlaylistTrackRow{{ID: 200, Title: "New"}})

	if v.selectedTrackIDs != nil {
		t.Errorf("expected selectedTrackIDs to be nil after openTracks, got %v", v.selectedTrackIDs)
	}
}

// TestQueueRenderSelectedAndFocusedMarker verifies that a selected+focused
// entry uses a plain "●" glyph (without inner ANSI reset) so the outer
// reverse-video highlight covers the full row uniformly.
func TestQueueRenderSelectedAndFocusedMarker(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "Focused", Artist: "A", DurationMs: 60_000, Selected: true},
		},
		cursor: 0,
		width:  120,
	}
	out := v.render()
	// The focused+selected line must contain "●" but must not contain the
	// inner accent+reset pair that unfocused selected entries use.
	lines := strings.Split(out, "\n")
	var focusedLine string
	for _, l := range lines {
		if strings.Contains(l, "Focused") {
			focusedLine = l
			break
		}
	}
	if focusedLine == "" {
		t.Fatal("focused entry must appear in the output")
	}
	if !strings.Contains(focusedLine, "●") {
		t.Error("selected+focused entry must contain ● marker")
	}
}

// TestFocusedPlaylistReturnsFalseWhenCursorOutOfBounds verifies that
// focusedPlaylist returns false when the cursor exceeds the playlist slice
// length, preventing an index-out-of-bounds panic.
func TestFocusedPlaylistReturnsFalseWhenCursorOutOfBounds(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		playlists: []db.PlaylistRow{{ID: 1, Name: "A"}},
		cursor:    5, // out of bounds
	}
	_, ok := v.focusedPlaylist()
	if ok {
		t.Fatal("focusedPlaylist must return false when cursor exceeds slice length")
	}
}

// TestFocusedPlaylistIDReturnsFalseWhenCursorOutOfBounds verifies that
// focusedPlaylistID returns false when the cursor exceeds the playlist slice
// length, preventing an index-out-of-bounds panic.
func TestFocusedPlaylistIDReturnsFalseWhenCursorOutOfBounds(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		playlists: []db.PlaylistRow{{ID: 1, Name: "A"}},
		cursor:    5, // out of bounds
	}
	_, ok := v.focusedPlaylistID()
	if ok {
		t.Fatal("focusedPlaylistID must return false when cursor exceeds slice length")
	}
}

// TestFocusedPlaylistReturnsFalseWhenCursorNegative verifies that
// focusedPlaylist returns false when the cursor is negative.
func TestFocusedPlaylistReturnsFalseWhenCursorNegative(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		playlists: []db.PlaylistRow{{ID: 1, Name: "A"}},
		cursor:    -1,
	}
	_, ok := v.focusedPlaylist()
	if ok {
		t.Fatal("focusedPlaylist must return false when cursor is negative")
	}
}

// TestQueueSelectNoPanicWhenCursorOutOfBounds verifies that the actionSelect
// handler in QueueView does not panic when the cursor exceeds the entries slice.
func TestQueueSelectNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Display: "A"},
		},
		cursor: 5, // out of bounds
	}
	// Must not panic.
	_, cmd := q.HandleAction(actionSelect)
	if cmd != nil {
		t.Fatal("actionSelect with out-of-bounds cursor must return nil cmd")
	}
}

// TestQueueDequeueNoPanicWhenCursorOutOfBounds verifies that the actionDequeue
// handler in QueueView does not panic when the cursor exceeds the entries slice.
func TestQueueDequeueNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Display: "A"},
		},
		cursor: 5, // out of bounds
	}
	// Must not panic.
	_, cmd := q.HandleAction(actionDequeue)
	if cmd != nil {
		t.Fatal("actionDequeue with out-of-bounds cursor must return nil cmd")
	}
}

// --- QueueView navigation ---

func TestQueueHandleActionCursorUpDown(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 5),
		cursor:  0,
		width:   80,
	}
	q, _ = q.HandleAction(actionCursorDown)
	if q.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", q.cursor)
	}
	q, _ = q.HandleAction(actionCursorDown)
	if q.cursor != 2 {
		t.Errorf("CursorDown: expected 2, got %d", q.cursor)
	}
	q, _ = q.HandleAction(actionCursorUp)
	if q.cursor != 1 {
		t.Errorf("CursorUp: expected 1, got %d", q.cursor)
	}
}

func TestQueueHandleActionCursorClampsAtBounds(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 3),
		cursor:  0,
		width:   80,
	}
	q, _ = q.HandleAction(actionCursorUp)
	if q.cursor != 0 {
		t.Errorf("CursorUp at 0: expected 0, got %d", q.cursor)
	}
	q.cursor = 2
	q, _ = q.HandleAction(actionCursorDown)
	if q.cursor != 2 {
		t.Errorf("CursorDown at end: expected 2, got %d", q.cursor)
	}
}

func TestQueueHandleActionGoToTopBottom(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 10),
		cursor:  5,
		width:   80,
	}
	q, _ = q.HandleAction(actionGoToTop)
	if q.cursor != 0 || q.scrollOffset != 0 {
		t.Errorf("GoToTop: cursor=%d scroll=%d, want 0,0", q.cursor, q.scrollOffset)
	}
	q, _ = q.HandleAction(actionGoToBottom)
	if q.cursor != 9 {
		t.Errorf("GoToBottom: expected cursor=9, got %d", q.cursor)
	}
}

func TestQueueHandleActionHalfPageDownUp(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 20),
		cursor:  0,
		height:  14, // visibleRows = max(1, 14-4) = 10
		width:   80,
	}
	q, _ = q.HandleAction(actionHalfPageDown)
	if q.cursor != 5 {
		t.Errorf("HalfPageDown: expected 5, got %d", q.cursor)
	}
	q, _ = q.HandleAction(actionHalfPageUp)
	if q.cursor != 0 {
		t.Errorf("HalfPageUp: expected 0, got %d", q.cursor)
	}
}

func TestQueueHandleActionFullPageDownUp(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 20),
		cursor:  0,
		height:  14, // visibleRows = 10
		width:   80,
	}
	q, _ = q.HandleAction(actionFullPageDown)
	if q.cursor != 10 {
		t.Errorf("FullPageDown: expected 10, got %d", q.cursor)
	}
	q, _ = q.HandleAction(actionFullPageUp)
	if q.cursor != 0 {
		t.Errorf("FullPageUp: expected 0, got %d", q.cursor)
	}
}

func TestQueueMoveQueueCursorEmpty(t *testing.T) {
	q := QueueView{cursor: 0}
	q2 := q.moveQueueCursor(5)
	if q2.cursor != 0 {
		t.Errorf("moveQueueCursor empty: expected 0, got %d", q2.cursor)
	}
}

func TestQueueVisibleRowsZeroHeight(t *testing.T) {
	q := QueueView{height: 0}
	if q.visibleRows() != 50 {
		t.Errorf("visibleRows height=0: expected 50, got %d", q.visibleRows())
	}
}

func TestQueueVisibleRowsSmallHeight(t *testing.T) {
	q := QueueView{height: 5}
	if q.visibleRows() != 1 {
		t.Errorf("visibleRows height=5: expected 1, got %d", q.visibleRows())
	}
}

// --- PlaylistsView HandleAction navigation ---

func TestPlaylistsHandleActionCursorUpDown(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		cursor:    0,
		playlists: make([]db.PlaylistRow, 5),
	}
	v = v.HandleAction(actionCursorDown)
	if v.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.cursor)
	}
	v = v.HandleAction(actionCursorUp)
	if v.cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionGoToTopBottom(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		cursor:    0,
		playlists: make([]db.PlaylistRow, 10),
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 9 {
		t.Errorf("GoToBottom: expected 9, got %d", v.cursor)
	}
	v = v.HandleAction(actionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionHalfPage(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		cursor:    0,
		height:    13, // visibleRows = max(1, 13-3) = 10
		playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HandleAction(actionHalfPageDown)
	if v.cursor != 5 {
		t.Errorf("HalfPageDown: expected 5, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionFullPage(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		cursor:    0,
		height:    13, // visibleRows = 10
		playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HandleAction(actionFullPageDown)
	if v.cursor != 10 {
		t.Errorf("FullPageDown: expected 10, got %d", v.cursor)
	}
}

// --- QueueView HandleAction: ClearQueue, MoveTrackUp, MoveTrackDown ---

func TestQueueHandleActionClearQueue(t *testing.T) {
	q := QueueView{entries: make([]QueueEntry, 3)}
	_, cmd := q.HandleAction(actionClearQueue)
	if cmd == nil {
		t.Fatal("ClearQueue: expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(ClearQueueMsg); !ok {
		t.Errorf("ClearQueue: expected ClearQueueMsg, got %T", msg)
	}
}

func TestQueueHandleActionMoveTrackUp(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 3),
		cursor:  1,
	}
	_, cmd := q.HandleAction(actionMoveTrackUp)
	if cmd == nil {
		t.Fatal("MoveTrackUp: expected non-nil cmd")
	}
	msg := cmd()
	if m, ok := msg.(MoveTrackUpMsg); !ok || m.Index != 1 {
		t.Errorf("MoveTrackUp: got %+v", msg)
	}
}

func TestQueueHandleActionMoveTrackDown(t *testing.T) {
	q := QueueView{
		entries: make([]QueueEntry, 3),
		cursor:  1,
	}
	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd == nil {
		t.Fatal("MoveTrackDown: expected non-nil cmd")
	}
	msg := cmd()
	if m, ok := msg.(MoveTrackDownMsg); !ok || m.Index != 1 {
		t.Errorf("MoveTrackDown: got %+v", msg)
	}
}

// TestQueueBulkSelectionToggleNoPanicWhenCursorOutOfBounds verifies that the
// actionBulkSelectionToggle handler does not panic when cursor is out of bounds.
func TestQueueBulkSelectionToggleNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Display: "A"},
		},
		cursor: 5, // out of bounds
	}
	// Must not panic.
	_, cmd := q.HandleAction(actionBulkSelectionToggle)
	if cmd != nil {
		t.Fatal("actionBulkSelectionToggle with out-of-bounds cursor must return nil cmd")
	}
}
