package metaexport

import (
	"bytes"
	"reflect"
	"testing"
)

func libraryTrack() Current {
	return Current{
		Path:   "/Music/Rock/a.mp3",
		Artist: "Miles",
		Title:  "So What",
		Album:  "Kind of Blue",
		Genre:  "Jazz; Cool",
		Track:  1,
		Disc:   1,
	}
}

func exportRow() Row {
	return Row{
		Hash:    "a3f",
		RootID:  1,
		RelPath: "Rock/a.mp3",
		Artist:  "Miles",
		Title:   "So What",
		Album:   "Kind of Blue",
		Genre:   "Jazz; Cool",
		Track:   1,
		Disc:    1,
		Line:    5,
	}
}

func diffOne(row Row) Plan {
	return Diff([]Row{row}, map[string]Current{"a3f": libraryTrack()})
}

func TestDiffDetectsPerFieldChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Row)
		field  func(Change) any
		want   any
		counts FieldCounts
	}{
		{
			name:   "artist",
			mutate: func(r *Row) { r.Artist = "Miles Davis" },
			field:  func(c Change) any { return c.Artist },
			want:   "Miles Davis",
			counts: FieldCounts{Artist: 1},
		},
		{
			name:   "title",
			mutate: func(r *Row) { r.Title = "Blue in Green" },
			field:  func(c Change) any { return c.Title },
			want:   "Blue in Green",
			counts: FieldCounts{Title: 1},
		},
		{
			name:   "album",
			mutate: func(r *Row) { r.Album = "Milestones" },
			field:  func(c Change) any { return c.Album },
			want:   "Milestones",
			counts: FieldCounts{Album: 1},
		},
		{
			name:   "genre stores the raw new string",
			mutate: func(r *Row) { r.Genre = "Bebop, Cool" },
			field:  func(c Change) any { return c.Genre },
			want:   "Bebop, Cool",
			counts: FieldCounts{Genre: 1},
		},
		{
			name:   "artist cleared",
			mutate: func(r *Row) { r.Artist = "" },
			field:  func(c Change) any { return c.Artist },
			want:   "",
			counts: FieldCounts{Artist: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := exportRow()
			tt.mutate(&row)
			p := diffOne(row)
			if len(p.Changes) != 1 {
				t.Fatalf("got %d changes, want 1", len(p.Changes))
			}
			got := tt.field(p.Changes[0]).(*string)
			if got == nil || *got != tt.want.(string) {
				t.Fatalf("field = %v, want %q", got, tt.want)
			}
			if p.Counts != tt.counts {
				t.Fatalf("counts = %+v, want %+v", p.Counts, tt.counts)
			}
		})
	}
}

func TestDiffPositions(t *testing.T) {
	row := exportRow()
	row.Track = 4
	row.Disc = 2
	p := diffOne(row)
	if len(p.Changes) != 1 {
		t.Fatalf("got %d changes, want 1", len(p.Changes))
	}
	c := p.Changes[0]
	if c.Track == nil || *c.Track != 4 || c.Disc == nil || *c.Disc != 2 {
		t.Fatalf("track/disc = %v/%v, want 4/2", c.Track, c.Disc)
	}
	if c.Artist != nil || c.Title != nil || c.Album != nil || c.Genre != nil {
		t.Fatalf("unchanged text fields should be nil: %+v", c)
	}
	if p.Counts != (FieldCounts{Track: 1, Disc: 1}) {
		t.Fatalf("counts = %+v", p.Counts)
	}
	if c.Line != 5 || c.Hash != "a3f" || c.Current != libraryTrack() {
		t.Fatalf("change lost its provenance: %+v", c)
	}
}

func TestDiffZeroingATrackNumberIsAChange(t *testing.T) {
	row := exportRow()
	row.Track = 0
	p := diffOne(row)
	if len(p.Changes) != 1 || p.Changes[0].Track == nil || *p.Changes[0].Track != 0 {
		t.Fatalf("clearing a track number must be an explicit change: %+v", p)
	}
}

func TestDiffGenreComparesCanonically(t *testing.T) {
	for _, genre := range []string{"Jazz; Cool", "Cool, Jazz", "Cool/Jazz", " Jazz ;Cool; Jazz "} {
		row := exportRow()
		row.Genre = genre
		p := diffOne(row)
		if len(p.Changes) != 0 || p.Unchanged != 1 {
			t.Fatalf("genre %q should be a no-op, got %+v", genre, p)
		}
	}
}

// Parse cleans every cell with Cell, so a library value whose quotes or
// padding are part of the tag must not come back from an untouched export as
// an edit.
func TestDiffUntouchedRoundTripIsUnchanged(t *testing.T) {
	cur := Current{
		Path:   "/Music/Bowie/heroes.mp3",
		Artist: "David Bowie ",
		Title:  `"Heroes"`,
		Album:  "'Low'",
		Genre:  `"Art Rock"`,
		Track:  3,
		Disc:   1,
	}
	doc := Document{
		Roots: []string{"/Music"},
		Rows: []Row{{
			Hash: "a3f", RootID: 1, RelPath: "Bowie/heroes.mp3",
			Artist: cur.Artist, Title: cur.Title, Album: cur.Album, Genre: cur.Genre,
			Track: cur.Track, Disc: cur.Disc,
		}},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, doc); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	f, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	p := Diff(f.Rows, map[string]Current{"a3f": cur})
	if len(p.Changes) != 0 || p.Unchanged != 1 {
		t.Fatalf("untouched round-trip should be a no-op, got %+v", p)
	}

	// A real edit to the same row still registers.
	row := f.Rows[0]
	row.Title = "Heroes (2017 Remaster)"
	p = Diff([]Row{row}, map[string]Current{"a3f": cur})
	if len(p.Changes) != 1 || p.Changes[0].Title == nil || *p.Changes[0].Title != row.Title {
		t.Fatalf("edited title should be a change, got %+v", p)
	}
	if c := p.Changes[0]; c.Artist != nil || c.Album != nil || c.Genre != nil {
		t.Fatalf("only the title changed, got %+v", c)
	}
}

func TestDiffUnchangedAndUnknown(t *testing.T) {
	rows := []Row{exportRow(), {Hash: "gone", RelPath: "Rock/b.mp3", Line: 6}}
	p := Diff(rows, map[string]Current{"a3f": libraryTrack()})
	if len(p.Changes) != 0 {
		t.Fatalf("got %d changes, want 0", len(p.Changes))
	}
	if p.Unchanged != 1 {
		t.Fatalf("Unchanged = %d, want 1", p.Unchanged)
	}
	if !reflect.DeepEqual(p.Unknown, []string{"gone"}) {
		t.Fatalf("Unknown = %v, want [gone]", p.Unknown)
	}
	if p.Moved != 0 {
		t.Fatalf("Moved = %d, want 0", p.Moved)
	}
}

func TestDiffMovedFileIsCountedNotSkipped(t *testing.T) {
	row := exportRow()
	row.RelPath = "Rock/old-name.mp3"
	row.Artist = "Miles Davis"
	p := diffOne(row)
	if len(p.Changes) != 1 || !p.Changes[0].PathMoved {
		t.Fatalf("a moved file must still apply: %+v", p)
	}
	if p.Moved != 1 {
		t.Fatalf("Moved = %d, want 1", p.Moved)
	}
}

func TestDiffMovedIgnoresARelocatedRoot(t *testing.T) {
	cur := libraryTrack()
	cur.Path = "/Volumes/Elsewhere/Rock/a.mp3"
	p := Diff([]Row{exportRow()}, map[string]Current{"a3f": cur})
	if p.Moved != 0 {
		t.Fatalf("Moved = %d, want 0: the path is still root-relative", p.Moved)
	}
}

func TestCanonicalGenre(t *testing.T) {
	if got := CanonicalGenre("Rock, Pop"); got != "Pop; Rock" {
		t.Fatalf("CanonicalGenre(%q) = %q", "Rock, Pop", got)
	}
	if got := CanonicalGenre(""); got != "" {
		t.Fatalf("CanonicalGenre(\"\") = %q", got)
	}
}
