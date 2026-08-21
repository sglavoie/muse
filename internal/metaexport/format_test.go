package metaexport

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

// goldenDoc exercises every value shape the escaping has to survive: an empty
// artist, an embedded TAB, an embedded LF, a literal backslash, an emoji, and
// a row under no configured root.
func goldenDoc() Document {
	return Document{
		Roots:       []string{"/Users/seb/Music", "/Volumes/Archive/FLAC"},
		GeneratedAt: time.Date(2026, 8, 21, 14, 3, 11, 0, time.UTC),
		ViewLabel:   "Browse ▸ Albums ▸ Most Relaxing Classical",
		Rows: []Row{
			{
				Hash:    "a3f91c",
				RootID:  1,
				RelPath: "Classical/Most Relaxing V1/01 Sheep.mp3",
				Artist:  "",
				Title:   "01.\tSheep may safely graze - J.S. Bach",
				Album:   "Most Relaxing\nClassical",
				Genre:   `Classical\Baroque`,
			},
			{
				Hash:    "b7d20e",
				RootID:  2,
				RelPath: "Jazz/Kind of Blue/01 So What.flac",
				Artist:  "Miles Davis 🎺",
				Title:   "So What",
				Album:   "Kind of Blue",
				Genre:   "Jazz",
				Track:   1,
				Disc:    1,
			},
			{
				Hash:    "c001de",
				RootID:  0,
				RelPath: "/tmp/loose/track.mp3",
				Artist:  "Unrooted",
				Title:   "Loose\rEnd",
				Genre:   "",
				Track:   3,
			},
		},
	}
}

const goldenTSV = "# muse-export v1\n" +
	"# generated\t2026-08-21T14:03:11Z\n" +
	"# view\tBrowse ▸ Albums ▸ Most Relaxing Classical\n" +
	"# count\t3\n" +
	"# root\t1\t/Users/seb/Music\n" +
	"# root\t2\t/Volumes/Archive/FLAC\n" +
	"hash\troot\tpath\tartist\ttitle\talbum\tgenre\ttrack\tdisc\n" +
	"a3f91c\t1\tClassical/Most Relaxing V1/01 Sheep.mp3\t\t" +
	`01.\tSheep may safely graze - J.S. Bach` + "\t" +
	`Most Relaxing\nClassical` + "\t" + `Classical\\Baroque` + "\t0\t0\n" +
	"b7d20e\t2\tJazz/Kind of Blue/01 So What.flac\tMiles Davis 🎺\tSo What\tKind of Blue\tJazz\t1\t1\n" +
	"c001de\t0\t/tmp/loose/track.mp3\tUnrooted\t" + `Loose\rEnd` + "\t\t\t3\t0\n"

func TestEncodeGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, goldenDoc()); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := buf.String(); got != goldenTSV {
		t.Fatalf("Encode mismatch\n got: %q\nwant: %q", got, goldenTSV)
	}
	for i, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if strings.ContainsAny(line, "\r") {
			t.Fatalf("line %d contains a raw CR: %q", i+1, line)
		}
	}
}

func TestEncodeParseRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, goldenDoc()); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	f, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", f.Warnings)
	}
	want := goldenDoc().Rows
	for i := range want {
		want[i].Line = 8 + i // 6 preamble lines + header
	}
	if !reflect.DeepEqual(f.Rows, want) {
		t.Fatalf("rows mismatch\n got: %#v\nwant: %#v", f.Rows, want)
	}
	if !reflect.DeepEqual(f.Roots, map[int]string{1: "/Users/seb/Music", 2: "/Volumes/Archive/FLAC"}) {
		t.Fatalf("roots mismatch: %#v", f.Roots)
	}
}
