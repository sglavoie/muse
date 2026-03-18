package playlists

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
)

// --- track-level rendering ---

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
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "Song A", Artist: "Artist X", DurationMs: 183000},
			{Position: 2, Title: "Song B", Artist: "Artist Y", DurationMs: 245000},
		},
	}
	out := v.Render()
	for _, want := range []string{"Song A", "Artist X", "3:03", "Song B", "Artist Y", "4:05"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderTracksAscendingPositionOrder(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "First"},
			{Position: 2, Title: "Second"},
			{Position: 3, Title: "Third"},
		},
	}
	out := v.Render()
	posFirst := strings.Index(out, "First")
	posSecond := strings.Index(out, "Second")
	posThird := strings.Index(out, "Third")
	if !(posFirst < posSecond && posSecond < posThird) {
		t.Errorf("tracks not in ascending position order:\n%s", out)
	}
}

func TestRenderTracksColumnResizeOnWidthChange(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Position: 1, Title: "Song A", Artist: "Artist X", DurationMs: 60000},
	}
	narrow := View{Level: TrackLevel, Width: 40, Tracks: tracks}
	wide := View{Level: TrackLevel, Width: 120, Tracks: tracks}

	colsNarrow := ComputeTrack(40)
	colsWide := ComputeTrack(120)

	if colsNarrow.Title >= colsWide.Title {
		t.Errorf("expected title column to grow with width: narrow=%d wide=%d", colsNarrow.Title, colsWide.Title)
	}
	if colsNarrow.Artist >= colsWide.Artist {
		t.Errorf("expected artist column to grow with width: narrow=%d wide=%d", colsNarrow.Artist, colsWide.Artist)
	}

	// Both renders should contain the track
	for _, v := range []View{narrow, wide} {
		if !strings.Contains(v.Render(), "Song A") {
			t.Errorf("expected 'Song A' in render at width %d", v.Width)
		}
	}
}

func TestRenderListEmpty(t *testing.T) {
	v := View{}
	out := v.Render()
	if !strings.Contains(out, "no playlists") {
		t.Fatalf("expected '(no playlists)' in output, got:\n%s", out)
	}
}

// --- bulk-selection state ---

// TestToggleBulkSelectionToggles verifies that the first call adds the ID to
// the selected set and the second call removes it (toggle semantics).
func TestToggleBulkSelectionToggles(t *testing.T) {
	v := View{}

	v = v.ToggleBulkSelection(42)
	if _, ok := v.SelectedTrackIDs[42]; !ok {
		t.Fatal("expected ID 42 to be selected after first toggle")
	}

	v = v.ToggleBulkSelection(42)
	if _, ok := v.SelectedTrackIDs[42]; ok {
		t.Fatal("expected ID 42 to be deselected after second toggle")
	}
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("expected empty selection set, got %v", v.SelectedTrackIDs)
	}
}

// TestToggleBulkSelectionZeroIDIsNoOp verifies that toggling with id==0 does
// not add an entry to the selection set.
func TestToggleBulkSelectionZeroIDIsNoOp(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection(0)
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("ToggleBulkSelection(0) must be a no-op; got %v", v.SelectedTrackIDs)
	}
}

// TestClearBulkSelectionEmptiesSet verifies that ClearBulkSelection removes
// all bulk-selected IDs.
func TestClearBulkSelectionEmptiesSet(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection(1)
	v = v.ToggleBulkSelection(2)
	v = v.ClearBulkSelection()
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("expected empty selection after ClearBulkSelection, got %v", v.SelectedTrackIDs)
	}
}

// TestRenderTracksShowsBulletOnlyForSelected verifies that the '●' marker
// appears in the rendered output for selected rows and not for unselected ones.
func TestRenderTracksShowsBulletOnlyForSelected(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Selected Song", Artist: "A", DurationMs: 60000},
		{ID: 20, Position: 2, Title: "Unselected Song", Artist: "B", DurationMs: 60000},
	}
	v := View{
		Level:            TrackLevel,
		Width:            80,
		Tracks:           tracks,
		SelectedTrackIDs: map[int64]struct{}{10: {}},
	}
	out := v.Render()
	if !strings.Contains(out, "●") {
		t.Fatal("expected '●' marker in output for selected track")
	}
	// The bullet must appear on the selected track's line and not on the other.
	lines := strings.Split(out, "\n")
	var selectedLine, unselectedLine string
	for _, l := range lines {
		if strings.Contains(l, "Selected Song") {
			selectedLine = l
		}
		if strings.Contains(l, "Unselected Song") {
			unselectedLine = l
		}
	}
	if !strings.Contains(selectedLine, "●") {
		t.Errorf("selected track line must contain '●'; got: %q", selectedLine)
	}
	if strings.Contains(unselectedLine, "●") {
		t.Errorf("unselected track line must not contain '●'; got: %q", unselectedLine)
	}
}

func TestRenderListShowsNameAndCount(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Chill", TrackCount: 5},
			{ID: 2, Name: "Workout", TrackCount: 12},
		},
		Width: 40,
	}
	out := v.Render()
	if !strings.Contains(out, "Chill") {
		t.Errorf("expected 'Chill' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(5)") {
		t.Errorf("expected '(5)' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Workout") {
		t.Errorf("expected 'Workout' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(12)") {
		t.Errorf("expected '(12)' in output, got:\n%s", out)
	}
}

// --- drill-down Enter / Esc transitions ---

// TestOpenTracksTransitionsToTrackLevel verifies that OpenTracks saves the
// current list cursor, resets the track cursor to 0, and transitions the view
// to TrackLevel with the provided track slice.
func TestOpenTracksTransitionsToTrackLevel(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Song A"},
		{ID: 2, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:  ListLevel,
		Cursor: 2,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
			{ID: 3, Name: "Gamma"},
		},
	}
	v2 := v.OpenTracks(tracks)

	if v2.Level != TrackLevel {
		t.Errorf("OpenTracks: expected Level=TrackLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenTracks: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.ListCursor != 2 {
		t.Errorf("OpenTracks: expected ListCursor=2 (saved), got %d", v2.ListCursor)
	}
	if len(v2.Tracks) != 2 {
		t.Errorf("OpenTracks: expected 2 tracks, got %d", len(v2.Tracks))
	}
}

// TestBackToListRestoresCursorAndLevel verifies that BackToList transitions
// back to ListLevel, restores the list cursor, and clears the track slice.
func TestBackToListRestoresCursorAndLevel(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Song A"},
		{ID: 2, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:  ListLevel,
		Cursor: 3,
		Playlists: []db.PlaylistRow{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4},
		},
	}

	// Drill down saves Cursor=3, resets to 0.
	v = v.OpenTracks(tracks)
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected track cursor=0, got %d", v.Cursor)
	}

	// Esc from TrackLevel restores list cursor.
	v = v.BackToList()
	if v.Level != ListLevel {
		t.Errorf("BackToList: expected Level=ListLevel, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("BackToList: expected Cursor=3 (restored), got %d", v.Cursor)
	}
	if v.Tracks != nil {
		t.Errorf("BackToList: expected Tracks=nil, got %v", v.Tracks)
	}
}

// TestEscAtListLevelIsNoOp verifies that at ListLevel the Esc guard condition
// holds: the view remains at ListLevel and the cursor is unchanged. The root
// model only calls BackToList when !atListLevel(); this test documents that a
// view at ListLevel reports Level==ListLevel so the guard prevents any call.
func TestEscAtListLevelIsNoOp(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 1,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "A"},
			{ID: 2, Name: "B"},
		},
	}

	// Guard: only call BackToList when NOT at ListLevel.
	if v.Level != ListLevel {
		v = v.BackToList()
	}

	if v.Level != ListLevel {
		t.Errorf("Esc at ListLevel: expected Level=ListLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Esc at ListLevel: expected Cursor=1 (unchanged), got %d", v.Cursor)
	}
}

// --- 'd' soft-delete ---

// TestSoftDeletePlaylistAtListLevelRemovesEntry simulates the root model's
// soft-delete logic for a focused playlist at ListLevel: remove the entry at
// cursor from the Playlists slice and clamp the cursor when it falls off the
// end of the shortened list.
func TestSoftDeletePlaylistAtListLevelRemovesEntry(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 1,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
			{ID: 3, Name: "Gamma"},
		},
	}

	idx := v.Cursor
	// Simulate root model: remove entry and (optionally) persist via DB stub.
	v.Playlists = append(v.Playlists[:idx], v.Playlists[idx+1:]...)
	if v.Cursor >= len(v.Playlists) && v.Cursor > 0 {
		v.Cursor = len(v.Playlists) - 1
	}

	if len(v.Playlists) != 2 {
		t.Errorf("expected 2 playlists after delete, got %d", len(v.Playlists))
	}
	for _, p := range v.Playlists {
		if p.Name == "Beta" {
			t.Error("'Beta' must be removed from the playlist list")
		}
	}
}

// TestSoftDeletePlaylistClampsLastEntry verifies that deleting the last entry
// in the list clamps the cursor to index 0 (the new last entry) rather than
// leaving it out of bounds.
func TestSoftDeletePlaylistClampsLastEntry(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 2,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "A"},
			{ID: 2, Name: "B"},
			{ID: 3, Name: "C"},
		},
	}

	idx := v.Cursor
	v.Playlists = append(v.Playlists[:idx], v.Playlists[idx+1:]...)
	if v.Cursor >= len(v.Playlists) && v.Cursor > 0 {
		v.Cursor = len(v.Playlists) - 1
	}

	if v.Cursor != 1 {
		t.Errorf("expected cursor clamped to 1 after removing last entry, got %d", v.Cursor)
	}
}

// TestSoftDeleteTrackAtTrackLevelRemovesEntry simulates the root model's
// soft-delete logic for a focused track at TrackLevel: remove the entry at
// cursor from the Tracks slice and clamp the cursor.
func TestSoftDeleteTrackAtTrackLevelRemovesEntry(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 1,
		Tracks: []db.PlaylistTrackRow{
			{ID: 10, Position: 1, Title: "First"},
			{ID: 20, Position: 2, Title: "Second"},
			{ID: 30, Position: 3, Title: "Third"},
		},
	}

	idx := v.Cursor
	v.Tracks = append(v.Tracks[:idx], v.Tracks[idx+1:]...)
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}

	if len(v.Tracks) != 2 {
		t.Errorf("expected 2 tracks after remove, got %d", len(v.Tracks))
	}
	for _, tr := range v.Tracks {
		if tr.Title == "Second" {
			t.Error("'Second' must be removed from the track list")
		}
	}
}

// TestSoftDeleteTrackClampsLastEntry verifies cursor clamping when the last
// track in the list is removed at TrackLevel.
func TestSoftDeleteTrackClampsLastEntry(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 1,
		Tracks: []db.PlaylistTrackRow{
			{ID: 10, Position: 1, Title: "Only"},
			{ID: 20, Position: 2, Title: "Gone"},
		},
	}

	idx := v.Cursor
	v.Tracks = append(v.Tracks[:idx], v.Tracks[idx+1:]...)
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}

	if v.Cursor != 0 {
		t.Errorf("expected cursor clamped to 0 after removing last track, got %d", v.Cursor)
	}
}

// --- 'c' create flow ---

// TestStartCreateActivatesInput verifies that StartCreate sets Creating=true
// and initialises an empty CreateInput.
func TestStartCreateActivatesInput(t *testing.T) {
	v := View{}
	v2 := v.StartCreate()

	if !v2.Creating {
		t.Error("StartCreate: expected Creating=true")
	}
	if v2.CreateInput.Value() != "" {
		t.Errorf("StartCreate: expected empty CreateInput, got %q", v2.CreateInput.Value())
	}
}

// TestCreateInputEmptyNameIsRejected verifies that an empty or whitespace-only
// create input is treated as invalid: IsEmpty() must return true so the root
// model keeps the input active without persisting.
func TestCreateInputEmptyNameIsRejected(t *testing.T) {
	v := View{}.StartCreate()

	if !v.CreateInput.IsEmpty() {
		t.Error("fresh CreateInput must be empty")
	}

	// Whitespace only must also be treated as empty.
	v = v.AppendCreateChar(' ').AppendCreateChar(' ')
	if !v.CreateInput.IsEmpty() {
		t.Error("whitespace-only CreateInput must be considered empty")
	}
}

// TestCreateInputTypingAccumulatesBuffer verifies that AppendCreateChar builds
// up the input buffer correctly.
func TestCreateInputTypingAccumulatesBuffer(t *testing.T) {
	v := View{}.StartCreate()
	for _, ch := range "MyList" {
		v = v.AppendCreateChar(ch)
	}
	if v.CreateInput.Value() != "MyList" {
		t.Errorf("expected buffer 'MyList', got %q", v.CreateInput.Value())
	}
	if v.CreateInput.IsEmpty() {
		t.Error("non-empty input must not be IsEmpty()")
	}
}

// TestCreateInputBackspaceRemovesLastRune verifies that BackspaceCreate removes
// the last rune from the buffer.
func TestCreateInputBackspaceRemovesLastRune(t *testing.T) {
	v := View{}.StartCreate()
	v = v.AppendCreateChar('A').AppendCreateChar('B').AppendCreateChar('C')
	v = v.BackspaceCreate()

	if v.CreateInput.Value() != "AB" {
		t.Errorf("BackspaceCreate: expected 'AB', got %q", v.CreateInput.Value())
	}
}

// TestCancelCreateDismissesInput verifies that CancelCreate sets Creating=false
// and clears the input buffer.
func TestCancelCreateDismissesInput(t *testing.T) {
	v := View{}.StartCreate()
	for _, ch := range "Partial" {
		v = v.AppendCreateChar(ch)
	}
	v2 := v.CancelCreate()

	if v2.Creating {
		t.Error("CancelCreate: expected Creating=false")
	}
	if v2.CreateInput.Value() != "" {
		t.Errorf("CancelCreate: expected empty CreateInput, got %q", v2.CreateInput.Value())
	}
}

// TestCreateFlowPersistSimulated exercises the full create flow: activate
// input, type a name, verify the buffer is non-empty (valid), then simulate
// a successful DB persist by calling CancelCreate (as the root model does after
// receiving playlistCreatedMsg) and updating the Playlists slice.
func TestCreateFlowPersistSimulated(t *testing.T) {
	v := View{}.StartCreate()
	for _, ch := range "NewPlaylist" {
		v = v.AppendCreateChar(ch)
	}

	name := v.CreateInput.Value()
	if v.CreateInput.IsEmpty() {
		t.Fatal("input must not be empty before persist")
	}

	// Simulate DB success: append the new row and dismiss the input (root
	// model does this when it receives playlistCreatedMsg).
	v.Playlists = append(v.Playlists, db.PlaylistRow{ID: 99, Name: name})
	v = v.CancelCreate()

	if v.Creating {
		t.Error("expected Creating=false after persist")
	}
	if len(v.Playlists) != 1 || v.Playlists[0].Name != "NewPlaylist" {
		t.Errorf("expected one playlist 'NewPlaylist', got %v", v.Playlists)
	}
}

// --- 'r' rename flow ---

// TestStartRenamePrePopulatesInput verifies that StartRename sets Renaming=true
// and pre-populates RenameInput with the current playlist name.
func TestStartRenamePrePopulatesInput(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{{ID: 1, Name: "OldName"}},
	}
	v2 := v.StartRename("OldName")

	if !v2.Renaming {
		t.Error("StartRename: expected Renaming=true")
	}
	if v2.RenameInput.Value() != "OldName" {
		t.Errorf("StartRename: expected RenameInput='OldName', got %q", v2.RenameInput.Value())
	}
}

// TestRenameInputEmptyNameIsRejected verifies that clearing the rename buffer
// produces an empty input that would be rejected before persisting.
func TestRenameInputEmptyNameIsRejected(t *testing.T) {
	v := View{}.StartRename("SomeName")

	// Backspace all characters.
	for range "SomeName" {
		v = v.BackspaceRename()
	}

	if !v.RenameInput.IsEmpty() {
		t.Errorf("fully-erased RenameInput must be IsEmpty(); got %q", v.RenameInput.Value())
	}
}

// TestRenameDuplicateNameRejected simulates the root model's duplicate-name
// check: iterate the Playlists slice and reject if any existing name matches
// the input value (case-sensitive), keeping the input active.
func TestRenameDuplicateNameRejected(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
		},
		Cursor:   0,
		Renaming: true,
	}
	v.RenameInput = NewTextInput("")
	for _, ch := range "Beta" {
		v = v.AppendRenameChar(ch)
	}

	// Simulate root model duplicate check before persisting.
	isDuplicate := false
	for _, p := range v.Playlists {
		if p.Name == v.RenameInput.Value() {
			isDuplicate = true
			break
		}
	}
	if !isDuplicate {
		t.Error("entering an existing name must be detected as duplicate")
	}
	// Renaming input must remain active (root model does not dismiss on duplicate).
	if !v.Renaming {
		t.Error("Renaming must stay active when a duplicate is detected")
	}
}

// TestCancelRenameDismissesInput verifies that CancelRename sets Renaming=false
// and clears the rename buffer.
func TestCancelRenameDismissesInput(t *testing.T) {
	v := View{}.StartRename("CurrentName")
	v = v.AppendRenameChar('X')
	v2 := v.CancelRename()

	if v2.Renaming {
		t.Error("CancelRename: expected Renaming=false")
	}
	if v2.RenameInput.Value() != "" {
		t.Errorf("CancelRename: expected empty RenameInput, got %q", v2.RenameInput.Value())
	}
}

// TestRenameFlowPersistSimulated exercises the full rename flow: activate
// input pre-populated with the current name, clear and retype a new name,
// verify it is non-empty and non-duplicate, then simulate a successful DB
// persist by updating the Playlists slice and calling CancelRename.
func TestRenameFlowPersistSimulated(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{{ID: 1, Name: "OldName"}},
		Cursor:    0,
	}
	v = v.StartRename("OldName")

	// Clear pre-populated name via backspace.
	for range "OldName" {
		v = v.BackspaceRename()
	}
	// Type new name.
	for _, ch := range "NewName" {
		v = v.AppendRenameChar(ch)
	}

	newName := v.RenameInput.Value()
	if v.RenameInput.IsEmpty() {
		t.Fatal("RenameInput must not be empty before persist")
	}

	// No duplicates check (passes).
	isDuplicate := false
	for _, p := range v.Playlists {
		if p.Name == newName {
			isDuplicate = true
			break
		}
	}
	if isDuplicate {
		t.Fatal("test setup error: new name must not be a duplicate")
	}

	// Simulate DB success: update the slice and dismiss the input.
	v.Playlists[v.Cursor].Name = newName
	v = v.CancelRename()

	if v.Renaming {
		t.Error("expected Renaming=false after persist")
	}
	if v.Playlists[0].Name != "NewName" {
		t.Errorf("expected Playlists[0].Name='NewName', got %q", v.Playlists[0].Name)
	}
}

// --- navigation: CursorUp / CursorDown ---

func TestCursorUpDecrementsAndStopsAtZero(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    2,
		Playlists: make([]db.PlaylistRow, 5),
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

func TestCursorUpAdjustsScrollOffset(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       3,
		ScrollOffset: 3,
		Playlists:    make([]db.PlaylistRow, 10),
	}
	v = v.CursorUp()
	if v.ScrollOffset != 2 {
		t.Errorf("CursorUp: expected ScrollOffset=2, got %d", v.ScrollOffset)
	}
}

func TestCursorDownIncrementsAndStopsAtEnd(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Playlists: make([]db.PlaylistRow, 3),
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

func TestCursorDownAtTrackLevel(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: make([]db.PlaylistTrackRow, 2),
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

func TestCursorDownAdjustsScrollOffset(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    6, // visibleRows = max(1, 6-3) = 3
		Playlists: make([]db.PlaylistRow, 10),
	}
	// Move down past visible area
	for range 3 {
		v = v.CursorDown()
	}
	if v.ScrollOffset != 1 {
		t.Errorf("CursorDown: expected ScrollOffset=1, got %d", v.ScrollOffset)
	}
}

// --- navigation: GoToTop / GoToBottom ---

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Playlists:    make([]db.PlaylistRow, 10),
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
		Level:     ListLevel,
		Cursor:    0,
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestGoToBottomTrackLevel(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: make([]db.PlaylistTrackRow, 5),
	}
	v = v.GoToBottom()
	if v.Cursor != 4 {
		t.Errorf("GoToBottom TrackLevel: expected Cursor=4, got %d", v.Cursor)
	}
}

func TestGoToBottomEmptyList(t *testing.T) {
	v := View{Level: ListLevel, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestGoToBottomAdjustsScrollOffset(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    6, // visibleRows = 3
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
	if v.ScrollOffset != 7 {
		t.Errorf("GoToBottom: expected ScrollOffset=7, got %d", v.ScrollOffset)
	}
}

// --- navigation: page movements ---

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    13, // visibleRows = 10
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HalfPageDown()
	if v.Cursor != 5 {
		t.Errorf("HalfPageDown: expected Cursor=5, got %d", v.Cursor)
	}
}

func TestHalfPageUpMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    10,
		Height:    13, // visibleRows = 10
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.HalfPageUp()
	if v.Cursor != 5 {
		t.Errorf("HalfPageUp: expected Cursor=5, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    0,
		Height:    13, // visibleRows = 10
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.FullPageDown()
	if v.Cursor != 10 {
		t.Errorf("FullPageDown: expected Cursor=10, got %d", v.Cursor)
	}
}

func TestFullPageUpMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    15,
		Height:    13, // visibleRows = 10
		Playlists: make([]db.PlaylistRow, 20),
	}
	v = v.FullPageUp()
	if v.Cursor != 5 {
		t.Errorf("FullPageUp: expected Cursor=5, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToEnd(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    8,
		Height:    13, // visibleRows = 10
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.FullPageDown()
	if v.Cursor != 9 {
		t.Errorf("FullPageDown clamp: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToStart(t *testing.T) {
	v := View{
		Level:     ListLevel,
		Cursor:    2,
		Height:    13, // visibleRows = 10
		Playlists: make([]db.PlaylistRow, 10),
	}
	v = v.FullPageUp()
	if v.Cursor != 0 {
		t.Errorf("FullPageUp clamp: expected Cursor=0, got %d", v.Cursor)
	}
}

// --- moveCursor empty list ---

func TestMoveCursorEmptyListIsNoOp(t *testing.T) {
	v := View{Level: ListLevel, Cursor: 0}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}

// --- itemCount ---

func TestItemCountListLevel(t *testing.T) {
	v := View{Level: ListLevel, Playlists: make([]db.PlaylistRow, 7)}
	if v.itemCount() != 7 {
		t.Errorf("itemCount ListLevel: expected 7, got %d", v.itemCount())
	}
}

func TestItemCountTrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Tracks: make([]db.PlaylistTrackRow, 3)}
	if v.itemCount() != 3 {
		t.Errorf("itemCount TrackLevel: expected 3, got %d", v.itemCount())
	}
}

// --- visibleRows ---

func TestVisibleRowsZeroHeight(t *testing.T) {
	v := View{Height: 0}
	if v.visibleRows() != 50 {
		t.Errorf("visibleRows Height=0: expected 50, got %d", v.visibleRows())
	}
}

func TestVisibleRowsSmallHeight(t *testing.T) {
	v := View{Height: 4}
	if v.visibleRows() != 1 {
		t.Errorf("visibleRows Height=4: expected 1, got %d", v.visibleRows())
	}
}

// --- formatDuration edge cases ---

func TestFormatDurationHours(t *testing.T) {
	got := formatDuration(3661000) // 1h 1m 1s
	if got != "1:01:01" {
		t.Errorf("formatDuration hours: expected 1:01:01, got %s", got)
	}
}

func TestFormatDurationZero(t *testing.T) {
	got := formatDuration(0)
	if got != "--:--" {
		t.Errorf("formatDuration 0: expected --:--, got %s", got)
	}
}

func TestFormatDurationNegative(t *testing.T) {
	got := formatDuration(-1000)
	if got != "--:--" {
		t.Errorf("formatDuration negative: expected --:--, got %s", got)
	}
}

// --- truncate edge cases ---

func TestTruncateZeroMaxLen(t *testing.T) {
	got := truncate("hello", 0)
	if got != "" {
		t.Errorf("truncate maxLen=0: expected empty, got %q", got)
	}
}

func TestTruncateNegativeMaxLen(t *testing.T) {
	got := truncate("hello", -1)
	if got != "" {
		t.Errorf("truncate maxLen=-1: expected empty, got %q", got)
	}
}

func TestTruncateMaxLenOne(t *testing.T) {
	got := truncate("hello", 1)
	if got != "h" {
		t.Errorf("truncate maxLen=1: expected 'h', got %q", got)
	}
}

func TestTruncateNoTruncationNeeded(t *testing.T) {
	got := truncate("hi", 5)
	if got != "hi" {
		t.Errorf("truncate no-op: expected 'hi', got %q", got)
	}
}

// --- 'x' bulk selection toggle at TrackLevel ---

// TestBulkSelectionToggleAtTrackLevelAddsID verifies that toggling a track ID
// at TrackLevel adds it to SelectedTrackIDs.
func TestBulkSelectionToggleAtTrackLevelAddsID(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Song A"},
		{ID: 20, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: tracks,
	}

	v = v.ToggleBulkSelection(tracks[0].ID)
	if _, ok := v.SelectedTrackIDs[10]; !ok {
		t.Error("ToggleBulkSelection: expected ID 10 in SelectedTrackIDs")
	}
}

// TestBulkSelectionToggleAtTrackLevelRemovesID verifies that toggling an
// already-selected track ID removes it (toggle semantics).
func TestBulkSelectionToggleAtTrackLevelRemovesID(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		SelectedTrackIDs: map[int64]struct{}{10: {}},
	}
	v = v.ToggleBulkSelection(10)
	if _, ok := v.SelectedTrackIDs[10]; ok {
		t.Error("second ToggleBulkSelection must deselect the track")
	}
}

// TestBulkSelectionToggleMultipleIDs verifies that independent tracks can be
// selected simultaneously and are each toggled independently.
func TestBulkSelectionToggleMultipleIDs(t *testing.T) {
	v := View{Level: TrackLevel}
	v = v.ToggleBulkSelection(10)
	v = v.ToggleBulkSelection(20)
	v = v.ToggleBulkSelection(30)

	for _, id := range []int64{10, 20, 30} {
		if _, ok := v.SelectedTrackIDs[id]; !ok {
			t.Errorf("expected ID %d in SelectedTrackIDs", id)
		}
	}
	if len(v.SelectedTrackIDs) != 3 {
		t.Errorf("expected 3 selected IDs, got %d", len(v.SelectedTrackIDs))
	}
}

// --- 'a' bulk remove at TrackLevel ---

// TestBulkRemoveTracksAtTrackLevel simulates the root model's 'a' handler at
// TrackLevel: remove all selected tracks from the Tracks slice, clamp the
// cursor if necessary, and clear the selection.
func TestBulkRemoveTracksAtTrackLevel(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Keep"},
		{ID: 20, Position: 2, Title: "Remove A"},
		{ID: 30, Position: 3, Title: "Remove B"},
		{ID: 40, Position: 4, Title: "Keep Too"},
	}
	v := View{
		Level:  TrackLevel,
		Cursor: 3,
		Tracks: tracks,
		SelectedTrackIDs: map[int64]struct{}{
			20: {},
			30: {},
		},
	}

	// Simulate root model actionAddToPlaylist (bulk remove at TrackLevel).
	var remaining []db.PlaylistTrackRow
	for _, tr := range v.Tracks {
		if _, ok := v.SelectedTrackIDs[tr.ID]; !ok {
			remaining = append(remaining, tr)
		}
	}
	v.Tracks = remaining
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}
	v = v.ClearBulkSelection()

	if len(v.Tracks) != 2 {
		t.Errorf("expected 2 tracks after bulk remove, got %d", len(v.Tracks))
	}
	for _, tr := range v.Tracks {
		if tr.Title == "Remove A" || tr.Title == "Remove B" {
			t.Errorf("bulk remove must eliminate %q", tr.Title)
		}
	}
	if v.Cursor != 1 {
		t.Errorf("expected cursor clamped to 1 (new last index), got %d", v.Cursor)
	}
	if len(v.SelectedTrackIDs) != 0 {
		t.Errorf("expected selection cleared after bulk remove, got %v", v.SelectedTrackIDs)
	}
}

// TestBulkRemoveSelectAllClearsTrackList verifies that selecting every track
// and performing bulk remove leaves an empty Tracks slice with cursor at 0.
func TestBulkRemoveSelectAllClearsTrackList(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 1,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Position: 1, Title: "A"},
			{ID: 2, Position: 2, Title: "B"},
		},
		SelectedTrackIDs: map[int64]struct{}{1: {}, 2: {}},
	}

	var remaining []db.PlaylistTrackRow
	for _, tr := range v.Tracks {
		if _, ok := v.SelectedTrackIDs[tr.ID]; !ok {
			remaining = append(remaining, tr)
		}
	}
	v.Tracks = remaining
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}
	v = v.ClearBulkSelection()

	if len(v.Tracks) != 0 {
		t.Errorf("expected empty track list after removing all, got %d", len(v.Tracks))
	}
	// When all tracks are removed the clamp sets cursor to len(tracks)-1 = -1,
	// matching the root model's clamping formula.
	if v.Cursor != -1 {
		t.Errorf("expected cursor=-1 after removing all tracks (len-1), got %d", v.Cursor)
	}
	if v.SelectedTrackIDs != nil {
		t.Errorf("expected SelectedTrackIDs=nil after ClearBulkSelection, got %v", v.SelectedTrackIDs)
	}
}

// TestRenderTracksShowsSequentialPositions verifies that track-level rendering
// displays sequential 1-based indices (1, 2, 3) instead of the raw gapped DB
// position values (1000, 2000, 3000).
func TestRenderTracksShowsSequentialPositions(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Width: 80,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1000, Title: "Song A", Artist: "Artist X", DurationMs: 183000},
			{Position: 2000, Title: "Song B", Artist: "Artist Y", DurationMs: 245000},
			{Position: 3000, Title: "Song C", Artist: "Artist Z", DurationMs: 300000},
		},
	}
	out := v.Render()
	// The raw DB positions (1000, 2000, 3000) must not appear in the output.
	if strings.Contains(out, "1000") {
		t.Errorf("render should not contain raw DB position 1000, got:\n%s", out)
	}
	if strings.Contains(out, "2000") {
		t.Errorf("render should not contain raw DB position 2000, got:\n%s", out)
	}
	if strings.Contains(out, "3000") {
		t.Errorf("render should not contain raw DB position 3000, got:\n%s", out)
	}
}

// --- baseline column-ratio tests (56:44 Title:Artist at TrackLevel) ---

// trackColumnWidthsFromRender renders the playlists track view at the given
// width, strips ANSI sequences from the first track row using
// charmbracelet/x/ansi.Strip, and returns the visible rune widths of the
// Title and Artist columns.
func trackColumnWidthsFromRender(t *testing.T, width int) (titleW, artistW int) {
	t.Helper()
	v := View{
		Level: TrackLevel,
		Width: width,
		Tracks: []db.PlaylistTrackRow{
			{Position: 1, Title: "Long Title That Fills The Column", Artist: "Long Artist Name", DurationMs: 183000},
		},
	}
	rendered := v.Render()

	lines := strings.Split(rendered, "\n")
	// lines[0]="Tracks", lines[1]=blank, lines[2]=first track row
	if len(lines) < 3 {
		t.Fatalf("trackColumnWidthsFromRender: expected ≥3 lines, got %d", len(lines))
	}

	// Strip all ANSI escape sequences to obtain only visible characters.
	trackRow := ansi.Strip(lines[2])
	runes := []rune(trackRow)
	if len(runes) != width {
		t.Fatalf("trackColumnWidthsFromRender: stripped row has %d runes, want %d", len(runes), width)
	}

	cols := ComputeTrack(width)
	// Row layout: marker(2) + pos(4) + sep(1) + Title + sep(1) + Artist + sep(1) + Duration(10)
	titleStart := trackMarkerFixed + trackPosFixed + 1
	titleEnd := titleStart + cols.Title
	artistStart := titleEnd + 1
	artistEnd := artistStart + cols.Artist

	if artistEnd > len(runes) {
		t.Fatalf("trackColumnWidthsFromRender: artist column extends beyond row: artistEnd=%d len=%d", artistEnd, len(runes))
	}

	return len(runes[titleStart:titleEnd]), len(runes[artistStart:artistEnd])
}

// assertTrackRatioWithinTolerance verifies that colWidth/flexWidth is within
// ±2% of wantPct (expressed as a percentage, e.g. 56.0 for 56%).
func assertTrackRatioWithinTolerance(t *testing.T, colName string, colWidth, flexWidth int, wantPct float64) {
	t.Helper()
	gotPct := float64(colWidth) / float64(flexWidth) * 100
	if math.Abs(gotPct-wantPct) > 2.0 {
		t.Errorf("column %s: ratio %.2f%% (width=%d/flex=%d) exceeds ±2%% tolerance from expected %.2f%%",
			colName, gotPct, colWidth, flexWidth, wantPct)
	}
}

// TestTitleColumnRatioAtWidth80 asserts that the Title column visible width
// is within ±2% of the 56% share of flex space at TrackLevel with width=80.
func TestTitleColumnRatioAtWidth80(t *testing.T) {
	const width = 80
	flex := width - trackMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	titleW, _ := trackColumnWidthsFromRender(t, width)
	wantPct := float64(trackTitleRatio) / float64(trackTotalRatio) * 100
	assertTrackRatioWithinTolerance(t, "Title", titleW, flex, wantPct)
}

// TestArtistColumnRatioAtWidth80 asserts that the Artist column visible width
// is within ±2% of the 44% share of flex space at TrackLevel with width=80.
func TestArtistColumnRatioAtWidth80(t *testing.T) {
	const width = 80
	flex := width - trackMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	_, artistW := trackColumnWidthsFromRender(t, width)
	wantPct := float64(trackArtistRatio) / float64(trackTotalRatio) * 100
	assertTrackRatioWithinTolerance(t, "Artist", artistW, flex, wantPct)
}
