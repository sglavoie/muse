package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFilterSummaryDescribesParserWithoutChangingMatches(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"live a:Miles Davis g:Jazz", "Text: live · Artist: Miles Davis · Genre: Jazz"},
		{"a:Alice artist:Bob l:", "Artist: Bob"},
		{"artist:", "Empty prefixes: no text constraint"},
		{"普通 a:音楽", "Text: 普通 · Artist: 音楽"},
		{"plain text", ""},
	} {
		m := newModelWithTracks()
		m.library.Filter.Text = tc.query
		if got := m.filterQuerySummary(); got != tc.want {
			t.Fatalf("%q: summary %q, want %q", tc.query, got, tc.want)
		}
	}
	m := newModelWithTracks()
	m.showFooterHints = true
	m = typeText(pressSlash(m), "a:Alice")
	for _, width := range []int{60, 100, 160} {
		m.termWidth = width
		out := ansi.Strip(m.renderFooterHint())
		if !strings.Contains(out, "enter apply") || !strings.Contains(out, "Artist: Alice") || ansi.StringWidth(out) > width {
			t.Fatalf("width %d: %q", width, out)
		}
	}
	if len(m.library.Panel().VisibleTracks()) != 3 {
		t.Fatal("summary changed filtering")
	}
	m = pressEnter(m)
	if strings.Contains(ansi.Strip(m.renderFooterHint()), "Artist: Alice") {
		t.Fatal("editing summary survived commit")
	}
	m.ActiveView = viewQueue
	m.queue.Filter.Text = "g:Jazz"
	if !strings.Contains(m.filterQuerySummary(), "ignored in Queue") {
		t.Fatal("queue summary promised unsupported genre filtering")
	}
	m.ActiveView = viewPlaylists
	m.playlists.Filter.Text = "a:Alice"
	if m.filterQuerySummary() != "" {
		t.Fatal("literal filters should not advertise field filtering")
	}
}
