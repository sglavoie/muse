package strip

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRender_QuietForm(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 80 {
		t.Fatalf("width = %d, want 80 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if !strings.HasPrefix(plain, "Library") {
		t.Errorf("plain should start with view label, got %q", plain)
	}
	if !strings.HasSuffix(plain, "3") {
		t.Errorf("plain should end with total count, got %q", plain)
	}
	if strings.Contains(plain, "·") {
		t.Errorf("quiet form should not show divider, got %q", plain)
	}
	if strings.Contains(plain, "sort ") || strings.Contains(plain, "filter ") {
		t.Errorf("quiet form should not show sort/filter chunks, got %q", plain)
	}
}

func TestRender_FullForm(t *testing.T) {
	s := styles.NewStyles(true)
	sort := SortChunk{Label: "Artist", Indicator: "▲"}
	filter := FilterChunk{Text: "alice"}
	counts := CountsChunk{Filtered: 2, Total: 7, Filtering: true}

	got := Render(80, []string{"Library"}, sort, filter, counts, nil, "", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 80 {
		t.Fatalf("width = %d, want 80 (plain=%q)", ansi.StringWidth(got), plain)
	}
	for _, want := range []string{"Library", "·", "sort Artist▲", `filter "alice"`, "2/7"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in %q", want, plain)
		}
	}
	libIdx := strings.Index(plain, "Library")
	sortIdx := strings.Index(plain, "sort Artist▲")
	filterIdx := strings.Index(plain, `filter "alice"`)
	countsIdx := strings.LastIndex(plain, "2/7")
	if !(libIdx < sortIdx && sortIdx < filterIdx && filterIdx < countsIdx) {
		t.Errorf("expected order Library < sort < filter < counts, got positions %d %d %d %d in %q",
			libIdx, sortIdx, filterIdx, countsIdx, plain)
	}
}

func TestRender_FilterActiveWithText(t *testing.T) {
	s := styles.NewStyles(true)
	// Simulate a textinput.View() output: the first character is wrapped in
	// reverse-video to stand in for the cursor.
	inputView := lipgloss.NewStyle().Reverse(true).Render("a") + "lice"
	filter := FilterChunk{Active: true, Text: "alice", InputView: inputView}

	got := Render(80, []string{"Library"}, SortChunk{}, filter, CountsChunk{Total: 7}, nil, "", s)
	if !strings.Contains(got, inputView) {
		t.Errorf("expected raw InputView preserved (with its own ANSI), got %q", got)
	}
	plain := ansi.Strip(got)
	// The textinput's rendered width counts as the value width — total strip
	// is still padded to width.
	if ansi.StringWidth(got) != 80 {
		t.Fatalf("width = %d, want 80 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if !strings.Contains(plain, `filter "alice"`) {
		t.Errorf("expected filter %q in plain %q", `filter "alice"`, plain)
	}
}

func TestRender_FilterActiveEmptyShowsHint(t *testing.T) {
	s := styles.NewStyles(true)
	hint := s.Dim.Render("a: artist  t: title")
	filter := FilterChunk{Active: true, Hint: hint}

	got := Render(80, []string{"Library"}, SortChunk{}, filter, CountsChunk{Total: 7}, nil, "", s)
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "a: artist") {
		t.Errorf("expected hint placeholder in active+empty filter chunk, got %q", plain)
	}
	if !strings.Contains(plain, `filter "`) {
		t.Errorf("expected filter prefix, got %q", plain)
	}
}

func TestRender_NarrowDropsCounts(t *testing.T) {
	s := styles.NewStyles(true)
	sort := SortChunk{Label: "Artist", Indicator: "▲"}
	filter := FilterChunk{Text: "alice"}
	counts := CountsChunk{Filtered: 2, Total: 7, Filtering: true}

	// Width just enough for view + sort + filter but not counts + gap.
	// "Library · sort Artist▲ · filter \"alice\"" = 7 + 3 + 12 + 3 + 14 = 39
	// Drop counts at 40 (no room for gap + "2/7").
	got := Render(40, []string{"Library"}, sort, filter, counts, nil, "", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 40 {
		t.Fatalf("width = %d, want 40 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if strings.Contains(plain, "2/7") {
		t.Errorf("narrow form should drop counts, but plain=%q", plain)
	}
	for _, want := range []string{"Library", "sort Artist▲", `filter "alice"`} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in %q (counts-dropped form should keep this)", want, plain)
		}
	}
}

func TestRender_CountsSuppressedWhenTotalZeroEvenIfFiltering(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(40, []string{"Library"}, SortChunk{}, FilterChunk{},
		CountsChunk{Filtered: 3, Total: 0, Filtering: true}, nil, "", s)
	plain := ansi.Strip(got)

	if strings.Contains(plain, "3/0") || strings.Contains(plain, "0/0") {
		t.Errorf("counts should be suppressed when total is zero, got %q", plain)
	}
}

func TestRender_NarrowerDropsSort(t *testing.T) {
	s := styles.NewStyles(true)
	sort := SortChunk{Label: "Artist", Indicator: "▲"}
	filter := FilterChunk{Text: "alice"}
	counts := CountsChunk{Filtered: 2, Total: 7, Filtering: true}

	// Width just enough for view + filter, not for sort.
	// "Library · filter \"alice\"" = 7 + 3 + 14 = 24
	got := Render(24, []string{"Library"}, sort, filter, counts, nil, "", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 24 {
		t.Fatalf("width = %d, want 24 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if strings.Contains(plain, "sort Artist") {
		t.Errorf("narrower form should drop sort, but plain=%q", plain)
	}
	if !strings.Contains(plain, `filter "alice"`) {
		t.Errorf("narrower form should keep filter, got %q", plain)
	}
}

func TestRender_NarrowestTruncatesFilter(t *testing.T) {
	s := styles.NewStyles(true)
	filter := FilterChunk{Text: "averyverylongfilterquery"}
	counts := CountsChunk{Total: 7}

	// Width 20: "Library · filter \"averyverylongfilterquery\"" overflows; we
	// expect the filter value to be truncated with "…" while the prefix and
	// closing quote are preserved.
	got := Render(20, []string{"Library"}, SortChunk{}, filter, counts, nil, "", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 20 {
		t.Fatalf("width = %d, want 20 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if !strings.Contains(plain, `filter "`) {
		t.Errorf("filter prefix should survive truncation, got %q", plain)
	}
	if !strings.HasSuffix(strings.TrimRight(plain, " "), `"`) {
		t.Errorf("filter closing quote should survive truncation, got %q", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("expected truncation ellipsis '…' in narrowest form, got %q", plain)
	}
	if strings.Contains(plain, "averyverylongfilterquery") {
		t.Errorf("filter value should be truncated, got full value in %q", plain)
	}
}

func TestRender_ZeroWidth(t *testing.T) {
	s := styles.NewStyles(true)
	if got := Render(0, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s); got != "" {
		t.Errorf("Render(0) = %q, want empty", got)
	}
}

func TestRender_EmptyBreadcrumb(t *testing.T) {
	s := styles.NewStyles(true)
	if got := Render(80, nil, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s); got != "" {
		t.Errorf("Render(empty breadcrumb) = %q, want empty", got)
	}
}

func TestRender_BreadcrumbChain(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Artists", "Brian Eno", "Heroes"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 7}, nil, "esc ←", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 80 {
		t.Fatalf("width = %d, want 80 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if !strings.Contains(plain, "Artists › Brian Eno › Heroes") {
		t.Errorf("expected breadcrumb chain in %q", plain)
	}
	if !strings.Contains(plain, "esc ←") {
		t.Errorf("expected esc hint in %q", plain)
	}
	if !strings.Contains(plain, "7") {
		t.Errorf("expected counts in %q", plain)
	}
	// Order: breadcrumb < counts < hint.
	bcIdx := strings.Index(plain, "Artists › Brian Eno › Heroes")
	countsIdx := strings.LastIndex(plain, "7")
	hintIdx := strings.LastIndex(plain, "esc ←")
	if !(bcIdx >= 0 && bcIdx < countsIdx && countsIdx < hintIdx) {
		t.Errorf("expected order breadcrumb < counts < hint, got %d %d %d in %q",
			bcIdx, countsIdx, hintIdx, plain)
	}
}

func TestRender_HintDropsFirst(t *testing.T) {
	s := styles.NewStyles(true)
	// Width tight enough that hint must drop but counts survive.
	// "Artists › Brian Eno · 2/7" = 7 + 3 + 9 + 1 + 3 = 23.
	// hint " esc ←" adds 7. Width 27 keeps counts (24) but cannot fit
	// the hint (would need 30).
	got := Render(27, []string{"Artists", "Brian Eno"}, SortChunk{}, FilterChunk{}, CountsChunk{Filtered: 2, Total: 7, Filtering: true}, nil, "esc ←", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 27 {
		t.Fatalf("width = %d, want 27 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if strings.Contains(plain, "esc") {
		t.Errorf("hint should have dropped first, got %q", plain)
	}
	if !strings.Contains(plain, "2/7") {
		t.Errorf("counts should survive, got %q", plain)
	}
	if !strings.Contains(plain, "Brian Eno") {
		t.Errorf("breadcrumb should survive, got %q", plain)
	}
}

func TestRender_BreadcrumbTruncatesLastCrumb(t *testing.T) {
	s := styles.NewStyles(true)
	// Width small enough that the last crumb cannot fully fit.
	got := Render(22, []string{"Artists", "A Very Long Artist Name"}, SortChunk{}, FilterChunk{}, CountsChunk{}, nil, "", s)
	plain := ansi.Strip(got)

	if ansi.StringWidth(got) != 22 {
		t.Fatalf("width = %d, want 22 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if !strings.Contains(plain, "Artists › ") {
		t.Errorf("parent crumb should remain intact, got %q", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("expected ellipsis on truncated last crumb, got %q", plain)
	}
	if strings.Contains(plain, "A Very Long Artist Name") {
		t.Errorf("last crumb should be truncated, got %q", plain)
	}
}

func TestRender_SingleCrumbAtTextSecondary(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s)
	secondary := s.TextSecondary.Render("Library")
	if !strings.Contains(got, secondary) {
		t.Errorf("single-crumb form should render label at TextSecondary; got %q", got)
	}
	dimmed := s.Dim.Render("Library")
	if strings.Contains(got, dimmed) {
		t.Errorf("single-crumb form must no longer render at Dim; got %q", got)
	}
}

func TestRender_MultiCrumbLastIsPrimaryParentsSecondary(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Artists", "Brian Eno"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "esc ←", s)
	dimmedRoot := s.Dim.Render("Artists")
	if strings.Contains(got, dimmedRoot) {
		t.Errorf("multi-crumb breadcrumb should not dim parent crumb; got %q", got)
	}
	secRoot := s.TextSecondary.Render("Artists")
	if !strings.Contains(got, secRoot) {
		t.Errorf("multi-crumb breadcrumb should render parent crumb at TextSecondary; got %q", got)
	}
	priLast := s.TextPrimary.Render("Brian Eno")
	if !strings.Contains(got, priLast) {
		t.Errorf("multi-crumb breadcrumb should render last crumb at TextPrimary; got %q", got)
	}
}

func TestRender_SelectionChunkRendered(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 7}, &SelectionChunk{Count: 3}, "", s)
	plain := ansi.Strip(got)
	if ansi.StringWidth(got) != 80 {
		t.Fatalf("width = %d, want 80 (plain=%q)", ansi.StringWidth(got), plain)
	}
	if !strings.Contains(plain, "◉ 3 selected") {
		t.Errorf("expected selection chip in %q", plain)
	}
	// The chip is rendered through a single combined fg+bg style so the
	// accent background carries across the whole chip; a nested inner
	// Render would emit an ANSI reset and drop the background mid-chip.
	chipStyle := s.Accent.Foreground(s.TextPrimary.GetForeground())
	chip := chipStyle.Render("◉ 3 selected")
	if !strings.Contains(got, chip) {
		t.Errorf("selection chip should render via a single fg+bg style; got %q", got)
	}
	// Guard against the documented nested-Render regression (bug hotspot #8).
	nested := s.Accent.Render(s.TextPrimary.Render("◉") + " 3 selected")
	if strings.Contains(got, nested) {
		t.Errorf("selection chip must not use nested Render (background-bleed bug); got %q", got)
	}
}

func TestRender_SelectionChunkSuppressedWhenZero(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 7}, &SelectionChunk{Count: 0}, "", s)
	plain := ansi.Strip(got)
	if strings.Contains(plain, "selected") {
		t.Errorf("zero-count selection should suppress chip, got %q", plain)
	}
}

func TestRender_SelectionChunkSurvivesLongerThanFilter(t *testing.T) {
	s := styles.NewStyles(true)
	// Width tight enough that filter must drop entirely but the chip survives.
	// breadcrumb "Library" (7) + sep+filter "· filter \"alice\"" (17) + sep+chip "· ◉ 2 selected" (16) = 40.
	// At width 28 we should keep breadcrumb + chip but drop filter.
	got := Render(28, []string{"Library"}, SortChunk{},
		FilterChunk{Text: "alice"}, CountsChunk{}, &SelectionChunk{Count: 2}, "", s)
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "◉ 2 selected") {
		t.Errorf("selection chip should survive at narrow width: %q", plain)
	}
	if strings.Contains(plain, "alice") {
		t.Errorf("filter should drop before selection chip at narrow width: %q", plain)
	}
}

func TestRender_HintStylingKeyLabelPair(t *testing.T) {
	s := styles.NewStyles(true)
	got := Render(80, []string{"Artists", "Brian Eno"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "esc ←", s)
	key := s.HelpKey.Render("esc")
	label := s.TextSecondary.Render("←")
	expected := key + " " + label
	if !strings.Contains(got, expected) {
		t.Errorf("hint should render as HelpKey key + TextSecondary label; got %q want substring %q", got, expected)
	}
	dimHint := s.Dim.Render("esc ←")
	if strings.Contains(got, dimHint) {
		t.Errorf("hint must no longer render with Dim style; got %q", got)
	}
}

// TestRender_BreadcrumbSliceUnchangedAfterNarrowRender verifies that a
// narrow render that truncates the last breadcrumb crumb does not mutate
// the caller's original slice. A subsequent wide render must still see the
// full, untruncated crumb.
func TestRender_BreadcrumbSliceUnchangedAfterNarrowRender(t *testing.T) {
	s := styles.NewStyles(true)
	crumbs := []string{"Artists", "A Very Long Artist Name"}
	original := make([]string, len(crumbs))
	copy(original, crumbs)

	// Narrow render: forces last-crumb truncation.
	Render(22, crumbs, SortChunk{}, FilterChunk{}, CountsChunk{}, nil, "", s)

	for i, c := range crumbs {
		if c != original[i] {
			t.Errorf("crumbs[%d] mutated: want %q, got %q", i, original[i], c)
		}
	}

	// Wide render after narrow: must show the full original crumb.
	wide := Render(80, crumbs, SortChunk{}, FilterChunk{}, CountsChunk{}, nil, "", s)
	plain := ansi.Strip(wide)
	if !strings.Contains(plain, "A Very Long Artist Name") {
		t.Errorf("wide render after narrow must show full crumb; got %q", plain)
	}
}
