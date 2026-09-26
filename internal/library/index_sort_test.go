package library

import "testing"

// TestIndex is the entry point matched by -run TestIndex; subtests cover all
// index behaviours.
func TestIndex(t *testing.T) {
	t.Run("IncrementalInsertion", testIndexIncrementalInsertion)
	t.Run("ArtistThenTitleSortStability", testIndexArtistThenTitleSortStability)
	t.Run("UnknownArtistBottomSort", testIndexUnknownArtistBottomSort)
}

func testIndexIncrementalInsertion(t *testing.T) {
	var idx LibraryIndex
	toInsert := []Track{
		{Artist: "Z", Title: "z1"},
		{Artist: "A", Title: "a1"},
		{Artist: "M", Title: "m1"},
	}
	for i, tr := range toInsert {
		idx.Insert(tr)
		if idx.Len() != i+1 {
			t.Fatalf("after %d inserts: got Len()=%d, want %d", i+1, idx.Len(), i+1)
		}
	}
	// Tracks must be in sorted order after each incremental insert.
	got := idx.Tracks()
	if got[0].Artist != "A" || got[1].Artist != "M" || got[2].Artist != "Z" {
		t.Errorf("unexpected order after incremental inserts: %v", got)
	}
}

func testIndexArtistThenTitleSortStability(t *testing.T) {
	var idx LibraryIndex
	tracks := []Track{
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	for _, tr := range tracks {
		idx.Insert(tr)
	}
	want := []Track{
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	got := idx.Tracks()
	if len(got) != len(want) {
		t.Fatalf("got %d tracks, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Artist != w.Artist || got[i].Title != w.Title {
			t.Errorf("position %d: got {%q, %q}, want {%q, %q}",
				i, got[i].Artist, got[i].Title, w.Artist, w.Title)
		}
	}
}

func testIndexUnknownArtistBottomSort(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: unknownArtist, Title: "zzz"})
	idx.Insert(Track{Artist: "Arca", Title: "Piel"})
	idx.Insert(Track{Artist: unknownArtist, Title: "aaa"})
	idx.Insert(Track{Artist: "Boards of Canada", Title: "Roygbiv"})

	got := idx.Tracks()
	// All named artists must precede Unknown Artist entries.
	namedDone := false
	for _, tr := range got {
		if tr.Artist == unknownArtist {
			namedDone = true
		} else if namedDone {
			t.Errorf("named artist %q appeared after Unknown Artist block", tr.Artist)
		}
	}
	// Unknown Artist entries must themselves be title-sorted.
	if got[2].Title != "aaa" || got[3].Title != "zzz" {
		t.Errorf("Unknown Artist tracks not title-sorted: %v", got[2:])
	}
}

func TestLess_NamedBeforeUnknown(t *testing.T) {
	a := Track{Artist: "Arca", Title: "Piel"}
	b := Track{Artist: unknownArtist, Title: "abc"}
	if !less(a, b) {
		t.Error("named artist must sort before Unknown Artist")
	}
	if less(b, a) {
		t.Error("Unknown Artist must not sort before named artist")
	}
}

func TestLess_SameArtistByTitle(t *testing.T) {
	a := Track{Artist: "Aphex Twin", Title: "Actium"}
	b := Track{Artist: "Aphex Twin", Title: "Xtal"}
	if !less(a, b) {
		t.Error("same artist: earlier title must sort first")
	}
	if less(b, a) {
		t.Error("same artist: later title must not sort first")
	}
}

func TestLess_DifferentArtists(t *testing.T) {
	a := Track{Artist: "Arca", Title: "Piel"}
	b := Track{Artist: "Zappa", Title: "Bobby Brown"}
	if !less(a, b) {
		t.Error("Arca must sort before Zappa")
	}
}

func TestLess_BothUnknownByTitle(t *testing.T) {
	a := Track{Artist: unknownArtist, Title: "aaa"}
	b := Track{Artist: unknownArtist, Title: "zzz"}
	if !less(a, b) {
		t.Error("same Unknown Artist: earlier title must sort first")
	}
}

// The default library order must ignore case like every other sort in the app,
// so a lowercase artist does not drop below "Zappa".
func TestLess_ArtistAlbumTitleIgnoreCase(t *testing.T) {
	cases := []struct {
		name string
		a, b Track
	}{
		{"artist", Track{Artist: "alt-J", Title: "x"}, Track{Artist: "Zappa", Title: "a"}},
		{"album", Track{Artist: "A", Album: "an awesome wave", Title: "x"}, Track{Artist: "A", Album: "Zebra", Title: "a"}},
		{"title", Track{Artist: "A", Album: "B", Title: "bloodflood"}, Track{Artist: "A", Album: "B", Title: "Taro"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !less(c.a, c.b) {
				t.Errorf("%+v must sort before %+v", c.a, c.b)
			}
			if less(c.b, c.a) {
				t.Errorf("%+v must not sort before %+v", c.b, c.a)
			}
		})
	}
}

// Case-insensitive ordering must still keep each exact artist spelling in one
// contiguous run, and stay consistent between BulkLoad and Insert.
func TestLibraryIndex_CaseVariantsStayContiguous(t *testing.T) {
	tracks := []Track{
		{Hash: "1", Artist: "ABBA", Title: "b"},
		{Hash: "2", Artist: "abba", Title: "a"},
		{Hash: "3", Artist: "Abba", Title: "c"},
		{Hash: "4", Artist: "ABBA", Title: "a"},
		{Hash: "5", Artist: "abba", Title: "c"},
		{Hash: "6", Artist: "Abba", Title: "a"},
		{Hash: "7", Artist: "beck", Title: "a"},
		{Hash: "8", Artist: "Air", Title: "a"},
	}

	var bulk LibraryIndex
	bulk.BulkLoad(tracks)
	var inserted LibraryIndex
	for _, tr := range tracks {
		inserted.Insert(tr)
	}

	for name, idx := range map[string]*LibraryIndex{"BulkLoad": &bulk, "Insert": &inserted} {
		got := idx.Tracks()
		var order []string
		seen := map[string]bool{}
		for i, tr := range got {
			if i > 0 && got[i-1].Artist != tr.Artist && seen[tr.Artist] {
				t.Errorf("%s: artist %q is not contiguous: %v", name, tr.Artist, got)
			}
			if !seen[tr.Artist] {
				seen[tr.Artist] = true
				order = append(order, tr.Artist)
			}
		}
		want := []string{"ABBA", "Abba", "abba", "Air", "beck"}
		if len(order) != len(want) {
			t.Fatalf("%s: artist order = %v, want %v", name, order, want)
		}
		for i := range want {
			if order[i] != want[i] {
				t.Fatalf("%s: artist order = %v, want %v", name, order, want)
			}
		}
		for _, tr := range tracks {
			if _, ok := idx.findIndexByHash(tr.Hash); !ok {
				t.Errorf("%s: hash %q not found", name, tr.Hash)
			}
		}
	}
}

func TestFoldCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "ABC", 0},
		{"Émile", "émile", 0},
		{"a", "B", -1},
		{"b", "A", 1},
		{"ab", "ABC", -1},
		{"abc", "AB", 1},
	}
	for _, c := range cases {
		if got := foldCompare(c.a, c.b); got != c.want {
			t.Errorf("foldCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLess_IdenticalTracks(t *testing.T) {
	a := Track{Artist: "Arca", Title: "Piel"}
	if less(a, a) {
		t.Error("identical tracks: less must return false")
	}
}

func TestLibraryIndex_InsertSortedOrder(t *testing.T) {
	var idx LibraryIndex

	tracks := []Track{
		{Artist: "Zappa", Title: "Bobby Brown"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Aphex Twin", Title: "Flim"},
	}
	for _, tr := range tracks {
		idx.Insert(tr)
	}

	got := idx.Tracks()
	if len(got) != len(tracks) {
		t.Fatalf("expected %d tracks, got %d", len(tracks), len(got))
	}

	want := []Track{
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	for i, w := range want {
		if got[i].Artist != w.Artist || got[i].Title != w.Title {
			t.Errorf("position %d: got {%q, %q}, want {%q, %q}",
				i, got[i].Artist, got[i].Title, w.Artist, w.Title)
		}
	}
}

func TestLibraryIndex_UnknownArtistSortsLast(t *testing.T) {
	var idx LibraryIndex

	idx.Insert(Track{Artist: unknownArtist, Title: "some-file"})
	idx.Insert(Track{Artist: "Boards of Canada", Title: "Roygbiv"})
	idx.Insert(Track{Artist: unknownArtist, Title: "another-file"})
	idx.Insert(Track{Artist: "Arca", Title: "Piel"})

	got := idx.Tracks()
	// All named artists must come before Unknown Artist entries.
	namedDone := false
	for _, tr := range got {
		if tr.Artist == unknownArtist {
			namedDone = true
		} else if namedDone {
			t.Errorf("named artist %q appeared after Unknown Artist block", tr.Artist)
		}
	}
	// Verify Unknown Artist entries are themselves title-sorted.
	if got[2].Title != "another-file" || got[3].Title != "some-file" {
		t.Errorf("Unknown Artist tracks not title-sorted: %v", got[2:])
	}
}
