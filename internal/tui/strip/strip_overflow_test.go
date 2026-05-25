package strip

import (
	"strings"
	"testing"
)

func TestRender_NarrowDropsCounts(t *testing.T) {
	s := newTestStyles()
	sort := SortChunk{Label: "Artist", Indicator: "▲"}
	filter := FilterChunk{Text: "alice"}
	counts := CountsChunk{Filtered: 2, Total: 7, Filtering: true}

	// Width just enough for view + sort + filter but not counts + gap.
	// "Library · sort Artist▲ · filter \"alice\"" = 7 + 3 + 12 + 3 + 14 = 39
	// Drop counts at 40 (no room for gap + "2/7").
	got := Render(40, []string{"Library"}, sort, filter, counts, nil, "", s)
	plain := requireRenderWidth(t, got, 40)

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
	s := newTestStyles()
	got := Render(40, []string{"Library"}, SortChunk{}, FilterChunk{},
		CountsChunk{Filtered: 3, Total: 0, Filtering: true}, nil, "", s)
	plain := requireRenderWidth(t, got, 40)

	if strings.Contains(plain, "3/0") || strings.Contains(plain, "0/0") {
		t.Errorf("counts should be suppressed when total is zero, got %q", plain)
	}
}

func TestRender_NarrowerDropsSort(t *testing.T) {
	s := newTestStyles()
	sort := SortChunk{Label: "Artist", Indicator: "▲"}
	filter := FilterChunk{Text: "alice"}
	counts := CountsChunk{Filtered: 2, Total: 7, Filtering: true}

	// Width just enough for view + filter, not for sort.
	// "Library · filter \"alice\"" = 7 + 3 + 14 = 24
	got := Render(24, []string{"Library"}, sort, filter, counts, nil, "", s)
	plain := requireRenderWidth(t, got, 24)

	if strings.Contains(plain, "sort Artist") {
		t.Errorf("narrower form should drop sort, but plain=%q", plain)
	}
	if !strings.Contains(plain, `filter "alice"`) {
		t.Errorf("narrower form should keep filter, got %q", plain)
	}
}

func TestRender_NarrowestTruncatesFilter(t *testing.T) {
	s := newTestStyles()
	filter := FilterChunk{Text: "averyverylongfilterquery"}
	counts := CountsChunk{Total: 7}

	// Width 20: "Library · filter \"averyverylongfilterquery\"" overflows; we
	// expect the filter value to be truncated with "…" while the prefix and
	// closing quote are preserved.
	got := Render(20, []string{"Library"}, SortChunk{}, filter, counts, nil, "", s)
	plain := requireRenderWidth(t, got, 20)

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

func TestRender_HintDropsFirst(t *testing.T) {
	s := newTestStyles()
	// Width tight enough that hint must drop but counts survive.
	// "Artists › Brian Eno · 2/7" = 7 + 3 + 9 + 1 + 3 = 23.
	// hint " esc ←" adds 7. Width 27 keeps counts (24) but cannot fit
	// the hint (would need 30).
	got := Render(27, []string{"Artists", "Brian Eno"}, SortChunk{}, FilterChunk{}, CountsChunk{Filtered: 2, Total: 7, Filtering: true}, nil, "esc ←", s)
	plain := requireRenderWidth(t, got, 27)

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

// TestRender_CountsClampNegativeFiltered verifies that a negative Filtered
// count never leaks into the user-visible strip (e.g. as "-1/7"). Upstream
// off-by-ones should be defensively clamped to 0 by the renderer.
func TestRender_CountsClampNegativeFiltered(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{},
		CountsChunk{Filtered: -5, Total: 7, Filtering: true}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)
	if strings.Contains(plain, "-5/7") || strings.Contains(plain, "-1") {
		t.Errorf("negative Filtered should be clamped before rendering; got %q", plain)
	}
	if !strings.Contains(plain, "0/7") {
		t.Errorf("expected clamped '0/7' counts; got %q", plain)
	}
}

// TestRender_CountsClampFilteredAboveTotal verifies that Filtered > Total is
// clamped to Total in the rendered output. A user-visible "100/7" makes no
// sense and signals a defensive bug to the user.
func TestRender_CountsClampFilteredAboveTotal(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{},
		CountsChunk{Filtered: 100, Total: 7, Filtering: true}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)
	if strings.Contains(plain, "100/7") {
		t.Errorf("Filtered > Total should be clamped before rendering; got %q", plain)
	}
	if !strings.Contains(plain, "7/7") {
		t.Errorf("expected clamped '7/7' counts; got %q", plain)
	}
}
