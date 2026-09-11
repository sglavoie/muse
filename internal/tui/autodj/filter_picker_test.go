package autodj

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestEnterOpensThePickerOnTheFocusedField(t *testing.T) {
	p := focus(t, newTestPanel(true), rowExcludeGenres)

	p, action := press(t, p, "enter")
	if action != actionNone {
		t.Fatalf("enter on a filter row returned %v, want the panel to handle it", action)
	}
	if p.picker == nil {
		t.Fatal("enter did not open a picker")
	}
	if got := p.picker.title(); got != " Exclude genres " {
		t.Fatalf("picker title = %q", got)
	}
	if got := len(p.picker.options); got != 3 {
		t.Fatalf("picker offers %d options, want the 3 catalog genres", got)
	}
}

func TestPickerOffersOptionsCaseInsensitivelySorted(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "enter")

	var labels []string
	for _, opt := range p.picker.options {
		labels = append(labels, opt.label)
	}
	want := []string{"Ambient", "jazz", "Rock"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Fatalf("options = %v, want %v", labels, want)
	}
}

func TestPickerPreSelectsTheCurrentFilter(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeTags)
	p, _ = press(t, p, "enter")

	if _, on := p.picker.chosen["1"]; !on {
		t.Fatalf("chosen = %v, want the applied tag 1 pre-selected", p.picker.chosen)
	}
	if got := len(p.picker.chosen); got != 1 {
		t.Fatalf("chosen holds %d keys, want only the applied one", got)
	}
}

func TestPickerSearchesByTypingAndConfirms(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "enter")

	// "jazz" must be typeable: every letter belongs to the query, not to
	// navigation.
	p, _ = press(t, p, "j", "a", "z")
	if got := p.picker.query; got != "jaz" {
		t.Fatalf("query = %q, want the typed letters", got)
	}
	if got := len(p.picker.filtered); got != 1 {
		t.Fatalf("%d rows match %q, want 1", got, p.picker.query)
	}

	p, _ = press(t, p, "tab") // deselect the already-chosen jazz
	p, _ = press(t, p, "ctrl+u")
	if got := p.picker.query; got != "" {
		t.Fatalf("query = %q, want ctrl+u to clear it", got)
	}
	p, _ = press(t, p, "R", "o")
	p, _ = press(t, p, "tab") // select Rock
	p, _ = press(t, p, "enter")

	if p.picker != nil {
		t.Fatal("enter did not close the picker")
	}
	if got := p.Draft().Include.Genres; len(got) != 1 || got[0] != "Rock" {
		t.Fatalf("include genres = %v, want [Rock]", got)
	}
	if got := p.AppliedSettings().Include.Genres; len(got) != 1 || got[0] != "jazz" {
		t.Fatalf("applied genres = %v, want the picker to edit the draft only", got)
	}
}

func TestPickerBackspaceEditsTheQuery(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeArtists)
	p, _ = press(t, p, "enter")

	p, _ = press(t, p, "z", "o", "backspace")
	if got := p.picker.query; got != "z" {
		t.Fatalf("query = %q, want z", got)
	}
	p, _ = press(t, p, "backspace", "backspace")
	if got := p.picker.query; got != "" {
		t.Fatalf("query = %q, want backspace on an empty query to be harmless", got)
	}
	if got := len(p.picker.filtered); got != 2 {
		t.Fatalf("%d rows visible, want the whole artist catalog back", got)
	}
}

func TestPickerCancelDiscardsItsEdits(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "enter")
	p, _ = press(t, p, "tab") // toggle whatever is focused
	p, _ = press(t, p, "esc")

	if p.picker != nil {
		t.Fatal("esc did not close the picker")
	}
	if got := p.Draft().Include.Genres; len(got) != 1 || got[0] != "jazz" {
		t.Fatalf("include genres = %v, want the draft untouched by a cancelled picker", got)
	}
	if p.Dirty() {
		t.Fatal("a cancelled picker left the draft dirty")
	}
}

func TestPickerCancelDoesNotCloseThePanel(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "enter")

	closed, action := press(t, p, "esc")
	if action != actionNone {
		t.Fatalf("esc in the picker returned %v, want the panel to stay open", action)
	}
	if closed.picker != nil {
		t.Fatal("esc did not close the picker")
	}
}

func TestPickerNavigationWrapsWithoutStealingLetters(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "enter")

	p, _ = press(t, p, "up")
	if got := p.picker.cursor; got != 2 {
		t.Fatalf("cursor = %d, want it wrapped to the last row", got)
	}
	p, _ = press(t, p, "ctrl+n")
	if got := p.picker.cursor; got != 0 {
		t.Fatalf("cursor = %d, want ctrl+n to move down and wrap", got)
	}
	p, _ = press(t, p, "ctrl+j", "ctrl+k")
	if got := p.picker.cursor; got != 0 {
		t.Fatalf("cursor = %d, want the ctrl pair to cancel out", got)
	}
	if got := p.picker.query; got != "" {
		t.Fatalf("query = %q, want navigation keys to stay out of the search box", got)
	}
}

func TestPickerKeepsAStaleSelectionItCannotShow(t *testing.T) {
	p := newTestPanel(true)
	p.draft.Include.TagIDs = []int64{1, 42} // 42 is not in the catalog
	p = focus(t, p, rowIncludeTags)

	p, _ = press(t, p, "enter")
	p, _ = press(t, p, "down") // move onto "focus"
	p, _ = press(t, p, "tab")  // add it
	p, _ = press(t, p, "enter")

	got := p.Draft().Include.TagIDs
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 42 {
		t.Fatalf("include tags = %v, want [1 2 42]: the unresolvable reference must survive", got)
	}
}

func TestPickerToggleOnAnEmptyResultIsHarmless(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "enter")
	p, _ = press(t, p, "q", "q", "q")

	if got := len(p.picker.filtered); got != 0 {
		t.Fatalf("%d rows match a nonsense query, want none", got)
	}
	p, _ = press(t, p, "tab", "up", "down")
	p, _ = press(t, p, "enter")

	if got := p.Draft().Include.Genres; len(got) != 1 || got[0] != "jazz" {
		t.Fatalf("include genres = %v, want the selection unchanged", got)
	}
}

func TestPickerRenderShowsSearchSelectionAndSwatch(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeTags)
	p, _ = press(t, p, "enter")

	out := p.Render(80, 24, styles.NewStyles(true), nil, Preview{})
	if !strings.Contains(plain(out), "Include tags") {
		t.Fatalf("picker render has no title:\n%s", out)
	}
	if !strings.Contains(plain(out), "type to search") {
		t.Fatalf("picker render has no search line:\n%s", out)
	}
	if !strings.Contains(plain(out), "[x] ● chill") || !strings.Contains(plain(out), "[ ] ● focus") {
		t.Fatalf("picker render does not mark the selection and its swatch:\n%s", out)
	}
	if !strings.Contains(plain(out), "[ ] loud") {
		t.Fatalf("a tag with no color did not render:\n%s", out)
	}
	if strings.Contains(plain(out), "Discovery") {
		t.Fatalf("an open picker still shows the rows behind it:\n%s", out)
	}
}

func TestPickerFitsTheMinimumTerminal(t *testing.T) {
	p := newTestPanel(true)
	p.catalog.Genres = make([]string, 40)
	for i := range p.catalog.Genres {
		p.catalog.Genres[i] = string(rune('a'+i%26)) + "-genre"
	}
	p = focus(t, p, rowIncludeGenres)
	p, _ = press(t, p, "enter")

	// Walk to the far end of the list: every step must stay inside the box.
	for range len(p.picker.options) - 1 {
		p, _ = press(t, p, "down")
	}
	assertFitsBox(t, p.Render(minWidth, minOverlayHeight, styles.NewStyles(true), nil, Preview{}), minWidth, minOverlayHeight)

	start, end := p.picker.rowWindow(minOverlayHeight)
	if p.picker.cursor < start || p.picker.cursor >= end {
		t.Fatalf("cursor %d outside the visible window [%d,%d)", p.picker.cursor, start, end)
	}
}

func TestSettingsRoundTripThroughTheDraftStayValid(t *testing.T) {
	p := focus(t, newTestPanel(false), rowExcludeArtists)
	p, _ = press(t, p, "enter")
	p, _ = press(t, p, "tab") // toggle the first artist
	p, _ = press(t, p, "enter")

	if err := p.Draft().Validate(); err != nil {
		t.Fatalf("draft is invalid after a picker round trip: %v", err)
	}
	if got := p.Draft().Anchor; got != radio.AnchorSeed {
		t.Fatalf("anchor = %q, want the picker to leave it alone", got)
	}
}
