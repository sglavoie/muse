package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func lyricsLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "line " + itoa(i)
	}
	return out
}

func plainRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = ansi.Strip(r)
	}
	return out
}

func TestLyricsSectionFollowsCurrentLine(t *testing.T) {
	l := &NowPlayingLyrics{Lines: lyricsLines(20), Synced: true, Current: 10, Anchor: 10, Fade: 1}
	got := plainRows(l.section(40, 6, newTestStyles()))
	want := []string{"Lyrics", "line 8", "line 9", "line 10", "line 11", "line 12", "line 13"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("window = %q, want %q", got, want)
	}
	// Near the start and end the window stops at the edges.
	l.Current, l.Anchor = 0, 0
	if got := plainRows(l.section(40, 3, newTestStyles())); got[1] != "line 0" {
		t.Fatalf("start: %q", got)
	}
	l.Current, l.Anchor = 19, 19
	if got := plainRows(l.section(40, 3, newTestStyles())); got[3] != "line 19" || len(got) != 4 {
		t.Fatalf("end: %q", got)
	}
	// Before the first line nothing is highlighted and the window starts at
	// the top.
	l.Current, l.Anchor = -1, -1
	if got := plainRows(l.section(40, 3, newTestStyles())); got[1] != "line 0" {
		t.Fatalf("before the first line: %q", got)
	}
}

func TestLyricsSectionStylesByRole(t *testing.T) {
	s := newTestStyles()
	l := &NowPlayingLyrics{Lines: []string{"sung", "now", "next", "later"}, Synced: true, Current: 1, Anchor: 1, Fade: 1}
	rows := l.section(40, 4, s)
	if rows[2] != s.AccentMark.Bold(true).Render("now") {
		t.Fatalf("current line should be bold accent: %q", rows[2])
	}
	if rows[1] != s.Dim.Render("sung") || rows[3] != s.TextSecondary.Render("next") || rows[4] != s.TextTertiary.Render("later") {
		t.Fatalf("roles: %q", rows)
	}
	// Mid-fade the current line is not yet the full accent.
	l.Fade = 0.3
	if rows := l.section(40, 4, s); rows[2] == s.AccentMark.Bold(true).Render("now") {
		t.Fatal("a fading highlight should differ from the settled one")
	}
}

func TestLyricsSectionWrapsCurrentAndTruncatesOthers(t *testing.T) {
	long := "夜明けまで歌おう — sing until the morning light comes over the hill"
	l := &NowPlayingLyrics{Lines: []string{long, long, long}, Synced: true, Current: 1, Anchor: 1, Fade: 1}
	rows := plainRows(l.section(20, 4, newTestStyles()))
	if len(rows) != 5 {
		t.Fatalf("rows = %q", rows)
	}
	for _, r := range rows {
		if w := ansi.StringWidth(r); w > 20 {
			t.Fatalf("row %q is %d cells wide", r, w)
		}
	}
	if !strings.HasSuffix(rows[1], "…") || !strings.HasPrefix(rows[2], "夜明け") || !strings.HasSuffix(rows[3], "…") {
		t.Fatalf("the current line wraps to two rows, the others truncate: %q", rows)
	}
}

func TestLyricsSectionShowsGapAndUnsynced(t *testing.T) {
	l := &NowPlayingLyrics{Lines: []string{"a", "", "b"}, Synced: true, Current: 1, Anchor: 1, Fade: 1}
	if rows := plainRows(l.section(20, 3, newTestStyles())); rows[2] != lyricsGap {
		t.Fatalf("an instrumental gap shows a note while it plays: %q", rows)
	}
	u := &NowPlayingLyrics{Lines: lyricsLines(10), Current: -1, Anchor: 4}
	if rows := plainRows(u.section(20, 3, newTestStyles())); rows[1] != "line 4" || len(rows) != 4 {
		t.Fatalf("unsynced lyrics start at the scroll position: %q", rows)
	}
	u.Anchor = 99
	if rows := plainRows(u.section(20, 3, newTestStyles())); rows[3] != "line 9" {
		t.Fatalf("scrolling stops at the end: %q", rows)
	}
}

func lyricsOverlay(lyr *NowPlayingLyrics) NowPlayingOverlay {
	return NowPlayingOverlay{
		Artist: "Artist", Title: "Song", Album: "Album", State: playback.StatePlaying,
		Details: "3:00 · Rock", Lyrics: lyr,
		UpNext: []UpNextSong{{Title: "Next one"}, {Title: "After that"}},
	}
}

func TestNowPlayingLyricsLayoutAtSizes(t *testing.T) {
	lyr := &NowPlayingLyrics{Lines: lyricsLines(30), Synced: true, Current: 12, Anchor: 12, Fade: 1}
	tests := []struct {
		w, h       int
		rows       int // lyric rows expected under the heading
		wantUpNext bool
	}{
		{60, 15, 3, false},
		{80, 24, 6, true},
		{120, 36, 9, true},
	}
	for _, tt := range tests {
		out := ansi.Strip(lyricsOverlay(lyr).Render(tt.w, tt.h, newTestStyles()))
		lines := strings.Split(out, "\n")
		if len(lines) != tt.h {
			t.Fatalf("%dx%d: %d lines", tt.w, tt.h, len(lines))
		}
		if !strings.Contains(out, "line 12") || !strings.Contains(out, "Lyrics") {
			t.Fatalf("%dx%d: lyrics missing:\n%s", tt.w, tt.h, out)
		}
		count := 0
		for _, line := range lines {
			if strings.Contains(line, "line ") {
				count++
			}
			if ansi.StringWidth(line) > tt.w {
				t.Fatalf("%dx%d: line too wide: %q", tt.w, tt.h, line)
			}
		}
		if count != tt.rows {
			t.Fatalf("%dx%d: %d lyric rows, want %d:\n%s", tt.w, tt.h, count, tt.rows, out)
		}
		if strings.Contains(out, "Up next") != tt.wantUpNext {
			t.Fatalf("%dx%d: up next shown = %v:\n%s", tt.w, tt.h, !tt.wantUpNext, out)
		}
		if !strings.Contains(lines[len(lines)-1], "esc close") {
			t.Fatalf("%dx%d: hint missing: %q", tt.w, tt.h, lines[len(lines)-1])
		}
	}
}

func TestNowPlayingLyricsOrderAndHint(t *testing.T) {
	lyr := &NowPlayingLyrics{Lines: lyricsLines(5), Synced: true, Current: 0, Anchor: 0, Fade: 1}
	out := ansi.Strip(lyricsOverlay(lyr).Render(120, 36, newTestStyles()))
	details, lyrics, next := strings.Index(out, "3:00 · Rock"), strings.Index(out, "Lyrics"), strings.Index(out, "Up next")
	if details < 0 || lyrics < details || next < lyrics {
		t.Fatalf("lyrics sit between the details and up next:\n%s", out)
	}
	if !strings.Contains(out, "L hide lyrics") {
		t.Fatalf("hint should offer to hide lyrics:\n%s", out)
	}
	lyr.Hidden = true
	out = ansi.Strip(lyricsOverlay(lyr).Render(120, 36, newTestStyles()))
	if strings.Contains(out, "line 0") || !strings.Contains(out, "L lyrics") {
		t.Fatalf("hidden lyrics leave only the hint:\n%s", out)
	}
	out = ansi.Strip(lyricsOverlay(nil).Render(120, 36, newTestStyles()))
	if strings.Contains(out, "Lyrics") || strings.Contains(out, "lyrics") {
		t.Fatalf("a song without lyrics shows no trace of them:\n%s", out)
	}
	unsynced := &NowPlayingLyrics{Lines: lyricsLines(5), Current: -1}
	if out := ansi.Strip(lyricsOverlay(unsynced).Render(120, 36, newTestStyles())); !strings.Contains(out, "↑/↓ lyrics") {
		t.Fatalf("unsynced lyrics offer scrolling:\n%s", out)
	}
}
