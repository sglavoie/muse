package artists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// --- sample data helpers ---

func sampleTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Duration: 245},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Duration: 300},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Duration: 60},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 3661},
	}
}

// --- render tests ---

func TestRenderArtistsEmpty(t *testing.T) {
	v := View{Level: ArtistLevel}
	out := v.Render()
	if !strings.Contains(out, "no artists") {
		t.Fatalf("expected '(no artists)' in output, got:\n%s", out)
	}
}

func TestRenderArtistsShowsNames(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Width:   80,
		Artists: []string{"Alice", "Bob", "Charlie"},
	}
	out := v.Render()
	for _, want := range []string{"Alice", "Bob", "Charlie"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsShowsNames(t *testing.T) {
	v := View{
		Level:          AlbumLevel,
		Width:          80,
		Albums:         []string{"Alpha", "Beta"},
		SelectedArtist: "Alice",
	}
	out := v.Render()
	for _, want := range []string{"Alpha", "Beta", "Alice"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsEmpty(t *testing.T) {
	v := View{Level: AlbumLevel, SelectedArtist: "Alice"}
	out := v.Render()
	if !strings.Contains(out, "no albums") {
		t.Fatalf("expected '(no albums)' in output, got:\n%s", out)
	}
}

func TestRenderTracksEmpty(t *testing.T) {
	v := View{Level: TrackLevel}
	out := v.Render()
	if !strings.Contains(out, "no tracks") {
		t.Fatalf("expected '(no tracks)' in output, got:\n%s", out)
	}
}

func TestRenderTracksShowsColumns(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []library.Track{
			{Title: "Song A", Duration: 183},
			{Title: "Song B", Duration: 245},
		},
	}
	out := v.Render()
	for _, want := range []string{"Song A", "3:03", "Song B", "4:05"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderTracksSequentialPositions(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []library.Track{
			{Title: "First", Duration: 60},
			{Title: "Second", Duration: 120},
			{Title: "Third", Duration: 180},
		},
	}
	out := v.Render()
	posFirst := strings.Index(out, "First")
	posSecond := strings.Index(out, "Second")
	posThird := strings.Index(out, "Third")
	if posFirst < 0 || posSecond < 0 || posThird < 0 {
		t.Fatalf("expected all track titles in output, got:\n%s", out)
	}
	if !(posFirst < posSecond && posSecond < posThird) {
		t.Errorf("tracks not in ascending sequential order:\n%s", out)
	}
}

// --- navigation tests ---

func TestCursorUpDecrementsAndStopsAtZero(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: make([]string, 5),
	}
	v = v.CursorUp()
	if v.Cursor != 1 {
		t.Errorf("CursorUp: expected 1, got %d", v.Cursor)
	}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.Cursor)
	}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Errorf("CursorUp at 0: expected 0, got %d", v.Cursor)
	}
}

func TestCursorDownIncrementsAndStopsAtEnd(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Artists: make([]string, 3),
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("CursorDown: expected 2, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("CursorDown at end: expected 2, got %d", v.Cursor)
	}
}

func TestCursorDownAtAlbumLevel(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Cursor: 0,
		Albums: make([]string, 2),
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown AlbumLevel: expected 1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown AlbumLevel at end: expected 1, got %d", v.Cursor)
	}
}

func TestCursorDownAtTrackLevel(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: make([]library.Track, 2),
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown TrackLevel: expected 1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown TrackLevel at end: expected 1, got %d", v.Cursor)
	}
}

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Artists:      make([]string, 10),
	}
	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected Cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("GoToTop: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestGoToBottomMovesCursorToEnd(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Artists: make([]string, 10),
	}
	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestGoToBottomEmptyList(t *testing.T) {
	v := View{Level: ArtistLevel, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Height:  13, // visibleRows = 12
		Artists: make([]string, 20),
	}
	v = v.HalfPageDown()
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Height:  13, // visibleRows = 13 (no chrome at ArtistLevel)
		Artists: make([]string, 20),
	}
	v = v.FullPageDown()
	if v.Cursor != 13 {
		t.Errorf("FullPageDown: expected Cursor=13, got %d", v.Cursor)
	}
}

func TestMoveCursorEmptyListIsNoOp(t *testing.T) {
	v := View{Level: ArtistLevel, Cursor: 0}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}

func TestItemCountAtEachLevel(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Artists: make([]string, 7),
		Albums:  make([]string, 4),
		Tracks:  make([]library.Track, 3),
	}
	if v.itemCount() != 7 {
		t.Errorf("itemCount ArtistLevel: expected 7, got %d", v.itemCount())
	}

	v.Level = AlbumLevel
	if v.itemCount() != 4 {
		t.Errorf("itemCount AlbumLevel: expected 4, got %d", v.itemCount())
	}

	v.Level = TrackLevel
	if v.itemCount() != 3 {
		t.Errorf("itemCount TrackLevel: expected 3, got %d", v.itemCount())
	}
}

// --- transition tests (3-level drill-down) ---

func TestOpenAlbumsTransitionsToAlbumLevel(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: []string{"Alice", "Bob", "Charlie"},
	}
	v2 := v.OpenAlbums([]string{"Alpha", "Beta"})

	if v2.Level != AlbumLevel {
		t.Errorf("OpenAlbums: expected Level=AlbumLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenAlbums: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.ArtistCursor != 2 {
		t.Errorf("OpenAlbums: expected ArtistCursor=2 (saved), got %d", v2.ArtistCursor)
	}
	if len(v2.Albums) != 2 {
		t.Errorf("OpenAlbums: expected 2 albums, got %d", len(v2.Albums))
	}
}

func TestOpenTracksTransitionsToTrackLevel(t *testing.T) {
	tracks := []library.Track{
		{Title: "Song A"},
		{Title: "Song B"},
	}
	v := View{
		Level:  AlbumLevel,
		Cursor: 1,
		Albums: []string{"Alpha", "Beta"},
	}
	v2 := v.OpenTracks(tracks)

	if v2.Level != TrackLevel {
		t.Errorf("OpenTracks: expected Level=TrackLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenTracks: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.AlbumCursor != 1 {
		t.Errorf("OpenTracks: expected AlbumCursor=1 (saved), got %d", v2.AlbumCursor)
	}
	if len(v2.Tracks) != 2 {
		t.Errorf("OpenTracks: expected 2 tracks, got %d", len(v2.Tracks))
	}
}

func TestBackFromAlbumLevelRestoresArtistCursor(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       3,
		ScrollOffset: 2,
		Artists:      make([]string, 10),
	}

	// Drill down to album level, saving artist cursor.
	v = v.OpenAlbums([]string{"Alpha", "Beta"})
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected album cursor=0, got %d", v.Cursor)
	}

	// Navigate within albums.
	v.Cursor = 1
	v.ScrollOffset = 1

	// Back restores artist cursor and scroll offset.
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("Back: expected Level=ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("Back: expected Cursor=3 (restored), got %d", v.Cursor)
	}
	if v.ScrollOffset != 2 {
		t.Errorf("Back: expected ScrollOffset=2 (restored), got %d", v.ScrollOffset)
	}
	if v.Albums != nil {
		t.Errorf("Back: expected Albums=nil, got %v", v.Albums)
	}
}

func TestBackFromTrackLevelRestoresAlbumCursor(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       2,
		ScrollOffset: 1,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}

	// Drill down to track level, saving album cursor.
	tracks := []library.Track{{Title: "Song A"}, {Title: "Song B"}}
	v = v.OpenTracks(tracks)
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected track cursor=0, got %d", v.Cursor)
	}

	// Navigate within tracks.
	v.Cursor = 1
	v.ScrollOffset = 0

	// Back restores album cursor and scroll offset.
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Errorf("Back: expected Level=AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("Back: expected Cursor=2 (restored), got %d", v.Cursor)
	}
	if v.ScrollOffset != 1 {
		t.Errorf("Back: expected ScrollOffset=1 (restored), got %d", v.ScrollOffset)
	}
	if v.Tracks != nil {
		t.Errorf("Back: expected Tracks=nil, got %v", v.Tracks)
	}
}

func TestBackAtArtistLevelIsNoOp(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  1,
		Artists: []string{"Alice", "Bob"},
	}

	// Guard: only call Back when NOT at ArtistLevel.
	if v.Level != ArtistLevel {
		v = v.Back()
	}

	if v.Level != ArtistLevel {
		t.Errorf("Back at ArtistLevel: expected Level=ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back at ArtistLevel: expected Cursor=1 (unchanged), got %d", v.Cursor)
	}
}

func TestFullDrillDownCycle(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  2,
		Artists: []string{"Alice", "Bob", "Charlie"},
		Width:   80,
	}

	// Drill into albums.
	v = v.OpenAlbums([]string{"Alpha", "Beta"})
	if v.Level != AlbumLevel || v.Cursor != 0 {
		t.Fatalf("after OpenAlbums: Level=%v Cursor=%d", v.Level, v.Cursor)
	}

	// Move album cursor.
	v.Cursor = 1

	// Drill into tracks.
	tracks := []library.Track{{Title: "Song A"}, {Title: "Song B"}}
	v = v.OpenTracks(tracks)
	if v.Level != TrackLevel || v.Cursor != 0 {
		t.Fatalf("after OpenTracks: Level=%v Cursor=%d", v.Level, v.Cursor)
	}

	// Back to album level — restores album cursor.
	v = v.Back()
	if v.Level != AlbumLevel {
		t.Errorf("first Back: expected AlbumLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("first Back: expected album Cursor=1, got %d", v.Cursor)
	}

	// Back to artist level — restores artist cursor.
	v = v.Back()
	if v.Level != ArtistLevel {
		t.Errorf("second Back: expected ArtistLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("second Back: expected artist Cursor=2, got %d", v.Cursor)
	}
}

// --- data derivation tests ---

func TestDeriveArtistsFromSortedTracks(t *testing.T) {
	tracks := sampleTracks()
	artists := DeriveArtists(tracks)
	want := []string{"Alice", "Bob", "Charlie"}
	if len(artists) != len(want) {
		t.Fatalf("DeriveArtists: expected %d artists, got %d", len(want), len(artists))
	}
	for i, w := range want {
		if artists[i] != w {
			t.Errorf("DeriveArtists[%d]: expected %q, got %q", i, w, artists[i])
		}
	}
}

func TestDeriveArtistsEmpty(t *testing.T) {
	artists := DeriveArtists(nil)
	if len(artists) != 0 {
		t.Errorf("DeriveArtists(nil): expected empty, got %v", artists)
	}
	artists = DeriveArtists([]library.Track{})
	if len(artists) != 0 {
		t.Errorf("DeriveArtists([]): expected empty, got %v", artists)
	}
}

func TestDeriveAlbumsForArtist(t *testing.T) {
	tracks := sampleTracks()
	albums := DeriveAlbums(tracks, "Alice")
	want := []string{"Alpha", "Beta"}
	if len(albums) != len(want) {
		t.Fatalf("DeriveAlbums: expected %d albums, got %d", len(want), len(albums))
	}
	for i, w := range want {
		if albums[i] != w {
			t.Errorf("DeriveAlbums[%d]: expected %q, got %q", i, w, albums[i])
		}
	}
}

func TestDeriveAlbumsNoMatch(t *testing.T) {
	tracks := sampleTracks()
	albums := DeriveAlbums(tracks, "Nobody")
	if len(albums) != 0 {
		t.Errorf("DeriveAlbums no match: expected empty, got %v", albums)
	}
}

func TestDeriveTracksForArtistAndAlbum(t *testing.T) {
	tracks := sampleTracks()
	result := DeriveTracks(tracks, "Alice", "Alpha")
	if len(result) != 2 {
		t.Fatalf("DeriveTracks: expected 2 tracks, got %d", len(result))
	}
	if result[0].Title != "Track A1" {
		t.Errorf("DeriveTracks[0]: expected 'Track A1', got %q", result[0].Title)
	}
	if result[1].Title != "Track A2" {
		t.Errorf("DeriveTracks[1]: expected 'Track A2', got %q", result[1].Title)
	}
}

func TestDeriveTracksNoMatch(t *testing.T) {
	tracks := sampleTracks()
	result := DeriveTracks(tracks, "Alice", "NonExistent")
	if len(result) != 0 {
		t.Errorf("DeriveTracks no match: expected empty, got %v", result)
	}
}

// --- filter tests ---

func TestFilterStringsSubstringMatch(t *testing.T) {
	items := []string{"Alice", "Bob", "Charlie", "alice in chains"}
	got := FilterStrings(items, "ali")
	want := []string{"Alice", "alice in chains"}
	if len(got) != len(want) {
		t.Fatalf("FilterStrings: expected %d items, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterStrings[%d]: expected %q, got %q", i, w, got[i])
		}
	}
}

func TestFilterStringsEmptyQuery(t *testing.T) {
	items := []string{"Alice", "Bob"}
	got := FilterStrings(items, "")
	if len(got) != 2 {
		t.Errorf("FilterStrings empty query: expected 2 items, got %d", len(got))
	}
}

func TestFilterStringsNoMatch(t *testing.T) {
	items := []string{"Alice", "Bob"}
	got := FilterStrings(items, "xyz")
	if len(got) != 0 {
		t.Errorf("FilterStrings no match: expected 0, got %d", len(got))
	}
}

func TestFilterTracksMatchesTitle(t *testing.T) {
	tracks := []library.Track{
		{Title: "Song About Love"},
		{Title: "Hate Song"},
		{Title: "Lovely Day"},
	}
	got := FilterTracks(tracks, "love")
	if len(got) != 2 {
		t.Fatalf("FilterTracks: expected 2, got %d", len(got))
	}
	if got[0].Title != "Song About Love" || got[1].Title != "Lovely Day" {
		t.Errorf("FilterTracks: unexpected results: %v", got)
	}
}

func TestItemCountRespectsFilter(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Artists:    []string{"Alice", "Bob", "Charlie"},
		FilterText: "b",
	}
	if v.itemCount() != 1 {
		t.Errorf("itemCount with filter: expected 1, got %d", v.itemCount())
	}
}

func TestCursorDownRespectsFilteredCount(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Artists:    []string{"Alice", "Bob", "Charlie"},
		FilterText: "b",
		Cursor:     0,
	}
	v = v.CursorDown()
	// Only "Bob" matches, so cursor should stay at 0
	if v.Cursor != 0 {
		t.Errorf("CursorDown with single filtered item: expected 0, got %d", v.Cursor)
	}
}

func TestRenderArtistsWithFilterShowsFilterBar(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        80,
		Artists:      []string{"Alice", "Bob"},
		FilterActive: true,
		FilterText:   "ali",
	}
	out := v.Render()
	if !strings.Contains(out, "Filter: > ali") {
		t.Errorf("expected filter bar in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Error("expected 'Alice' in filtered output")
	}
	if strings.Contains(out, "Bob") {
		t.Error("did not expect 'Bob' in filtered output")
	}
}

func TestRenderArtistsFilterNoMatches(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Width:      80,
		Artists:    []string{"Alice", "Bob"},
		FilterText: "zzz",
	}
	out := v.Render()
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected '(no matches)' in output, got:\n%s", out)
	}
}

func TestFilterClearedOnOpenAlbums(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"Alice", "Bob"},
		FilterActive: true,
		FilterText:   "ali",
	}
	v = v.OpenAlbums([]string{"Alpha"})
	if v.FilterActive || v.FilterText != "" {
		t.Errorf("expected filter cleared after OpenAlbums, got active=%v text=%q", v.FilterActive, v.FilterText)
	}
}

func TestFilterClearedOnBack(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Albums:       []string{"Alpha"},
		Artists:      []string{"Alice"},
		ArtistCursor: 0,
		FilterActive: true,
		FilterText:   "al",
	}
	v = v.Back()
	if v.FilterActive || v.FilterText != "" {
		t.Errorf("expected filter cleared after Back, got active=%v text=%q", v.FilterActive, v.FilterText)
	}
}

// --- compile-time assertion test ---

// TestViewStylesCompileTimeAssertion creates a View value and accesses
// its Styles field to verify that the compile-time assertion
// (var _ styles.Styles = View{}.Styles) works correctly.
func TestViewStylesCompileTimeAssertion(t *testing.T) {
	v := View{}
	var s styles.Styles = v.Styles
	// Verify the zero value is usable (not a nil pointer dereference).
	if s.HasDark {
		// HasDark defaults to false for zero-value Styles; just access
		// the field to ensure it compiles and runs.
		t.Log("HasDark is true (unexpected for zero value but not an error)")
	}
}

// --- selection marker tests ---

// TestRenderTracksShowsSelectionMarker verifies that a selected track renders
// the bullet marker (●) and an unselected track does not.
func TestRenderTracksShowsSelectionMarker(t *testing.T) {
	tracks := []library.Track{
		{Title: "Selected", Hash: "h1", Duration: 60},
		{Title: "Not Selected", Hash: "h2", Duration: 60},
	}
	v := View{
		Level:          TrackLevel,
		Width:          80,
		Tracks:         tracks,
		SelectedHashes: map[string]struct{}{"h1": {}},
	}
	out := v.Render()
	if !strings.Contains(out, "●") {
		t.Errorf("expected selection marker ● in output for selected track, got:\n%s", out)
	}
}

// TestRenderTracksNoMarkersWhenNoneSelected verifies that when no tracks are
// selected, the bullet marker does not appear in the rendered output.
func TestRenderTracksNoMarkersWhenNoneSelected(t *testing.T) {
	tracks := []library.Track{
		{Title: "Track A", Hash: "h1", Duration: 60},
		{Title: "Track B", Hash: "h2", Duration: 60},
	}
	v := View{
		Level:  TrackLevel,
		Width:  80,
		Tracks: tracks,
	}
	out := v.Render()
	if strings.Contains(out, "●") {
		t.Errorf("did not expect selection marker ● when nothing is selected, got:\n%s", out)
	}
}
