package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueRenderEmptyShowsPlaceholder verifies the empty-queue message.
func TestQueueRenderEmptyShowsPlaceholder(t *testing.T) {
	v := QueueView{}
	got := v.render(styles.NewStyles(true), -1, nil, nil, nil)
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
	out := v.render(styles.NewStyles(true), -1, nil, nil, nil)

	if !strings.Contains(out, "1") {
		t.Error("position column must contain entry index 1")
	}
	if !strings.Contains(out, "2") {
		t.Error("position column must contain entry index 2")
	}
	if !strings.Contains(out, "Song One") {
		t.Error("title column must contain 'Song One'")
	}
	if !strings.Contains(out, "Song Two") {
		t.Error("title column must contain 'Song Two'")
	}
	if !strings.Contains(out, "Band A") {
		t.Error("artist column must contain 'Band A'")
	}
	if !strings.Contains(out, "Band B") {
		t.Error("artist column must contain 'Band B'")
	}
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
	out := v.render(styles.NewStyles(true), -1, nil, nil, nil)
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
	out := v.render(s, -1, nil, nil, nil)
	if !strings.Contains(out, "[missing]") {
		t.Error("missing entry must contain '[missing]' label")
	}
	dimSample := styledText(s.Dim, "x")
	if ansi.Strip(dimSample) == dimSample {
		return
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
	v.cursor = 0
	s := styles.NewStyles(true)
	out := v.render(s, -1, nil, nil, nil)

	want := styledText(s.AccentMark, "●")
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
		Title:      strings.Repeat("T", 60),
		Artist:     strings.Repeat("A", 60),
		DurationMs: 90_000,
		Position:   1000,
	}

	narrow := makeQueue(80, []QueueEntry{entry})
	wide := makeQueue(160, []QueueEntry{entry})

	narrowOut := narrow.render(styles.NewStyles(true), -1, nil, nil, nil)
	wideOut := wide.render(styles.NewStyles(true), -1, nil, nil, nil)

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
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

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

// TestFormatQueueDuration covers the duration formatting helper.
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
// entry is rendered with a "●" selection marker.
func TestQueueRenderSelectedEntryShowsMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false},
		{Title: "Selected", Artist: "B", DurationMs: 120_000, Selected: true},
	})
	out := v.render(styles.NewStyles(true), -1, nil, nil, nil)
	if !strings.Contains(out, "●") {
		t.Fatal("selected queue entry must show a ● marker")
	}
}

// TestQueueRenderUnselectedEntryHasNoMarker verifies that unselected queue
// entries do not contain a selection marker glyph.
func TestQueueRenderUnselectedEntryHasNoMarker(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Normal", Artist: "A", DurationMs: 60_000, Selected: false}})
	out := v.render(styles.NewStyles(true), -1, nil, nil, nil)
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
	v.cursor = 0
	out := v.render(styles.NewStyles(true), 1, nil, nil, nil)
	stripped := ansi.Strip(out)

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
	v := makeQueue(120, []QueueEntry{{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000}})
	out := v.render(styles.NewStyles(true), -1, nil, nil, nil)
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
// currently playing and a favorite gets the NowPlayingFavorite style.
func TestQueueRenderNowPlayingFavoriteStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "FavPlaying", Artist: "B", Hash: "bbb", DurationMs: 120_000},
	})
	v.cursor = 0
	favs := map[string]struct{}{"bbb": {}}
	s := styles.NewStyles(true)
	out := v.render(s, 1, favs, nil, nil)

	npRef := s.NowPlaying.Render("REF")
	npfRef := s.NowPlayingFavorite.Render("REF")

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

	npPrefix, _, _ := strings.Cut(npRef, "REF")
	npfPrefix, _, _ := strings.Cut(npfRef, "REF")

	if !strings.Contains(rawLine, npfPrefix) {
		t.Errorf("now-playing+favorite title cell must use NowPlayingFavorite style; got: %q", rawLine)
	}
	if !strings.Contains(rawLine, npPrefix) {
		t.Errorf("non-favorited cells of now-playing entry must use NowPlaying style; got: %q", rawLine)
	}
}

// TestQueueRenderSelectedAndFocusedMarker verifies that a selected+focused
// entry uses a plain "●" glyph.
func TestQueueRenderSelectedAndFocusedMarker(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{{Title: "Focused", Artist: "A", DurationMs: 60_000, Selected: true}},
		cursor:  0,
		width:   120,
	}
	out := v.render(styles.NewStyles(true), -1, nil, nil, nil)
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

// TestQueueSelectNoPanicWhenCursorOutOfBounds verifies that the actionSelect
// handler in QueueView does not panic when the cursor exceeds the entries slice.
func TestQueueSelectNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := QueueView{entries: []QueueEntry{{Path: "/a.flac", Display: "A"}}, cursor: 5}
	_, cmd := q.HandleAction(actionSelect)
	if cmd != nil {
		t.Fatal("actionSelect with out-of-bounds cursor must return nil cmd")
	}
}

// TestQueueDequeueNoPanicWhenCursorOutOfBounds verifies that the actionDequeue
// handler in QueueView does not panic when the cursor exceeds the entries slice.
func TestQueueDequeueNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := QueueView{entries: []QueueEntry{{Path: "/a.flac", Display: "A"}}, cursor: 5}
	_, cmd := q.HandleAction(actionDequeue)
	if cmd != nil {
		t.Fatal("actionDequeue with out-of-bounds cursor must return nil cmd")
	}
}

func TestQueueHandleActionCursorUpDown(t *testing.T) {
	q := QueueView{entries: make([]QueueEntry, 5), cursor: 0, width: 80}
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
	q := QueueView{entries: make([]QueueEntry, 3), cursor: 0, width: 80}
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
	q := QueueView{entries: make([]QueueEntry, 10), cursor: 5, width: 80}
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
	q := QueueView{entries: make([]QueueEntry, 20), cursor: 0, height: 14, width: 80}
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
	q := QueueView{entries: make([]QueueEntry, 20), cursor: 0, height: 14, width: 80}
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
	q := QueueView{entries: make([]QueueEntry, 3), cursor: 1}
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
	q := QueueView{entries: make([]QueueEntry, 3), cursor: 1}
	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd == nil {
		t.Fatal("MoveTrackDown: expected non-nil cmd")
	}
	msg := cmd()
	if m, ok := msg.(MoveTrackDownMsg); !ok || m.Index != 1 {
		t.Errorf("MoveTrackDown: got %+v", msg)
	}
}

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

// TestQueueBulkSelectionToggleNoPanicWhenCursorOutOfBounds verifies that the
// actionBulkSelectionToggle handler does not panic when cursor is out of bounds.
func TestQueueBulkSelectionToggleNoPanicWhenCursorOutOfBounds(t *testing.T) {
	q := QueueView{entries: []QueueEntry{{Path: "/a.flac", Display: "A"}}, cursor: 5}
	_, cmd := q.HandleAction(actionBulkSelectionToggle)
	if cmd != nil {
		t.Fatal("actionBulkSelectionToggle with out-of-bounds cursor must return nil cmd")
	}
}

// TestQueueRenderArtistOnlyFavoriteStyle verifies that when only the artist is
// favorited, only the artist cell receives FavArtist styling.
func TestQueueRenderArtistOnlyFavoriteStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Song", Artist: "FavArtist", Album: "NormalAlbum", Hash: "aaa", DurationMs: 60_000}})
	v.cursor = 1
	favArtists := map[string]struct{}{"FavArtist": {}}
	s := styles.NewStyles(true)
	out := v.render(s, -1, nil, favArtists, nil)

	artistRef := s.FavArtist.Render("REF")
	if ansi.Strip(artistRef) == artistRef {
		return
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
	v := makeQueue(120, []QueueEntry{{Title: "Song", Artist: "FavArtist", Album: "FavAlbum", Hash: "favhash", DurationMs: 60_000}})
	v.cursor = 1
	favHashes := map[string]struct{}{"favhash": {}}
	favArtists := map[string]struct{}{"FavArtist": {}}
	favAlbums := map[string]struct{}{"FavAlbum": {}}
	s := styles.NewStyles(true)
	out := v.render(s, -1, favHashes, favArtists, favAlbums)

	titleRef := s.Favorite.Render("REF")
	if ansi.Strip(titleRef) == titleRef {
		return
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
// focused+favorited row gets Accent background wrapping while preserving the
// favorite title color.
func TestQueueRenderFocusedWithFavoriteUsesAccentBackground(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "FocFav", Artist: "A", Album: "B", Hash: "favhash", DurationMs: 60_000}})
	v.cursor = 0
	favHashes := map[string]struct{}{"favhash": {}}
	s := styles.NewStyles(true)
	out := v.render(s, -1, favHashes, nil, nil)

	favAccentRef := s.FavoriteAccent.Render("REF")
	if ansi.Strip(favAccentRef) == favAccentRef {
		return
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
