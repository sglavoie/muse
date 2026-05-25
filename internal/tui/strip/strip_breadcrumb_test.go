package strip

import (
	"strings"
	"testing"
)

func TestRender_BreadcrumbChain(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Artists", "Brian Eno", "Heroes"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 7}, nil, "esc ←", s)
	plain := requireRenderWidth(t, got, 80)

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

func TestRender_BreadcrumbTruncatesLastCrumb(t *testing.T) {
	s := newTestStyles()
	// Width small enough that the last crumb cannot fully fit.
	got := Render(22, []string{"Artists", "A Very Long Artist Name"}, SortChunk{}, FilterChunk{}, CountsChunk{}, nil, "", s)
	plain := requireRenderWidth(t, got, 22)

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
	s := newTestStyles()
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
	s := newTestStyles()
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

// TestRender_BreadcrumbSliceUnchangedAfterNarrowRender verifies that a
// narrow render that truncates the last breadcrumb crumb does not mutate
// the caller's original slice. A subsequent wide render must still see the
// full, untruncated crumb.
func TestRender_BreadcrumbSliceUnchangedAfterNarrowRender(t *testing.T) {
	s := newTestStyles()
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
	plain := requireRenderWidth(t, wide, 80)
	if !strings.Contains(plain, "A Very Long Artist Name") {
		t.Errorf("wide render after narrow must show full crumb; got %q", plain)
	}
}

// TestRender_EmptyCrumbsFiltered verifies that empty strings within the
// breadcrumb slice do not render as free-floating chevrons. Passing
// []string{"", "Foo", ""} previously produced " › Foo › " — chevron
// separators with no anchoring labels. The renderer must filter such
// crumbs out (or treat a fully empty result like the zero-breadcrumb
// case).
func TestRender_EmptyCrumbsFiltered(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"", "Foo", ""}, SortChunk{}, FilterChunk{},
		CountsChunk{Total: 0}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)
	if strings.Contains(plain, "›") {
		t.Errorf("empty crumbs must not render free-floating chevrons; got %q", plain)
	}
	if !strings.Contains(plain, "Foo") {
		t.Errorf("non-empty crumbs must survive filtering; got %q", plain)
	}
}

// TestRender_AllEmptyCrumbsReturnsEmpty verifies that a breadcrumb made up
// entirely of empty strings is treated the same as the zero-breadcrumb
// case (empty render), not a blank chevron-only line.
func TestRender_AllEmptyCrumbsReturnsEmpty(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"", "", ""}, SortChunk{}, FilterChunk{},
		CountsChunk{Total: 7}, nil, "", s)
	if got != "" {
		t.Errorf("all-empty crumbs should render as empty string, got %q", got)
	}
}

// TestRender_NarrowestBreadcrumbKeepsChevronStyling verifies that when the
// fallback "drop parent crumbs entirely" branch fires (because no per-crumb
// budget can fit a parent + chevron), the surviving single crumb does not
// contain stray plain chevron characters borrowed from the parent path.
// Previously the joined-then-truncated path produced a single string like
// "AAA › B…" where the chevron rendered through TextSecondary rather than
// the Dim chevron style.
func TestRender_NarrowestBreadcrumbKeepsChevronStyling(t *testing.T) {
	s := newTestStyles()
	got := Render(8, []string{"AAA", "BBB", "CCC"}, SortChunk{}, FilterChunk{},
		CountsChunk{}, nil, "", s)
	plain := requireRenderWidth(t, got, 8)
	if strings.Contains(plain, "›") {
		t.Errorf("ultra-narrow breadcrumb should drop parents rather than leak unstyled chevrons; got %q", plain)
	}
	// The last crumb (CCC) should be the surviving anchor; "AAA" and "BBB"
	// should have been dropped to free room.
	if strings.Contains(plain, "AAA") || strings.Contains(plain, "BBB") {
		t.Errorf("parent crumbs should be dropped at width 8, got %q", plain)
	}
	if !strings.Contains(plain, "C") {
		t.Errorf("last crumb should survive (possibly truncated), got %q", plain)
	}
}
