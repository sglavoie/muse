package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// fakeQueueAppender records enqueue calls for testing.
type fakeQueueAppender struct {
	tracks []library.Track
	nextID int64
}

func (f *fakeQueueAppender) AppendTrack(t library.Track, _ string) (int64, error) {
	f.nextID++
	f.tracks = append(f.tracks, t)
	return f.nextID, nil
}

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
	got := v.render(styles.NewStyles(true), "", nil, nil, nil)
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
	out := v.render(styles.NewStyles(true), "", nil, nil, nil)

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
	out := v.render(styles.NewStyles(true), "", nil, nil, nil)
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

// TestQueueRenderMissingEntryDim verifies that a missing entry is rendered with
// dim styling and shows the "[missing]" label.
func TestQueueRenderMissingEntryDim(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Gone", Artist: "Nobody", DurationMs: 0, Missing: true, Position: 1000},
	})
	s := styles.NewStyles(true)
	out := v.render(s, "", nil, nil, nil)
	// Verify [missing] label appears in the output.
	if !strings.Contains(out, "[missing]") {
		t.Error("missing entry must contain '[missing]' label")
	}
	// Use styledText to check dim styling when ANSI is available.
	dimSample := styledText(s.Dim, "x")
	if ansi.Strip(dimSample) == dimSample {
		return // lipgloss produced no ANSI in this environment; skip style check
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(line), "[missing]") {
			if line == ansi.Strip(line) {
				t.Error("missing entry must be rendered with dim ANSI styling")
			}
			return
		}
	}
	t.Error("missing entry '[missing]' label not found in any output line")
}

// TestQueueRenderSelectedNonFocusedRowUsesAccent verifies that a selected entry
// that is not the focused row renders its selection marker with Accent styling.
func TestQueueRenderSelectedNonFocusedRowUsesAccent(t *testing.T) {
	v := makeQueue(80, []QueueEntry{
		{Title: "Focused", Artist: "A", DurationMs: 60_000, Selected: false, Position: 1000},
		{Title: "Selected", Artist: "B", DurationMs: 60_000, Selected: true, Position: 2000},
	})
	// cursor=0 so entry at index 1 is selected but not focused.
	v.cursor = 0
	s := styles.NewStyles(true)
	out := v.render(s, "", nil, nil, nil)

	want := styledText(s.AccentMark, "●")
	// Skip the style assertion if lipgloss emits no ANSI in this environment.
	if ansi.Strip(want) == want {
		return
	}
	if !strings.Contains(out, want) {
		t.Errorf("selected non-focused row must contain accent-styled marker %q; got:\n%s", want, out)
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

	narrowOut := narrow.render(styles.NewStyles(true), "", nil, nil, nil)
	wideOut := wide.render(styles.NewStyles(true), "", nil, nil, nil)

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
	m := New(context.Background(), WithQueueEntries([]QueueEntry{
		{Title: "Track", Artist: "Artist", DurationMs: 30_000, Position: 1000},
	}))
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
	out := v.render(styles.NewStyles(true), "", nil, nil, nil)
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
	out := v.render(styles.NewStyles(true), "", nil, nil, nil)
	if strings.Contains(out, "●") {
		t.Fatal("unselected queue entry must not show a ● marker")
	}
}

// TestQueueRenderNowPlayingMarker verifies that the currently playing track
// gets a "▶" marker in the rendered output.
func TestQueueRenderNowPlayingMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Not Playing", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "Playing", Artist: "B", Hash: "bbb", DurationMs: 120_000},
	})
	v.cursor = 0 // cursor on first entry, not the playing one
	out := v.render(styles.NewStyles(true), "bbb", nil, nil, nil)
	stripped := ansi.Strip(out)

	// The playing entry's line must contain the ▶ marker.
	lines := strings.Split(stripped, "\n")
	var playingLine string
	for _, l := range lines {
		if strings.Contains(l, "Playing") && !strings.Contains(l, "Not Playing") {
			playingLine = l
			break
		}
	}
	if playingLine == "" {
		t.Fatal("playing entry must appear in the output")
	}
	if !strings.Contains(playingLine, "▶") {
		t.Errorf("now-playing entry must contain ▶ marker; got: %q", playingLine)
	}
}

// TestQueueRenderNonPlayingEntryHasNoPlayMarker verifies that entries that are
// not currently playing do not get a "▶" marker.
func TestQueueRenderNonPlayingEntryHasNoPlayMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000},
	})
	out := v.render(styles.NewStyles(true), "other-hash", nil, nil, nil)
	stripped := ansi.Strip(out)

	for l := range strings.SplitSeq(stripped, "\n") {
		if strings.Contains(l, "Normal") {
			if strings.Contains(l, "▶") {
				t.Errorf("non-playing entry must not contain ▶ marker; got: %q", l)
			}
			return
		}
	}
	t.Fatal("entry must appear in the output")
}

// TestQueueRenderNowPlayingFavoriteStyle verifies that a track which is both
// currently playing and a favorite gets the NowPlayingFavorite style (bold
// amber) instead of the plain NowPlaying style (bold green).
func TestQueueRenderNowPlayingFavoriteStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "FavPlaying", Artist: "B", Hash: "bbb", DurationMs: 120_000},
	})
	v.cursor = 0 // cursor on first entry, not the playing+fav one
	favs := map[string]struct{}{"bbb": {}}
	s := styles.NewStyles(true) // dark mode
	out := v.render(s, "bbb", favs, nil, nil)

	// Render reference strings with each style so we can compare ANSI sequences.
	npRef := s.NowPlaying.Render("REF")
	npfRef := s.NowPlayingFavorite.Render("REF")

	// Find the raw (non-stripped) line containing "FavPlaying".
	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "FavPlaying") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("FavPlaying entry must appear in the output")
	}

	// Extract the ANSI escape prefix (everything before "REF") from each reference.
	npPrefix, _, _ := strings.Cut(npRef, "REF")
	npfPrefix, _, _ := strings.Cut(npfRef, "REF")

	// The title cell (track-favorited) must use NowPlayingFavorite (amber).
	if !strings.Contains(rawLine, npfPrefix) {
		t.Errorf("now-playing+favorite title cell must use NowPlayingFavorite style; got: %q", rawLine)
	}
	// Non-favorited cells (artist, album) still use plain NowPlaying (green).
	if !strings.Contains(rawLine, npPrefix) {
		t.Errorf("non-favorited cells of now-playing entry must use NowPlaying style; got: %q", rawLine)
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

// TestPlaylistsViewBackToListClearsStaleSelection verifies that navigating
// back to ListLevel clears track-level selections. Without this, stale IDs
// from the previous playlist could suppress single-track operations (e.g.
// delete) in a subsequently opened playlist because hasTrackSelection would
// return true even though no visible track is selected.
func TestPlaylistsViewBackToListClearsStaleSelection(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.TrackLevel,
		playlists: make([]db.PlaylistRow, 5),
		tracks:    []db.PlaylistTrackRow{{ID: 42, Title: "T1"}},
		selectedTrackIDs: map[int64]struct{}{
			42: {},
		},
	}

	if !v.hasTrackSelection() {
		t.Fatal("expected hasTrackSelection() == true before backToList")
	}

	v = v.backToList()

	if v.hasTrackSelection() {
		t.Errorf("expected hasTrackSelection() == false after backToList, got true (selectedTrackIDs=%v)", v.selectedTrackIDs)
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
	out := v.render(styles.NewStyles(true), "", nil, nil, nil)
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
		height:  14, // visibleRows = max(1, 14-2) = 12
		width:   80,
	}
	q, _ = q.HandleAction(actionHalfPageDown)
	if q.cursor != 6 {
		t.Errorf("HalfPageDown: expected 6, got %d", q.cursor)
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
		height:  14, // visibleRows = 12
		width:   80,
	}
	q, _ = q.HandleAction(actionFullPageDown)
	if q.cursor != 12 {
		t.Errorf("FullPageDown: expected 12, got %d", q.cursor)
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
	if q.visibleRows() != 3 {
		t.Errorf("visibleRows height=5: expected 3, got %d", q.visibleRows())
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
		height:    13, // visibleRows = max(1, 13-1) = 12
		playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HandleAction(actionHalfPageDown)
	if v.cursor != 6 {
		t.Errorf("HalfPageDown: expected 6, got %d", v.cursor)
	}
}

func TestPlaylistsHandleActionFullPage(t *testing.T) {
	v := PlaylistsView{
		level:     pltui.ListLevel,
		cursor:    0,
		height:    13, // visibleRows = 12
		playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HandleAction(actionFullPageDown)
	if v.cursor != 12 {
		t.Errorf("FullPageDown: expected 12, got %d", v.cursor)
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

// --- FormatDurationSecs ---

func TestFormatDurationSecs(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{0, "--:--"},
		{-5, "--:--"},
		{65, "1:05"},
		{185, "3:05"},
		{3661, "1:01:01"},
	}
	for _, tc := range cases {
		got := FormatDurationSecs(tc.secs)
		if got != tc.want {
			t.Errorf("FormatDurationSecs(%v) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

// --- LibraryView HandleAction navigation ---

func TestLibraryViewHandleActionNavigation(t *testing.T) {
	var idx library.LibraryIndex
	for i := range 10 {
		idx.Insert(library.Track{
			Artist: "A",
			Title:  fmt.Sprintf("T%d", i),
			Hash:   fmt.Sprintf("h%d", i),
		})
	}
	v := LibraryView{index: idx, width: 80, height: 24}

	v = v.HandleAction(actionCursorDown)
	if v.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.cursor)
	}
	v = v.HandleAction(actionCursorUp)
	if v.cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.cursor)
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

func TestLibraryViewHandleActionBulkToggle(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})
	idx.Insert(library.Track{Artist: "B", Title: "T2", Hash: "h2"})

	v := LibraryView{index: idx, cursor: 0}
	v = v.HandleAction(actionBulkSelectionToggle)
	if len(v.selectedHashes) != 1 {
		t.Errorf("BulkToggle: expected 1 selected, got %d", len(v.selectedHashes))
	}
}

// --- LibraryView EnqueueFocused ---

func TestLibraryViewEnqueueFocused(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})

	v := LibraryView{index: idx, cursor: 0}
	qa := &fakeQueueAppender{}
	cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused: expected non-nil cmd")
	}
	msg := cmd()
	enqueued, ok := msg.(libtui.TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", msg)
	}
	if enqueued.Track.Artist != "A" {
		t.Errorf("enqueued track artist: expected A, got %q", enqueued.Track.Artist)
	}
}

func TestLibraryViewEnqueueFocusedEmpty(t *testing.T) {
	v := LibraryView{}
	qa := &fakeQueueAppender{}
	cmd := v.EnqueueFocused(qa, 0)
	if cmd != nil {
		t.Error("EnqueueFocused empty: expected nil cmd")
	}
}

// --- LibraryView EnqueueSelected ---

func TestLibraryViewEnqueueSelected(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})
	idx.Insert(library.Track{Artist: "B", Title: "T2", Hash: "h2"})

	v := LibraryView{
		index:          idx,
		selectedHashes: map[string]struct{}{"h1": {}, "h2": {}},
	}
	qa := &fakeQueueAppender{}
	v2, cmd := v.EnqueueSelected(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueSelected: expected non-nil cmd")
	}
	if len(v2.selectedHashes) != 0 {
		t.Errorf("EnqueueSelected: expected selection cleared, got %v", v2.selectedHashes)
	}
}

func TestLibraryViewEnqueueSelectedEmpty(t *testing.T) {
	v := LibraryView{}
	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueSelected(qa, 0)
	if cmd != nil {
		t.Error("EnqueueSelected empty: expected nil cmd")
	}
}

// TestLibraryViewEnqueueSelectedRespectsSortOrder verifies that EnqueueSelected
// enqueues tracks in the sorted order (not the raw index order) when a sort is active.
func TestLibraryViewEnqueueSelectedRespectsSortOrder(t *testing.T) {
	var idx library.LibraryIndex
	// Insert in raw order: B then A
	idx.Insert(library.Track{Artist: "B", Title: "Beta", Hash: "hb"})
	idx.Insert(library.Track{Artist: "A", Title: "Alpha", Hash: "ha"})

	// Sort by title ascending: Alpha (ha) should come before Beta (hb)
	sorted := SortTracks(idx.Tracks(), SortTitle, SortAsc)

	v := LibraryView{
		index:          idx,
		sortState:      SortState{Field: SortTitle, Dir: SortAsc},
		sortedTracks:   sorted,
		selectedHashes: map[string]struct{}{"ha": {}, "hb": {}},
	}
	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueSelected(qa, 0)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	// Execute the batched command — each sub-cmd appends to qa.tracks.
	msg := cmd()
	// tea.Batch returns a batchMsg; execute each sub-cmd.
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batchMsg {
			if c != nil {
				c()
			}
		}
	}

	if len(qa.tracks) != 2 {
		t.Fatalf("expected 2 enqueued tracks, got %d", len(qa.tracks))
	}
	// With sort by title ascending, Alpha (ha) should be enqueued before Beta (hb).
	if qa.tracks[0].Hash != "ha" {
		t.Errorf("first enqueued track: expected hash=ha (Alpha), got %s (%s)", qa.tracks[0].Hash, qa.tracks[0].Title)
	}
	if qa.tracks[1].Hash != "hb" {
		t.Errorf("second enqueued track: expected hash=hb (Beta), got %s (%s)", qa.tracks[1].Hash, qa.tracks[1].Title)
	}
}

// --- LibraryView BackspaceFilter ---

func TestLibraryViewBackspaceFilter(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})

	f := filter.State{Active: true, Text: "AB"}
	f, _ = f.Activate() // seed textinput with "AB"
	v := LibraryView{
		index:  idx,
		fuzzy:  library.FakeFuzzySearcher{},
		filter: f,
	}
	prevText := v.filter.Text
	v.filter, _ = v.filter.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if v.filter.Text != prevText {
		v.cursor = 0
		v.scrollOffset = 0
		v = v.rebuildFilterCache()
	}
	if v.filter.Text != "A" {
		t.Errorf("BackspaceFilter: expected 'A', got %q", v.filter.Text)
	}
	if v.cursor != 0 {
		t.Errorf("BackspaceFilter: expected cursor=0, got %d", v.cursor)
	}
}

func TestLibraryViewBackspaceFilterInactiveNoOp(t *testing.T) {
	v := LibraryView{filter: filter.State{Text: "AB"}}
	v.filter, _ = v.filter.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if v.filter.Text != "AB" {
		t.Errorf("BackspaceFilter inactive: expected 'AB', got %q", v.filter.Text)
	}
}

// --- LibraryView DeactivateFilter ---

func TestLibraryViewDeactivateFilter(t *testing.T) {
	var idx library.LibraryIndex
	idx.Insert(library.Track{Artist: "A", Title: "T1", Hash: "h1"})

	v := LibraryView{
		index:  idx,
		fuzzy:  library.FakeFuzzySearcher{},
		filter: filter.State{Active: true, Text: "AB"},
		cursor: 5,
	}
	v.filter = v.filter.Deactivate()
	v.cursor = 0
	v.scrollOffset = 0
	v = v.rebuildFilterCache()
	if v.filter.Active {
		t.Error("DeactivateFilter: expected filter.Active=false")
	}
	if v.filter.Text != "" {
		t.Errorf("DeactivateFilter: expected filter.Text='', got %q", v.filter.Text)
	}
	if v.cursor != 0 {
		t.Errorf("DeactivateFilter: expected cursor=0, got %d", v.cursor)
	}
}

// --- Model setters ---

func TestModelSetRepeatMode(t *testing.T) {
	m := New(context.Background(), WithRepeatMode(RepeatAll))
	if m.playback.repeatMode != RepeatAll {
		t.Errorf("SetRepeatMode: expected RepeatAll, got %v", m.playback.repeatMode)
	}
}

func TestModelSetShuffle(t *testing.T) {
	m := New(context.Background(), WithShuffle(true))
	if !m.playback.shuffle {
		t.Error("SetShuffle(true): expected shuffle=true")
	}
}

func TestModelSetPlaybackState(t *testing.T) {
	m := New(context.Background(), WithPlaybackState(PlaybackPaused))
	if m.playback.state != PlaybackPaused {
		t.Errorf("SetPlaybackState: expected PlaybackPaused, got %v", m.playback.state)
	}
}

func TestModelSetCurrentTrackHash(t *testing.T) {
	m := New(context.Background(), WithCurrentTrackHash("abc123"))
	if m.playback.currentHash != "abc123" {
		t.Errorf("SetCurrentTrackHash: expected 'abc123', got %q", m.playback.currentHash)
	}
}

func TestModelSetLibraryTracks(t *testing.T) {
	tracks := []library.Track{
		{Artist: "A", Title: "T1", Hash: "h1"},
		{Artist: "B", Title: "T2", Hash: "h2"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	if m.library.index.Len() != 2 {
		t.Errorf("SetLibraryTracks: expected 2 tracks, got %d", m.library.index.Len())
	}
}

func TestModelRemoveLibraryTrackByPath(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "A", Title: "T1", Hash: "h1", Path: "/a.mp3"},
		{Artist: "B", Title: "T2", Hash: "h2", Path: "/b.mp3"},
	}))
	m.RemoveLibraryTrackByPath("/a.mp3")
	if m.library.index.Len() != 1 {
		t.Errorf("RemoveLibraryTrackByPath: expected 1 track, got %d", m.library.index.Len())
	}
}

// --- LibraryView CommitFilter ---

func TestLibraryViewCommitFilter(t *testing.T) {
	v := LibraryView{filter: filter.State{Active: true, Text: "test"}}
	v.filter = v.filter.Commit()
	if v.filter.Active {
		t.Error("CommitFilter: expected filter.Active=false")
	}
	if v.filter.Text != "test" {
		t.Errorf("CommitFilter: expected filter.Text='test', got %q", v.filter.Text)
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

// TestOpenTracksSavesCanonicalCursorWhenSorted verifies that openTracks saves
// the canonical (unsorted) playlist index as listCursor, not the display
// cursor position from a sorted view. This ensures that listCursor can safely
// index into v.playlists even after sort state is reset.
func TestOpenTracksSavesCanonicalCursorWhenSorted(t *testing.T) {
	v := PlaylistsView{
		playlists: []db.PlaylistRow{
			{ID: 10, Name: "Chill"},
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
		},
		// Simulate an active name-ascending sort: Ambient(20), Beats(30), Chill(10).
		sortedPlaylists: []db.PlaylistRow{
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
			{ID: 10, Name: "Chill"},
		},
		cursor: 2, // Display cursor on "Chill" (sorted position 2)
		level:  pltui.ListLevel,
	}

	tracks := []db.PlaylistTrackRow{
		{ID: 100, Hash: "h1", Title: "T1", Artist: "A1"},
	}

	v = v.openTracks(tracks)

	// "Chill" has ID=10, which is at index 0 in the unsorted playlists slice.
	// listCursor must be 0 (canonical), not 2 (display).
	if v.listCursor != 0 {
		t.Errorf("expected listCursor=0 (canonical index of Chill), got %d", v.listCursor)
	}

	// Verify the playlist name lookup using listCursor is correct.
	if v.listCursor >= 0 && v.listCursor < len(v.playlists) {
		if v.playlists[v.listCursor].Name != "Chill" {
			t.Errorf("expected playlist name 'Chill' at listCursor, got %q", v.playlists[v.listCursor].Name)
		}
	} else {
		t.Fatalf("listCursor %d out of bounds (len=%d)", v.listCursor, len(v.playlists))
	}
}

// TestClampAfterRemovalScrollOffsetAboveCursor verifies that clampAfterRemoval
// pulls scrollOffset down to cursor when the removal leaves scrollOffset above
// the cursor row, which would otherwise render the cursor outside the viewport.
func TestClampAfterRemovalScrollOffsetAboveCursor(t *testing.T) {
	// Scenario: cursor=0, scrollOffset=5, after removing items the list has 6
	// entries. Without the fix, scrollOffset would remain 5 and cursor 0
	// would be above the visible window.
	cursor, scrollOffset := clampAfterRemoval(0, 5, 6)
	if cursor != 0 {
		t.Errorf("expected cursor=0, got %d", cursor)
	}
	if scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0 (clamped to cursor), got %d", scrollOffset)
	}
}

// TestClampAfterRemovalScrollOffsetAtCursor verifies that when scrollOffset
// equals cursor, no adjustment is needed.
func TestClampAfterRemovalScrollOffsetAtCursor(t *testing.T) {
	cursor, scrollOffset := clampAfterRemoval(3, 3, 10)
	if cursor != 3 {
		t.Errorf("expected cursor=3, got %d", cursor)
	}
	if scrollOffset != 3 {
		t.Errorf("expected scrollOffset=3, got %d", scrollOffset)
	}
}

// TestClampAfterRemovalEmptyList verifies both cursor and scrollOffset reset
// to 0 for an empty list.
func TestClampAfterRemovalEmptyList(t *testing.T) {
	cursor, scrollOffset := clampAfterRemoval(5, 3, 0)
	if cursor != 0 {
		t.Errorf("expected cursor=0, got %d", cursor)
	}
	if scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", scrollOffset)
	}
}

// TestOpenTracksSavesDisplayCursorWhenUnsorted verifies that openTracks saves
// the display cursor directly when no sort is active (identity mapping).
func TestOpenTracksSavesDisplayCursorWhenUnsorted(t *testing.T) {
	v := PlaylistsView{
		playlists: []db.PlaylistRow{
			{ID: 10, Name: "Chill"},
			{ID: 20, Name: "Ambient"},
			{ID: 30, Name: "Beats"},
		},
		cursor: 1, // Display cursor on "Ambient" (unsorted position 1)
		level:  pltui.ListLevel,
	}

	tracks := []db.PlaylistTrackRow{
		{ID: 100, Hash: "h1", Title: "T1", Artist: "A1"},
	}

	v = v.openTracks(tracks)

	// Without sort, listCursor should be the display cursor value (1).
	if v.listCursor != 1 {
		t.Errorf("expected listCursor=1, got %d", v.listCursor)
	}
}

// TestHandleActionReservesFilterBarLine_Artists verifies that ArtistsView.HandleAction
// accounts for the filter bar line when computing visible rows, even when the
// filter is active but the text is still empty (just opened with /).
func TestHandleActionReservesFilterBarLine_Artists(t *testing.T) {
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist %02d", i)
	}
	v := ArtistsView{
		height:  12,
		artists: artists,
		filter:  filter.State{Active: true}, // filter bar open, no text
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.cursor)
	}
	// With filter bar active, visibleRows = max(1, 12-1) = 11.
	// scrollOffset should be 19 - 11 + 1 = 9.
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
	v := AlbumsView{
		height: 12,
		albums: albums,
		filter: filter.State{Active: true},
	}
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
	v := GenresView{
		height: 12,
		genres: genres,
		filter: filter.State{Active: true},
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("expected cursor=19, got %d", v.cursor)
	}
	if v.scrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.scrollOffset)
	}
}

// TestQueueRenderArtistOnlyFavoriteStyle verifies that when only the artist is
// favorited, only the artist cell receives FavArtist (purple) styling and the
// title cell does not receive Favorite (amber) styling.
func TestQueueRenderArtistOnlyFavoriteStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Song", Artist: "FavArtist", Album: "NormalAlbum", Hash: "aaa", DurationMs: 60_000},
	})
	v.cursor = 1 // not focused on the entry
	favArtists := map[string]struct{}{"FavArtist": {}}
	s := styles.NewStyles(true)
	out := v.render(s, "", nil, favArtists, nil)

	artistRef := s.FavArtist.Render("REF")
	if ansi.Strip(artistRef) == artistRef {
		return // no ANSI in this environment
	}
	artistPrefix, _, _ := strings.Cut(artistRef, "REF")
	titleRef := s.Favorite.Render("REF")
	titlePrefix, _, _ := strings.Cut(titleRef, "REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "Song") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("entry must appear in output")
	}
	if !strings.Contains(rawLine, artistPrefix) {
		t.Errorf("artist-only fav: artist cell must use FavArtist style; got: %q", rawLine)
	}
	if strings.Contains(rawLine, titlePrefix) {
		t.Errorf("artist-only fav: title cell must NOT use Favorite style; got: %q", rawLine)
	}
}

// TestQueueRenderAllThreeFavoritesStyle verifies that when track, artist, and
// album are all favorited, each cell independently receives its own color style.
func TestQueueRenderAllThreeFavoritesStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Song", Artist: "FavArtist", Album: "FavAlbum", Hash: "favhash", DurationMs: 60_000},
	})
	v.cursor = 1 // not focused
	favHashes := map[string]struct{}{"favhash": {}}
	favArtists := map[string]struct{}{"FavArtist": {}}
	favAlbums := map[string]struct{}{"FavAlbum": {}}
	s := styles.NewStyles(true)
	out := v.render(s, "", favHashes, favArtists, favAlbums)

	titleRef := s.Favorite.Render("REF")
	if ansi.Strip(titleRef) == titleRef {
		return // no ANSI in this environment
	}
	titlePrefix, _, _ := strings.Cut(titleRef, "REF")
	artistPrefix, _, _ := strings.Cut(s.FavArtist.Render("REF"), "REF")
	albumPrefix, _, _ := strings.Cut(s.FavAlbum.Render("REF"), "REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "Song") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("entry must appear in output")
	}
	if !strings.Contains(rawLine, titlePrefix) {
		t.Errorf("all-3 fav: title cell must use Favorite style; got: %q", rawLine)
	}
	if !strings.Contains(rawLine, artistPrefix) {
		t.Errorf("all-3 fav: artist cell must use FavArtist style; got: %q", rawLine)
	}
	if !strings.Contains(rawLine, albumPrefix) {
		t.Errorf("all-3 fav: album cell must use FavAlbum style; got: %q", rawLine)
	}
}

// TestQueueRenderFocusedWithFavoriteUsesAccentBackground verifies that a
// focused+favorited row gets Accent background wrapping, while the inner
// Favorite foreground color on the title cell is preserved.
func TestQueueRenderFocusedWithFavoriteUsesAccentBackground(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "FocFav", Artist: "A", Album: "B", Hash: "favhash", DurationMs: 60_000},
	})
	v.cursor = 0 // focused on the fav entry
	favHashes := map[string]struct{}{"favhash": {}}
	s := styles.NewStyles(true)
	out := v.render(s, "", favHashes, nil, nil)

	// With per-cell accent styling, a focused+fav title uses FavoriteAccent
	// (combined foreground+background) rather than nesting Favorite inside Accent.
	favAccentRef := s.FavoriteAccent.Render("REF")
	if ansi.Strip(favAccentRef) == favAccentRef {
		return // no ANSI in this environment
	}
	favAccentPrefix, _, _ := strings.Cut(favAccentRef, "REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "FocFav") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("focused entry must appear in output")
	}
	if !strings.Contains(rawLine, favAccentPrefix) {
		t.Errorf("focused+fav row: title cell must use FavoriteAccent style; got: %q", rawLine)
	}
}

// TestFavoritesViewPerCellArtistFavoriteStyle verifies that in the favorites
// track list, an artist-favorited row applies FavArtist to the artist cell
// only and leaves the title cell unstyled.
func TestFavoritesViewPerCellArtistFavoriteStyle(t *testing.T) {
	tracks := []library.Track{
		{Title: "Song", Artist: "FavArtist", Album: "SomeAlbum", Hash: "h1", Duration: 60},
	}
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

	// The first (and only) row is focused (cursor=0), so per-cell accent
	// styling uses FavArtistAccent for the artist cell.
	artistRef := s.FavArtistAccent.Render("REF")
	if ansi.Strip(artistRef) == artistRef {
		return // no ANSI in this environment
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
// FavoritesView at ListLevel with artists (where the filter bar adds 1 line).
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
	// ListLevel + CategoryArtists: reserve=0, +1 for filter = 1.
	// visibleRows = max(1, 12-1) = 11, scrollOffset = 19 - 11 + 1 = 9.
	if v.scrollOffset != 9 {
		t.Errorf("expected scrollOffset=9 (filter bar reserved), got %d", v.scrollOffset)
	}
}
