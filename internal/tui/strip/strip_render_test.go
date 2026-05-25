package strip

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRender_QuietForm(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

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
	s := newTestStyles()
	sort := SortChunk{Label: "Artist", Indicator: "▲"}
	filter := FilterChunk{Text: "alice"}
	counts := CountsChunk{Filtered: 2, Total: 7, Filtering: true}

	got := Render(80, []string{"Library"}, sort, filter, counts, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

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
	s := newTestStyles()
	// Simulate a textinput.View() output: the first character is wrapped in
	// reverse-video to stand in for the cursor.
	inputView := lipgloss.NewStyle().Reverse(true).Render("a") + "lice"
	filter := FilterChunk{Active: true, Text: "alice", InputView: inputView}

	got := Render(80, []string{"Library"}, SortChunk{}, filter, CountsChunk{Total: 7}, nil, "", s)
	if !strings.Contains(got, inputView) {
		t.Errorf("expected raw InputView preserved (with its own ANSI), got %q", got)
	}

	plain := requireRenderWidth(t, got, 80)
	// The textinput's rendered width counts as the value width — total strip
	// is still padded to width.
	if !strings.Contains(plain, `filter "alice"`) {
		t.Errorf("expected filter %q in plain %q", `filter "alice"`, plain)
	}
}

func TestRender_FilterActiveEmptyShowsHint(t *testing.T) {
	s := newTestStyles()
	hint := s.Dim.Render("a: artist  t: title")
	filter := FilterChunk{Active: true, Hint: hint}

	got := Render(80, []string{"Library"}, SortChunk{}, filter, CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)
	if !strings.Contains(plain, "a: artist") {
		t.Errorf("expected hint placeholder in active+empty filter chunk, got %q", plain)
	}
	if !strings.Contains(plain, `filter "`) {
		t.Errorf("expected filter prefix, got %q", plain)
	}
}

func TestRender_ZeroWidth(t *testing.T) {
	s := newTestStyles()
	if got := Render(0, []string{"Library"}, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s); got != "" {
		t.Errorf("Render(0) = %q, want empty", got)
	}
}

func TestRender_EmptyBreadcrumb(t *testing.T) {
	s := newTestStyles()
	if got := Render(80, nil, SortChunk{}, FilterChunk{}, CountsChunk{Total: 3}, nil, "", s); got != "" {
		t.Errorf("Render(empty breadcrumb) = %q, want empty", got)
	}
}
