package metaexport

import (
	"errors"
	"strings"
	"testing"
)

const testHeader = "hash\troot\tpath\tartist\ttitle\talbum\tgenre\ttrack\tdisc"

func doc(lines ...string) string {
	preamble := []string{
		"# muse-export v1",
		"# generated\t2026-08-21T14:03:11Z",
		"# root\t1\t/Music",
	}
	return strings.Join(append(preamble, lines...), "\n") + "\n"
}

func parseOK(t *testing.T, src string) *File {
	t.Helper()
	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func TestParseLenient(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		wantRow      Row
		wantWarnings int
	}{
		{
			name: "plain row",
			src: doc(testHeader,
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
		},
		{
			name: "fenced block",
			src: "```tsv\n" + doc(testHeader,
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2") + "```\n",
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 6},
		},
		{
			name: "reordered columns",
			src: doc("title\thash\tdisc\ttrack\tgenre\talbum\tartist\tpath\troot",
				"So What\ta3f\t2\t1\tJazz\tKind of Blue\tMiles\tRock/a.mp3\t1"),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
		},
		{
			name: "extra column ignored with a warning",
			src: doc(testHeader+"\tnotes",
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2\tfixed the artist"),
			wantRow:      Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
			wantWarnings: 1,
		},
		{
			name: "quotes and invisible padding stripped",
			src: doc(testHeader,
				"\"a3f\"\t1\t'Rock/a.mp3'\t\u00a0Miles\u200b\t“So What”\tKind of Blue\tJazz\t\" 1 \"\t2"),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
		},
		{
			name: "prose and blank lines after the header",
			src: doc(testHeader,
				"",
				"# here are the corrected rows",
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 7},
		},
		{
			name: "crlf endings and no trailing newline",
			src: strings.TrimSuffix(strings.ReplaceAll(doc(testHeader,
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"), "\n", "\r\n"), "\r\n"),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
		},
		{
			name: "minor version bump is readable",
			src: strings.Replace(doc(testHeader,
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"), "v1", "v1.4", 1),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
		},
		{
			name: "root 0 keeps an absolute path",
			src: doc(testHeader,
				"a3f\t0\t/tmp/loose.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"),
			wantRow: Row{Hash: "a3f", RootID: 0, RelPath: "/tmp/loose.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
		},
		{
			name: "unparsable root warns but keeps the row",
			src: doc(testHeader,
				"a3f\tmain\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"),
			wantRow:      Row{Hash: "a3f", RootID: 0, RelPath: "Rock/a.mp3", Artist: "Miles", Title: "So What", Album: "Kind of Blue", Genre: "Jazz", Track: 1, Disc: 2, Line: 5},
			wantWarnings: 1,
		},
		{
			name: "empty values stay empty",
			src: doc(testHeader,
				"a3f\t1\tRock/a.mp3\t\t\t\t\t\t"),
			wantRow: Row{Hash: "a3f", RootID: 1, RelPath: "Rock/a.mp3", Line: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseOK(t, tt.src)
			if len(f.Rows) != 1 {
				t.Fatalf("got %d rows, want 1: %#v", len(f.Rows), f.Rows)
			}
			if f.Rows[0] != tt.wantRow {
				t.Fatalf("row = %#v\nwant %#v", f.Rows[0], tt.wantRow)
			}
			if len(f.Warnings) != tt.wantWarnings {
				t.Fatalf("got %d warnings %v, want %d", len(f.Warnings), f.Warnings, tt.wantWarnings)
			}
			if f.Skipped != 0 {
				t.Fatalf("Skipped = %d, want 0: a warning is not a dropped row", f.Skipped)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want error
	}{
		{
			name: "bad magic",
			src:  "hash\troot\n",
			want: ErrBadMagic,
		},
		{
			name: "empty input",
			src:  "",
			want: ErrBadMagic,
		},
		{
			name: "unknown major version",
			src: strings.Replace(doc(testHeader,
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"), "v1", "v2", 1),
			want: ErrUnsupportedVersion,
		},
		{
			name: "header missing a required column",
			src: doc("hash\troot\tpath\tartist\ttitle\talbum\ttrack\tdisc",
				"a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\t1\t2"),
			want: ErrMissingColumn,
		},
		{
			name: "zero data rows",
			src:  doc(testHeader),
			want: ErrNoRows,
		},
		{
			name: "every row skipped is still zero data rows",
			src:  doc(testHeader, "\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"),
			want: ErrNoRows,
		},
		{
			name: "markdown table",
			src: doc("| hash | root | path | artist | title | album | genre | track | disc |",
				"| a3f | 1 | Rock/a.mp3 | Miles | So What | Kind of Blue | Jazz | 1 | 2 |"),
			want: ErrMarkdownTable,
		},
		{
			name: "markdown table pasted without a preamble",
			src:  "| hash | root |\n| --- | --- |\n",
			want: ErrMarkdownTable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(tt.src))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse err = %v, want %v", err, tt.want)
			}
			if f != nil {
				t.Fatalf("Parse returned a file alongside the error: %#v", f)
			}
		})
	}
}

func TestParseSkipsBadRows(t *testing.T) {
	good := "a3f\t1\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2"
	tests := []struct {
		name string
		row  string
		warn string
	}{
		{name: "too few fields", row: "b7d\t1\tRock/b.mp3\tMiles", warn: "4 fields, want 9"},
		{name: "too many fields", row: good + "\textra", warn: "10 fields, want 9"},
		{name: "empty hash", row: "\t1\tRock/b.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2", warn: "empty hash"},
		{name: "unparsable track", row: "b7d\t1\tRock/b.mp3\tMiles\tSo What\tKind of Blue\tJazz\tone\t2", warn: "track"},
		{name: "unparsable disc", row: "b7d\t1\tRock/b.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\tB", warn: "disc"},
		{name: "invalid escape", row: `b7d` + "\t1\tRock/b.mp3\tMiles\t" + `So \What` + "\tKind of Blue\tJazz\t1\t2", warn: "invalid escape"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseOK(t, doc(testHeader, good, tt.row))
			if len(f.Rows) != 1 || f.Rows[0].Hash != "a3f" {
				t.Fatalf("bad row was not skipped: %#v", f.Rows)
			}
			if len(f.Warnings) != 1 {
				t.Fatalf("got %d warnings %v, want 1", len(f.Warnings), f.Warnings)
			}
			if f.Skipped != 1 {
				t.Fatalf("Skipped = %d, want 1", f.Skipped)
			}
			if w := f.Warnings[0]; w.Line != 6 || !strings.Contains(w.Msg, tt.warn) {
				t.Fatalf("warning = %q, want line 6 mentioning %q", w.String(), tt.warn)
			}
		})
	}
}

func TestParseDuplicateHashLastWins(t *testing.T) {
	f := parseOK(t, doc(testHeader,
		"a3f\t1\tRock/a.mp3\tMiles\tfirst\tKind of Blue\tJazz\t1\t2",
		"b7d\t1\tRock/b.mp3\tBill\tother\tKind of Blue\tJazz\t3\t1",
		"a3f\t1\tRock/a.mp3\tMiles\tsecond\tKind of Blue\tJazz\t1\t2"))
	if len(f.Rows) != 2 {
		t.Fatalf("got %d rows, want 2: %#v", len(f.Rows), f.Rows)
	}
	if f.Rows[0].Title != "second" {
		t.Fatalf("row 0 title = %q, want the later duplicate %q", f.Rows[0].Title, "second")
	}
	if f.Rows[1].Hash != "b7d" {
		t.Fatalf("dedupe reordered the rows: %#v", f.Rows)
	}
	if len(f.Warnings) != 1 || !strings.Contains(f.Warnings[0].Msg, "duplicate hash") {
		t.Fatalf("warnings = %v, want one duplicate-hash warning", f.Warnings)
	}
}

func TestParseRoots(t *testing.T) {
	f := parseOK(t, "# muse-export v1\n"+
		"# root\t1\t/Music\n"+
		"# root\t2\t/Volumes/Odd\\tName\n"+
		"# root\tzero\t/nope\n"+
		"# root\t3\n"+
		"# future\tsomething new\n"+
		testHeader+"\n"+
		"a3f\t2\tRock/a.mp3\tMiles\tSo What\tKind of Blue\tJazz\t1\t2\n")
	if f.Version != Version {
		t.Fatalf("Version = %d, want %d", f.Version, Version)
	}
	if f.Roots[1] != "/Music" || f.Roots[2] != "/Volumes/Odd\tName" || len(f.Roots) != 2 {
		t.Fatalf("Roots = %#v", f.Roots)
	}
	if len(f.Warnings) != 2 {
		t.Fatalf("got %d warnings %v, want 2 (bad id, missing path)", len(f.Warnings), f.Warnings)
	}
}
