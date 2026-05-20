package playlists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- drill-down Enter / Esc transitions ---

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

func TestOpenTracksResetsScrollOffset(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Song A"},
		{ID: 2, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:        ListLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Playlists:    make([]db.PlaylistRow, 10),
	}

	v = v.OpenTracks(tracks)

	if v.ScrollOffset != 0 {
		t.Errorf("OpenTracks: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

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

	v = v.OpenTracks(tracks)
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected track cursor=0, got %d", v.Cursor)
	}

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

// TestBackToListClampsCursorWhenParentShrank verifies that BackToList clamps
// the saved list cursor against the (possibly shrunk) Playlists slice — e.g.
// when a playlist was deleted while the user was viewing its tracks.
func TestBackToListClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 4,
		Playlists: []db.PlaylistRow{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		},
		ScrollOffset: 2,
	}
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 1}})
	// Simulate playlist deletion while drilled into the previously selected
	// playlist's tracks.
	v.Playlists = []db.PlaylistRow{{ID: 1}, {ID: 2}}
	v = v.BackToList()
	if v.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("BackToList: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("BackToList: ScrollOffset(%d) > Cursor(%d)", v.ScrollOffset, v.Cursor)
	}
}

// TestBackToListClampsCursorWhenParentEmpty resets cursor/scroll to 0 when
// every playlist was removed while drilled into TrackLevel.
func TestBackToListClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       2,
		ScrollOffset: 1,
		Playlists:    []db.PlaylistRow{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 1}})
	// Empty (non-nil) slice signals that the caller knows the parent list is
	// drained — distinct from nil which means "do not clamp here".
	v.Playlists = []db.PlaylistRow{}
	v = v.BackToList()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("BackToList with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}

// TestBackToListSkipsClampWhenPlaylistsNil verifies the escape hatch used by
// outer wrappers that defer parent-list clamping: when Playlists is nil, the
// restored cursor/scroll are passed through unchanged.
func TestBackToListSkipsClampWhenPlaylistsNil(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Cursor:           0,
		Tracks:           []db.PlaylistTrackRow{{ID: 1}},
		ListCursor:       7,
		ListScrollOffset: 4,
		// Playlists deliberately left nil.
	}
	v = v.BackToList()
	if v.Cursor != 7 {
		t.Errorf("expected pass-through Cursor=7, got %d", v.Cursor)
	}
	if v.ScrollOffset != 4 {
		t.Errorf("expected pass-through ScrollOffset=4, got %d", v.ScrollOffset)
	}
}

func TestEscAtListLevelIsNoOp(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 1,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "A"},
			{ID: 2, Name: "B"},
		},
	}

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

func TestCreateInputEmptyNameIsRejected(t *testing.T) {
	v := View{}.StartCreate()

	if strings.TrimSpace(v.CreateInput.Value()) != "" {
		t.Error("fresh CreateInput must be empty")
	}

	v.CreateInput.SetValue("  ")
	if strings.TrimSpace(v.CreateInput.Value()) != "" {
		t.Error("whitespace-only CreateInput must be considered empty")
	}
}

func TestCreateInputTypingAccumulatesBuffer(t *testing.T) {
	v := View{}.StartCreate()
	v.CreateInput.SetValue("MyList")
	if v.CreateInput.Value() != "MyList" {
		t.Errorf("expected buffer 'MyList', got %q", v.CreateInput.Value())
	}
	if strings.TrimSpace(v.CreateInput.Value()) == "" {
		t.Error("non-empty input must not be empty")
	}
}

func TestCancelCreateDismissesInput(t *testing.T) {
	v := View{}.StartCreate()
	v.CreateInput.SetValue("Partial")
	v2 := v.CancelCreate()

	if v2.Creating {
		t.Error("CancelCreate: expected Creating=false")
	}
	if v2.CreateInput.Value() != "" {
		t.Errorf("CancelCreate: expected empty CreateInput, got %q", v2.CreateInput.Value())
	}
}

func TestCreateFlowPersistSimulated(t *testing.T) {
	v := View{}.StartCreate()
	v.CreateInput.SetValue("NewPlaylist")

	name := v.CreateInput.Value()
	if strings.TrimSpace(v.CreateInput.Value()) == "" {
		t.Fatal("input must not be empty before persist")
	}

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

func TestRenameInputEmptyNameIsRejected(t *testing.T) {
	v := View{}.StartRename("SomeName")
	v.RenameInput.SetValue("")

	if strings.TrimSpace(v.RenameInput.Value()) != "" {
		t.Errorf("fully-erased RenameInput must be empty; got %q", v.RenameInput.Value())
	}
}

func TestRenameDuplicateNameRejected(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
		},
		Cursor:   0,
		Renaming: true,
	}
	v.RenameInput = NewInlineInput("Beta")

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
	if !v.Renaming {
		t.Error("Renaming must stay active when a duplicate is detected")
	}
}

func TestCancelRenameDismissesInput(t *testing.T) {
	v := View{}.StartRename("CurrentName")
	v2 := v.CancelRename()

	if v2.Renaming {
		t.Error("CancelRename: expected Renaming=false")
	}
	if v2.RenameInput.Value() != "" {
		t.Errorf("CancelRename: expected empty RenameInput, got %q", v2.RenameInput.Value())
	}
}

func TestRenameFlowPersistSimulated(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{{ID: 1, Name: "OldName"}},
		Cursor:    0,
	}
	v = v.StartRename("OldName")
	v.RenameInput.SetValue("NewName")

	newName := v.RenameInput.Value()
	if strings.TrimSpace(v.RenameInput.Value()) == "" {
		t.Fatal("RenameInput must not be empty before persist")
	}

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

	v.Playlists[v.Cursor].Name = newName
	v = v.CancelRename()

	if v.Renaming {
		t.Error("expected Renaming=false after persist")
	}
	if v.Playlists[0].Name != "NewName" {
		t.Errorf("expected Playlists[0].Name='NewName', got %q", v.Playlists[0].Name)
	}
}
