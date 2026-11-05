package lyrics

import (
	"reflect"
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestParseSynced(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Line
	}{
		{
			name: "centiseconds",
			in:   "[00:12.34]Hello\n[00:15.00]World",
			want: []Line{{ms(12340), "Hello"}, {ms(15000), "World"}},
		},
		{
			name: "no fraction, one and three digits",
			in:   "[00:01]a\n[00:02.5]b\n[00:03.456]c",
			want: []Line{{ms(1000), "a"}, {ms(2500), "b"}, {ms(3456), "c"}},
		},
		{
			name: "colon hundredths and hours",
			in:   "[00:04:50]a\n[01:00:00.00]b",
			want: []Line{{ms(4500), "a"}, {time.Hour, "b"}},
		},
		{
			name: "minutes past an hour without hours",
			in:   "[75:00.00]late",
			want: []Line{{75 * time.Minute, "late"}},
		},
		{
			name: "several stamps on one line are sorted",
			in:   "[00:30.00]chorus again\n[00:10.00][01:02.00]chorus\n[00:20.00]verse",
			want: []Line{{ms(10000), "chorus"}, {ms(20000), "verse"}, {ms(30000), "chorus again"}, {ms(62000), "chorus"}},
		},
		{
			name: "metadata ignored and untimed text dropped",
			in:   "[ar:Someone]\n[ti:Song]\n[length: 03:20]\n[by:me]\ncredits line\n[00:01.00]sung",
			want: []Line{{ms(1000), "sung"}},
		},
		{
			name: "blank timed lines are gaps",
			in:   "[00:01.00]one\n[00:05.00]\n[00:09.00]two",
			want: []Line{{ms(1000), "one"}, {ms(5000), ""}, {ms(9000), "two"}},
		},
		{
			name: "enhanced word times stripped",
			in:   "[00:12.00]<00:12.00>Hello <00:12.50>bright <00:13.10>world<00:14.00>",
			want: []Line{{ms(12000), "Hello bright world"}},
		},
		{
			name: "bracketed text after a stamp is kept",
			in:   "[00:01.00][Chorus] la la",
			want: []Line{{ms(1000), "[Chorus] la la"}},
		},
		{
			name: "spaces between stamps and text",
			in:   "  [00:01.00] [00:03.00]   spaced   out  ",
			want: []Line{{ms(1000), "spaced out"}, {ms(3000), "spaced out"}},
		},
		{
			name: "CRLF and lone CR",
			in:   "[00:01.00]a\r\n[00:02.00]b\r[00:03.00]c\r\n",
			want: []Line{{ms(1000), "a"}, {ms(2000), "b"}, {ms(3000), "c"}},
		},
		{
			name: "malformed stamps are text",
			in:   "[00:01.00]ok\n[aa:bb]not a time\n[00:0x]also not",
			want: []Line{{ms(1000), "ok"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseString(tt.in)
			if got == nil || !got.Synced {
				t.Fatalf("want synced lyrics, got %+v", got)
			}
			if !reflect.DeepEqual(got.Lines, tt.want) {
				t.Fatalf("lines\n got %+v\nwant %+v", got.Lines, tt.want)
			}
		})
	}
}

func TestParseOffset(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []time.Duration
	}{
		{"positive shows earlier", "[offset:+500]\n[00:01.00]a\n[00:02.00]b", []time.Duration{ms(500), ms(1500)}},
		{"negative shows later", "[00:01.00]a\n[offset:-250]", []time.Duration{ms(1250)}},
		{"never before zero", "[offset:2000]\n[00:01.00]a\n[00:05.00]b", []time.Duration{0, ms(3000)}},
		{"bad offset ignored", "[offset:soon]\n[00:01.00]a", []time.Duration{ms(1000)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseString(tt.in)
			var at []time.Duration
			for _, l := range got.Lines {
				at = append(at, l.At)
			}
			if !reflect.DeepEqual(at, tt.want) {
				t.Fatalf("times = %v, want %v", at, tt.want)
			}
		})
	}
}

func TestParseUnsynced(t *testing.T) {
	got := ParseString("\n\n[ti:Song]\nFirst line\n<00:01.00>Second line\n\n\n\nNext stanza\n\n")
	if got == nil || got.Synced {
		t.Fatalf("want unsynced lyrics, got %+v", got)
	}
	want := []Line{{Text: "First line"}, {Text: "Second line"}, {Text: ""}, {Text: "Next stanza"}}
	if !reflect.DeepEqual(got.Lines, want) {
		t.Fatalf("lines\n got %+v\nwant %+v", got.Lines, want)
	}
	if got.Index(time.Minute) != -1 {
		t.Fatal("unsynced lyrics have no current line")
	}
}

func TestParseEmpty(t *testing.T) {
	for _, in := range []string{"", "\n\n", "[ar:Only]\n[ti:Tags]", "[00:01.00]\n[00:02.00]"} {
		if got := ParseString(in); got != nil {
			t.Errorf("ParseString(%q) = %+v, want nil", in, got)
		}
	}
}

func TestParseEncodings(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"utf-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, "[00:01.00]Café"...), "Café"},
		{"latin-1", []byte("[00:01.00]Caf\xe9 \x93quoted\x94"), "Café “quoted”"},
		{"utf-16 LE", []byte{0xFF, 0xFE, '[', 0, '0', 0, '0', 0, ':', 0, '0', 0, '1', 0, ']', 0, 'h', 0, 'i', 0}, "hi"},
		{"utf-16 BE", []byte{0xFE, 0xFF, 0, '[', 0, '0', 0, '0', 0, ':', 0, '0', 0, '1', 0, ']', 0, 'h', 0, 'i'}, "hi"},
		{"wide characters", []byte("[00:01.00]夜明けまで歌おう"), "夜明けまで歌おう"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.in)
			if got == nil || len(got.Lines) != 1 || got.Lines[0].Text != tt.want {
				t.Fatalf("Parse = %+v, want one line %q", got, tt.want)
			}
		})
	}
}

func TestIndexBoundaries(t *testing.T) {
	l := ParseString("[00:10.00]a\n[00:20.00]b\n[00:20.00]b again\n[00:30.00]c")
	tests := []struct {
		pos  time.Duration
		want int
	}{
		{0, -1},
		{ms(9999), -1},
		{ms(10000), 0},
		{ms(19999), 0},
		{ms(20000), 2}, // two lines at one time: the later one is current
		{ms(29999), 2},
		{ms(30000), 3},
		{time.Hour, 3},
	}
	for _, tt := range tests {
		if got := l.Index(tt.pos); got != tt.want {
			t.Errorf("Index(%v) = %d, want %d", tt.pos, got, tt.want)
		}
	}
	var none *Lyrics
	if none.Index(time.Second) != -1 || !none.Empty() {
		t.Fatal("nil lyrics have no lines")
	}
}

func TestSyntheticIsSyncedAndSteady(t *testing.T) {
	l := Synthetic()
	if !l.Synced || l.Empty() || l.Index(0) != 0 || l.Index(83*time.Second) != 10 {
		t.Fatalf("synthetic lyrics: synced %v, index at 1:23 = %d", l.Synced, l.Index(83*time.Second))
	}
}
