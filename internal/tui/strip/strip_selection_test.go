package strip

import (
	"strings"
	"testing"
)

func TestRender_SelectionChunkRendered(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 7}, &SelectionChunk{Count: 3}, "", s)
	plain := requireRenderWidth(t, got, 80)
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

func TestRender_SelectionChunkBetweenBreadcrumbAndSortFilter(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{Label: "Artist", Indicator: "▲"},
		FilterChunk{Text: "alice"}, CountsChunk{Total: 7}, &SelectionChunk{Count: 3}, "", s)
	plain := requireRenderWidth(t, got, 80)

	breadcrumbIdx := strings.Index(plain, "Library")
	selectionIdx := strings.Index(plain, "◉ 3 selected")
	sortIdx := strings.Index(plain, "sort Artist▲")
	filterIdx := strings.Index(plain, `filter "alice"`)
	if !(breadcrumbIdx >= 0 && breadcrumbIdx < selectionIdx && selectionIdx < sortIdx && sortIdx < filterIdx) {
		t.Errorf("expected selection between breadcrumb and sort/filter, got positions breadcrumb=%d selection=%d sort=%d filter=%d in %q",
			breadcrumbIdx, selectionIdx, sortIdx, filterIdx, plain)
	}
}

func TestRender_SelectionChunkSuppressedWhenZero(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 7}, &SelectionChunk{Count: 0}, "", s)
	plain := requireRenderWidth(t, got, 80)
	if strings.Contains(plain, "selected") {
		t.Errorf("zero-count selection should suppress chip, got %q", plain)
	}
}

func TestRenderSelectionDurationYieldsBeforeOtherDetails(t *testing.T) {
	sel := &SelectionChunk{Count: 2, Duration: "7:08+"}
	for _, width := range []int{100, 60, 45, 28, 15} {
		got := Render(width, []string{"Library"}, SortChunk{Label: "Artist"},
			FilterChunk{Text: "alice"}, CountsChunk{Total: 7}, sel, "", newTestStyles())
		plain := requireRenderWidth(t, got, width)
		if width == 100 && !strings.Contains(plain, "2 selected · 7:08+") {
			t.Fatalf("wide strip missing duration: %q", plain)
		}
		if width == 60 && (strings.Contains(plain, "7:08+") || !strings.Contains(plain, `filter "alice"`) || !strings.Contains(plain, "sort Artist")) {
			t.Fatalf("duration should yield before sort/filter: %q", plain)
		}
		if width == 28 && !strings.Contains(plain, "2 selected") {
			t.Fatalf("selection count should survive duration: %q", plain)
		}
	}
	if sel.Duration != "7:08+" {
		t.Fatal("render mutated caller's selection")
	}
}

func TestRender_SelectionChunkSurvivesLongerThanFilter(t *testing.T) {
	s := newTestStyles()
	// Width tight enough that filter must drop entirely but the chip survives.
	// breadcrumb "Library" (7) + sep+filter "· filter \"alice\"" (17) + sep+chip "· ◉ 2 selected" (16) = 40.
	// At width 28 we should keep breadcrumb + chip but drop filter.
	got := Render(28, []string{"Library"}, SortChunk{},
		FilterChunk{Text: "alice"}, CountsChunk{}, &SelectionChunk{Count: 2}, "", s)
	plain := requireRenderWidth(t, got, 28)
	if !strings.Contains(plain, "◉ 2 selected") {
		t.Errorf("selection chip should survive at narrow width: %q", plain)
	}
	if strings.Contains(plain, "alice") {
		t.Errorf("filter should drop before selection chip at narrow width: %q", plain)
	}
}
