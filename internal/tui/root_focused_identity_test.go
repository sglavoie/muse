package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFocusedIdentityFollowsCursorWithoutUsingPlayback(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.statusBar.Title = "Different playing song"
	first := ansi.Strip(m.renderFocusedIdentity(4))
	m.library.Cursor++
	second := ansi.Strip(m.renderFocusedIdentity(4))
	if !strings.Contains(first, "Track A1") || !strings.Contains(second, "Track A2") || strings.Contains(second, "Different playing song") {
		t.Fatalf("%q then %q", first, second)
	}
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "Focused · Track A2") {
		t.Fatal(screen)
	}
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	if m.renderFocusedIdentity(4) != "" {
		t.Fatal("compact view should keep table space")
	}
	if hint := ansi.Strip(m.renderFooterHint()); !strings.Contains(hint, "i info") || !strings.Contains(hint, "enter enqueue") {
		t.Fatal(hint)
	}
}

func TestCompactMixHintsKeepBackAndHideUnavailableFilter(t *testing.T) {
	m := newRichFixture()
	m.ActiveView = viewMixes
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	if hint := ansi.Strip(m.renderFooterHint()); strings.Contains(hint, "/ filter") {
		t.Fatal(hint)
	}
	m.mixes.Level = 1
	m.mixes.TrackView.Index.BulkLoad(richLibraryTracks())
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	hint := ansi.Strip(m.renderFooterHint())
	for _, essential := range []string{"enter enqueue", "esc", "/ filter", "? help"} {
		if !strings.Contains(hint, essential) {
			t.Fatalf("missing %s: %s", essential, hint)
		}
	}
}

func TestFocusedIdentityWrapsOnlyIntoSpareRows(t *testing.T) {
	m := newNarrowFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	out := ansi.Strip(m.renderFocusedIdentity(4))
	if !strings.Contains(out, "Reasonable Column Width") || strings.Count(out, "\n") > 3 {
		t.Fatal(out)
	}
	for _, budget := range []int{0, 1, 2, 3, 4} {
		out = ansi.Strip(m.renderFocusedIdentity(budget))
		if budget < 2 && out != "" {
			t.Fatal("identity stole table space")
		}
		if out != "" && strings.Count(out, "\n")+1 > budget {
			t.Fatal("identity exceeded spare rows")
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > 80 {
				t.Fatal("identity exceeded terminal width")
			}
		}
	}
}
