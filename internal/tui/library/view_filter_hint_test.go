package library

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestFilterBarHintShownWhenEmpty verifies that the empty-filter hint text
// is shown at TextSecondary weight (Zeigarnik: the unfinished input keeps
// drawing the eye) when the filter bar is active but no text has been
// typed yet.
func TestFilterBarHintShownWhenEmpty(t *testing.T) {
	idx := buildIndex(sampleTracks)
	secondaryStyle := styles.NewStyles(true).TextSecondary
	v := Panel{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v.Styles.TextSecondary = secondaryStyle
	v = v.ActivateFilter()
	got := v.Render()
	// The hint should contain the prefix shortcuts.
	if !strings.Contains(got, "a: artist") {
		t.Errorf("filter bar hint should show 'a: artist' when empty, got:\n%s", got)
	}
	if !strings.Contains(got, "t: title") {
		t.Errorf("filter bar hint should show 't: title' when empty, got:\n%s", got)
	}
	if !strings.Contains(got, "l: album") {
		t.Errorf("filter bar hint should show 'l: album' when empty, got:\n%s", got)
	}
	// The hint text should be wrapped in TextSecondary styling.
	hint := "a: artist  t: title  l: album  g: genre  aa: album artist"
	styledHint := styledText(secondaryStyle, hint)
	if ansi.Strip(styledHint) == styledHint {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}
	if !strings.Contains(got, styledHint) {
		t.Errorf("filter bar hint should use TextSecondary styling, got:\n%q", got)
	}
}

// TestFilterBarHintDisappearsOnInput verifies that the dim hint text is
// replaced by the actual filter text once the user starts typing.
func TestFilterBarHintDisappearsOnInput(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := Panel{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	got := v.Render()
	if strings.Contains(got, "a: artist") {
		t.Errorf("filter bar hint should not appear once text is entered, got:\n%s", got)
	}
	if !strings.Contains(ansi.Strip(got), `filter "`) {
		t.Errorf("filter chunk should show typed text, got:\n%s", got)
	}
}
