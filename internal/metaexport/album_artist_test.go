package metaexport

import (
	"bytes"
	"strings"
	"testing"
)

func TestAlbumArtistRoundTripAndLegacyImport(t *testing.T) {
	legacy := "# muse-export v1\nhash\troot\tpath\tartist\ttitle\talbum\tgenre\ttrack\tdisc\nh\t1\ta.flac\tSinger\tSong\tAlbum\tJazz\t1\t1\n"
	current := map[string]Current{"h": {Artist: "Singer", Title: "Song", Album: "Album", Genre: "Jazz", Track: 1, Disc: 1, AlbumArtist: "Various Artists"}}
	parsed, err := Parse(strings.NewReader(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if plan := Diff(parsed.Rows, current); len(plan.Changes) != 0 {
		t.Fatalf("legacy import changed album artist: %+v", plan)
	}
	for _, value := range []string{"Various Artists", "New Artist", ""} {
		var buf bytes.Buffer
		row := parsed.Rows[0]
		row.AlbumArtist = value
		if err := Encode(&buf, Document{Rows: []Row{row}}); err != nil {
			t.Fatal(err)
		}
		file, err := Parse(&buf)
		if err != nil {
			t.Fatal(err)
		}
		plan := Diff(file.Rows, current)
		if value == "Various Artists" {
			if len(plan.Changes) != 0 {
				t.Fatal("unchanged export changed metadata")
			}
			continue
		}
		if len(plan.Changes) != 1 || plan.Counts.AlbumArtist != 1 || plan.Changes[0].AlbumArtist == nil || *plan.Changes[0].AlbumArtist != value {
			t.Fatalf("bad album artist diff: %+v", plan)
		}
	}
}

// An empty album artist is exported as a trailing tab, which chat clients and
// editors strip. The shortened row must still import, and clearing the album
// artist that way must still apply.
func TestAlbumArtistEmptySurvivesTrailingWhitespaceStrip(t *testing.T) {
	var buf bytes.Buffer
	rows := []Row{{Hash: "h", RootID: 1, RelPath: "a.flac", Artist: "Singer", Title: "Song", Track: 1, Disc: 1}}
	if err := Encode(&buf, Document{Roots: []string{"/Music"}, Rows: rows}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	file, err := Parse(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Rows) != 1 || file.Skipped != 0 {
		t.Fatalf("rows=%+v skipped=%d warnings=%v, want the stripped row kept", file.Rows, file.Skipped, file.Warnings)
	}
	if row := file.Rows[0]; !row.AlbumArtistPresent || row.AlbumArtist != "" {
		t.Fatalf("album artist = %q present=%v, want empty and present", row.AlbumArtist, row.AlbumArtistPresent)
	}
	current := map[string]Current{"h": {Artist: "Singer", Title: "Song", Track: 1, Disc: 1, AlbumArtist: "Various Artists"}}
	if plan := Diff(file.Rows, current); plan.Counts.AlbumArtist != 1 {
		t.Fatalf("clearing album artist via a stripped row was not applied: %+v", plan)
	}

	// A row cut short before a required column is still rejected.
	short := "# muse-export v1\n" + strings.Join(Columns, "\t") + "\nh\t1\ta.flac\tSinger\tSong\tAlbum\tJazz\t1\n"
	file, err = Parse(strings.NewReader(short + "ok\t1\tb.flac\tA\tT\tB\tG\t1\t1\tX\n"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Skipped != 1 || len(file.Rows) != 1 || file.Rows[0].Hash != "ok" {
		t.Fatalf("rows=%+v skipped=%d, want the row missing disc skipped", file.Rows, file.Skipped)
	}
}
