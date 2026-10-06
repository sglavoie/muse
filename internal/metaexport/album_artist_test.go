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
