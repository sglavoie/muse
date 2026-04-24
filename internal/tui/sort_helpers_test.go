package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestCycleSortField(t *testing.T) {
	fields := []SortField{SortDefault, SortArtist, SortTitle, SortAlbum}

	tests := []struct {
		name    string
		current SortField
		want    SortField
	}{
		{"default to artist", SortDefault, SortArtist},
		{"artist to title", SortArtist, SortTitle},
		{"title to album", SortTitle, SortAlbum},
		{"album wraps to default", SortAlbum, SortDefault},
		{"not in cycle returns first", SortDuration, SortDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CycleSortField(tt.current, fields)
			if got != tt.want {
				t.Errorf("CycleSortField(%d, fields) = %d, want %d", tt.current, got, tt.want)
			}
		})
	}
}

func TestCycleSortFieldSingleElement(t *testing.T) {
	fields := []SortField{SortDefault}
	got := CycleSortField(SortDefault, fields)
	if got != SortDefault {
		t.Errorf("single-element cycle: got %d, want %d", got, SortDefault)
	}
}

func TestCycleSortFieldEmpty(t *testing.T) {
	got := CycleSortField(SortDefault, nil)
	if got != SortDefault {
		t.Errorf("empty cycle: got %d, want %d", got, SortDefault)
	}
}

func TestSortTracks(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Charlie", Title: "Zulu", Album: "Red", Duration: 300},
		{Artist: "Alpha", Title: "Bravo", Album: "Blue", Duration: 100},
		{Artist: "", Title: "Unknown", Album: "", Duration: 200},
		{Artist: "Bravo", Title: "Alpha", Album: "Green", Duration: 150},
	}

	t.Run("sort by artist asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortArtist, SortAsc)
		if len(sorted) != 4 {
			t.Fatalf("expected 4 tracks, got %d", len(sorted))
		}
		if sorted[0].Artist != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Artist)
		}
		if sorted[1].Artist != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Artist)
		}
		if sorted[2].Artist != "Charlie" {
			t.Errorf("third should be Charlie, got %q", sorted[2].Artist)
		}
		if sorted[3].Artist != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Artist)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortArtist, SortDesc)
		if sorted[0].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", sorted[0].Artist)
		}
		if sorted[1].Artist != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Artist)
		}
		if sorted[2].Artist != "Alpha" {
			t.Errorf("third should be Alpha, got %q", sorted[2].Artist)
		}
		if sorted[3].Artist != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Artist)
		}
	})

	t.Run("sort by title asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortTitle, SortAsc)
		if sorted[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Title)
		}
		if sorted[1].Title != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Title)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortAsc)
		if sorted[0].Duration != 100 {
			t.Errorf("first duration should be 100, got %f", sorted[0].Duration)
		}
		if sorted[3].Duration != 300 {
			t.Errorf("last duration should be 300, got %f", sorted[3].Duration)
		}
	})

	t.Run("sort by duration desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortDesc)
		if sorted[0].Duration != 300 {
			t.Errorf("first duration should be 300, got %f", sorted[0].Duration)
		}
		if sorted[3].Duration != 100 {
			t.Errorf("last duration should be 100, got %f", sorted[3].Duration)
		}
	})

	t.Run("sort by album asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortAlbum, SortAsc)
		if sorted[0].Album != "Blue" {
			t.Errorf("first should be Blue, got %q", sorted[0].Album)
		}
		if sorted[3].Album != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Album)
		}
	})

	t.Run("sort by date added desc", func(t *testing.T) {
		dateTracks := []library.Track{
			{Title: "Old", CreatedAt: 10},
			{Title: "Newest", CreatedAt: 30},
			{Title: "Mid", CreatedAt: 20},
		}
		sorted := SortTracks(dateTracks, SortDateAdded, SortDesc)
		if sorted[0].Title != "Newest" {
			t.Errorf("first should be Newest, got %q", sorted[0].Title)
		}
		if sorted[2].Title != "Old" {
			t.Errorf("last should be Old, got %q", sorted[2].Title)
		}
	})

	t.Run("sort default returns nil", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDefault, SortAsc)
		if sorted != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("sort empty tracks returns nil", func(t *testing.T) {
		sorted := SortTracks(nil, SortArtist, SortAsc)
		if sorted != nil {
			t.Error("empty tracks should return nil")
		}
	})

	t.Run("original slice not modified", func(t *testing.T) {
		_ = SortTracks(tracks, SortArtist, SortAsc)
		if tracks[0].Artist != "Charlie" {
			t.Error("original slice was modified")
		}
	})
}

func TestSortFieldLabel(t *testing.T) {
	tests := []struct {
		field SortField
		want  string
	}{
		{SortDefault, "default"},
		{SortArtist, "Artist"},
		{SortTitle, "Title"},
		{SortAlbum, "Album"},
		{SortDuration, "Duration"},
		{SortDateAdded, "Date Added"},
		{SortName, "Name"},
		{SortTrackCount, "Track Count"},
		{SortSource, "Source"},
	}
	for _, tt := range tests {
		got := SortFieldLabel(tt.field)
		if got != tt.want {
			t.Errorf("SortFieldLabel(%d) = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestSortDirIndicator(t *testing.T) {
	if got := SortDirIndicator(SortAsc); got != "▲" {
		t.Errorf("SortAsc indicator = %q, want ▲", got)
	}
	if got := SortDirIndicator(SortDesc); got != "▼" {
		t.Errorf("SortDesc indicator = %q, want ▼", got)
	}
}

func TestSortStrings(t *testing.T) {
	items := []string{"Charlie", "Alpha", "", "Bravo"}

	t.Run("sort asc", func(t *testing.T) {
		sorted := SortStrings(items, SortAsc)
		if len(sorted) != 4 {
			t.Fatalf("expected 4, got %d", len(sorted))
		}
		if sorted[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0])
		}
		if sorted[1] != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1])
		}
		if sorted[2] != "Charlie" {
			t.Errorf("third should be Charlie, got %q", sorted[2])
		}
		if sorted[3] != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3])
		}
	})

	t.Run("sort desc", func(t *testing.T) {
		sorted := SortStrings(items, SortDesc)
		if sorted[0] != "Charlie" {
			t.Errorf("first should be Charlie, got %q", sorted[0])
		}
		if sorted[1] != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1])
		}
		if sorted[2] != "Alpha" {
			t.Errorf("third should be Alpha, got %q", sorted[2])
		}
		if sorted[3] != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3])
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		if SortStrings(nil, SortAsc) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("original not modified", func(t *testing.T) {
		_ = SortStrings(items, SortAsc)
		if items[0] != "Charlie" {
			t.Error("original slice was modified")
		}
	})

	t.Run("sentinel unknown artist sorts last asc", func(t *testing.T) {
		items := []string{"Charlie", "(unknown artist)", "Alpha"}
		sorted := SortStrings(items, SortAsc)
		if sorted[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0])
		}
		if sorted[1] != "Charlie" {
			t.Errorf("second should be Charlie, got %q", sorted[1])
		}
		if sorted[2] != "(unknown artist)" {
			t.Errorf("last should be (unknown artist), got %q", sorted[2])
		}
	})

	t.Run("sentinel unknown album sorts last desc", func(t *testing.T) {
		items := []string{"(unknown album)", "Bravo", "Alpha"}
		sorted := SortStrings(items, SortDesc)
		if sorted[0] != "Bravo" {
			t.Errorf("first should be Bravo, got %q", sorted[0])
		}
		if sorted[1] != "Alpha" {
			t.Errorf("second should be Alpha, got %q", sorted[1])
		}
		if sorted[2] != "(unknown album)" {
			t.Errorf("last should be (unknown album), got %q", sorted[2])
		}
	})

	t.Run("sentinel unknown genre sorts last", func(t *testing.T) {
		items := []string{"(unknown genre)", "Rock", "Jazz"}
		sorted := SortStrings(items, SortAsc)
		if sorted[0] != "Jazz" {
			t.Errorf("first should be Jazz, got %q", sorted[0])
		}
		if sorted[1] != "Rock" {
			t.Errorf("second should be Rock, got %q", sorted[1])
		}
		if sorted[2] != "(unknown genre)" {
			t.Errorf("last should be (unknown genre), got %q", sorted[2])
		}
	})
}

func TestAdjustScroll(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		scrollOffset int
		maxRows      int
		want         int
	}{
		{"cursor above viewport", 2, 5, 10, 2},
		{"cursor below viewport", 15, 0, 10, 6},
		{"cursor within viewport", 5, 0, 10, 0},
		{"cursor at viewport bottom edge", 9, 0, 10, 0},
		{"zero maxRows", 5, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustScroll(tt.cursor, tt.scrollOffset, tt.maxRows)
			if got != tt.want {
				t.Errorf("adjustScroll(%d, %d, %d) = %d, want %d",
					tt.cursor, tt.scrollOffset, tt.maxRows, got, tt.want)
			}
		})
	}
}

func TestFindStringPosition(t *testing.T) {
	items := []string{"Alpha", "Bravo", "Charlie"}
	if findStringPosition(items, "Bravo") != 1 {
		t.Error("expected 1 for Bravo")
	}
	if findStringPosition(items, "Missing") != 0 {
		t.Error("expected 0 for missing item")
	}
	if findStringPosition(nil, "A") != 0 {
		t.Error("expected 0 for nil items")
	}
}

func TestFindTrackHashPosition(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a"},
		{Hash: "b"},
		{Hash: "c"},
	}
	if findTrackHashPosition(tracks, "b") != 1 {
		t.Error("expected 1 for hash b")
	}
	if findTrackHashPosition(tracks, "missing") != 0 {
		t.Error("expected 0 for missing hash")
	}
	if findTrackHashPosition(tracks, "") != 0 {
		t.Error("expected 0 for empty hash")
	}
}

func TestFindPlaylistPosition(t *testing.T) {
	pls := []db.PlaylistRow{
		{ID: 10, Name: "A"},
		{ID: 20, Name: "B"},
		{ID: 30, Name: "C"},
	}
	if findPlaylistPosition(pls, 20) != 1 {
		t.Error("expected 1 for ID 20")
	}
	if findPlaylistPosition(pls, 99) != 0 {
		t.Error("expected 0 for missing ID")
	}
}

func TestCompareStringsUnknownLast(t *testing.T) {
	tests := []struct {
		a, b string
		dir  SortDir
		want bool
	}{
		{"Alpha", "Bravo", SortAsc, true},
		{"Alpha", "Bravo", SortDesc, false},
		{"", "Alpha", SortAsc, false},
		{"", "Alpha", SortDesc, false},
		{"Alpha", "", SortAsc, true},
		{"Alpha", "", SortDesc, true},
		{"", "", SortAsc, false},
		{"Unknown Artist", "Alpha", SortAsc, false},
		{"Unknown Artist", "Alpha", SortDesc, false},
	}
	for _, tt := range tests {
		got := compareStringsUnknownLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("compareStringsUnknownLast(%q, %q, %d) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}

func TestSortCycleQueueAllFields(t *testing.T) {
	state := SortState{Field: SortDefault}

	expected := []SortField{SortTitle, SortArtist, SortAlbum, SortDuration, SortSource, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, queueSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortCyclePlaylistListFields(t *testing.T) {
	state := SortState{Field: SortDefault}
	expected := []SortField{SortName, SortTrackCount, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, playlistListSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortCyclePlaylistTrackFields(t *testing.T) {
	state := SortState{Field: SortDefault}
	expected := []SortField{SortTitle, SortArtist, SortAlbum, SortDuration, SortDefault}
	for _, want := range expected {
		state.Field = CycleSortField(state.Field, playlistTrackSortFields)
		if state.Field != want {
			t.Errorf("expected %d, got %d", want, state.Field)
		}
	}
}

func TestSortTracksZeroDurationSortsLast(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Duration: 0},
		{Hash: "2", Duration: 200},
		{Hash: "3", Duration: 100},
		{Hash: "4", Duration: 0},
	}
	t.Run("asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortAsc)
		if sorted[0].Hash != "3" {
			t.Errorf("first should be h3 (100), got %s", sorted[0].Hash)
		}
		if sorted[1].Hash != "2" {
			t.Errorf("second should be h2 (200), got %s", sorted[1].Hash)
		}
		if sorted[2].Duration != 0 || sorted[3].Duration != 0 {
			t.Error("zero-duration tracks should be last")
		}
	})
	t.Run("desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortDesc)
		if sorted[0].Hash != "2" {
			t.Errorf("first should be h2 (200), got %s", sorted[0].Hash)
		}
		if sorted[1].Hash != "3" {
			t.Errorf("second should be h3 (100), got %s", sorted[1].Hash)
		}
		if sorted[2].Duration != 0 || sorted[3].Duration != 0 {
			t.Error("zero-duration tracks should be last")
		}
	})
}

func TestSortQueueIndicesZeroDurationSortsLast(t *testing.T) {
	entries := []QueueEntry{
		{Title: "A", DurationMs: 0},
		{Title: "B", DurationMs: 5000},
		{Title: "C", DurationMs: 2000},
	}
	t.Run("asc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortDuration, SortAsc)
		if entries[order[0]].Title != "C" {
			t.Errorf("first should be C (2000ms), got %s", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "B" {
			t.Errorf("second should be B (5000ms), got %s", entries[order[1]].Title)
		}
		if entries[order[2]].DurationMs != 0 {
			t.Error("zero-duration entry should be last")
		}
	})
	t.Run("desc", func(t *testing.T) {
		order := SortQueueIndices(entries, SortDuration, SortDesc)
		if entries[order[0]].Title != "B" {
			t.Errorf("first should be B (5000ms), got %s", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "C" {
			t.Errorf("second should be C (2000ms), got %s", entries[order[1]].Title)
		}
		if entries[order[2]].DurationMs != 0 {
			t.Error("zero-duration entry should be last")
		}
	})
}

func TestSortPlaylistTrackIndicesZeroDurationSortsLast(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{Hash: "h1", Title: "A", DurationMs: 0},
		{Hash: "h2", Title: "B", DurationMs: 3000},
		{Hash: "h3", Title: "C", DurationMs: 1000},
	}
	order := SortPlaylistTrackIndices(tracks, SortDuration, SortAsc)
	if tracks[order[0]].Hash != "h3" {
		t.Errorf("first should be h3 (1000ms), got %s", tracks[order[0]].Hash)
	}
	if tracks[order[1]].Hash != "h2" {
		t.Errorf("second should be h2 (3000ms), got %s", tracks[order[1]].Hash)
	}
	if tracks[order[2]].DurationMs != 0 {
		t.Error("zero-duration track should be last")
	}
}

func TestCompareDurationsZeroLast(t *testing.T) {
	tests := []struct {
		a, b float64
		dir  SortDir
		want bool
	}{
		{100, 200, SortAsc, true},
		{200, 100, SortAsc, false},
		{100, 200, SortDesc, false},
		{200, 100, SortDesc, true},
		{0, 100, SortAsc, false},  // zero after known
		{0, 100, SortDesc, false}, // zero after known
		{100, 0, SortAsc, true},   // known before zero
		{100, 0, SortDesc, true},  // known before zero
		{0, 0, SortAsc, false},    // stable
		{0, 0, SortDesc, false},   // stable
	}
	for _, tt := range tests {
		got := compareDurationsZeroLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("compareDurationsZeroLast(%v, %v, %v) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}

func TestCompareDurationMsZeroLast(t *testing.T) {
	tests := []struct {
		a, b int64
		dir  SortDir
		want bool
	}{
		{1000, 2000, SortAsc, true},
		{2000, 1000, SortAsc, false},
		{1000, 2000, SortDesc, false},
		{2000, 1000, SortDesc, true},
		{0, 1000, SortAsc, false},
		{0, 1000, SortDesc, false},
		{1000, 0, SortAsc, true},
		{1000, 0, SortDesc, true},
		{0, 0, SortAsc, false},
		{0, 0, SortDesc, false},
	}
	for _, tt := range tests {
		got := compareDurationMsZeroLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("compareDurationMsZeroLast(%v, %v, %v) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}

func TestSortTracksStableOrder(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "Same", Title: "B"},
		{Hash: "2", Artist: "Same", Title: "A"},
		{Hash: "3", Artist: "Same", Title: "C"},
	}
	sorted := SortTracks(tracks, SortArtist, SortAsc)
	if sorted[0].Hash != "1" || sorted[1].Hash != "2" || sorted[2].Hash != "3" {
		t.Error("stable sort should preserve order for equal keys")
	}
}

func TestSortTracksCaseInsensitive(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "bravo"},
		{Hash: "2", Artist: "Alpha"},
		{Hash: "3", Artist: "CHARLIE"},
	}
	sorted := SortTracks(tracks, SortArtist, SortAsc)
	if sorted[0].Artist != "Alpha" || sorted[1].Artist != "bravo" || sorted[2].Artist != "CHARLIE" {
		t.Errorf("case-insensitive sort failed: got %q, %q, %q",
			sorted[0].Artist, sorted[1].Artist, sorted[2].Artist)
	}
}

func TestSortNotification(t *testing.T) {
	tests := []struct {
		state SortState
		want  string
	}{
		{SortState{Field: SortDefault}, "Sort: default"},
		{SortState{Field: SortArtist, Dir: SortAsc}, "Sort: Artist ▲"},
		{SortState{Field: SortTitle, Dir: SortDesc}, "Sort: Title ▼"},
		{SortState{Field: SortDuration, Dir: SortAsc}, "Sort: Duration ▲"},
		{SortState{Field: SortName, Dir: SortDesc}, "Sort: Name ▼"},
		{SortState{Field: SortTrackCount, Dir: SortAsc}, "Sort: Track Count ▲"},
		{SortState{Field: SortSource, Dir: SortDesc}, "Sort: Source ▼"},
	}
	for _, tt := range tests {
		got := sortNotification(tt.state)
		if got != tt.want {
			t.Errorf("sortNotification(%+v) = %q, want %q", tt.state, got, tt.want)
		}
	}
}
