package strip

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRender_HintStylingKeyLabelPair(t *testing.T) {
	s := newTestStyles()
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

func TestRender_BreadcrumbLineBreaksNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Artists", "Brian\nEno\r\nHeroes"}, SortChunk{}, FilterChunk{},
		CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

	if strings.ContainsAny(plain, "\r\n") {
		t.Fatalf("strip must remain single-line, got %q", plain)
	}
	if !strings.Contains(plain, "Brian Eno Heroes") {
		t.Errorf("line breaks should normalize to spaces, got %q", plain)
	}
}

func TestRender_FilterLineBreaksNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{}, FilterChunk{Text: "alice\nbob"},
		CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

	if strings.ContainsAny(plain, "\r\n") {
		t.Fatalf("strip must remain single-line, got %q", plain)
	}
	if !strings.Contains(plain, `filter "alice bob"`) {
		t.Errorf("line breaks should normalize to spaces in filter text, got %q", plain)
	}
}

func TestRender_PlainFieldsStripANSIAndControlBytes(t *testing.T) {
	s := newTestStyles()
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("Red")
	styledMode := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("Mode")
	got := Render(120,
		[]string{"Artists" + red, "Brian\bEno"},
		SortChunk{Label: "Album" + styledMode, Indicator: "\x07▼"},
		FilterChunk{Text: "alice\x07bob"},
		CountsChunk{Total: 7},
		nil,
		"esc"+red+" back",
		s,
	)
	plain := requireRenderWidth(t, got, 120)

	if strings.Contains(plain, "\x1b") || strings.Contains(plain, "\x07") || strings.Contains(plain, "\b") {
		t.Fatalf("plain strip fields must not leak ANSI/control bytes, got %q", plain)
	}
	for _, want := range []string{"ArtistsRed", "Brian Eno", "sort AlbumMode ▼", `filter "alice bob"`, "escRed back"} {
		if !strings.Contains(plain, want) {
			t.Errorf("sanitized plain field missing %q in %q", want, plain)
		}
	}
}

func TestRender_ActiveInputViewPreservesCursorANSI(t *testing.T) {
	s := newTestStyles()
	inputView := lipgloss.NewStyle().Reverse(true).Render("a") + "lice"

	got := Render(80, []string{"Library"}, SortChunk{},
		FilterChunk{Active: true, Text: "alice", InputView: inputView},
		CountsChunk{Total: 7}, nil, "", s)

	if !strings.Contains(got, inputView) {
		t.Errorf("active filter input view should preserve caller-rendered cursor ANSI, got %q", got)
	}
}

func TestRender_HintLineBreaksNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Artists"}, SortChunk{}, FilterChunk{},
		CountsChunk{Total: 7}, nil, "esc\nback", s)
	plain := requireRenderWidth(t, got, 80)

	if strings.ContainsAny(plain, "\r\n") {
		t.Fatalf("strip must remain single-line, got %q", plain)
	}
	if !strings.Contains(plain, "esc back") {
		t.Errorf("line breaks should normalize to spaces in hint text, got %q", plain)
	}
}

func TestRender_HintWhitespaceNormalizedBeforeWidthCalculation(t *testing.T) {
	s := newTestStyles()
	got := Render(30, []string{"Artists"}, SortChunk{}, FilterChunk{},
		CountsChunk{Total: 7}, nil, " esc   back ", s)
	plain := requireRenderWidth(t, got, 30)

	if !strings.HasSuffix(plain, "7 esc back") {
		t.Errorf("hint whitespace should normalize before rendering, got %q", plain)
	}
	if strings.Contains(plain, "  back") || strings.HasSuffix(plain, " ") {
		t.Errorf("hint should not retain layout-only whitespace, got %q", plain)
	}
}

func TestRender_TabCharactersNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(100, []string{"Artists\tView"}, SortChunk{Label: "Album\tName", Indicator: "▼"},
		FilterChunk{Text: "alice\tbob"}, CountsChunk{Total: 7}, nil, "esc\tback", s)
	plain := requireRenderWidth(t, got, 100)

	if strings.Contains(plain, "\t") {
		t.Fatalf("strip must not render tabs because terminal tab stops break width arithmetic, got %q", plain)
	}
	for _, want := range []string{"Artists View", "sort Album Name▼", `filter "alice bob"`, "esc back"} {
		if !strings.Contains(plain, want) {
			t.Errorf("tabs should normalize to spaces; missing %q in %q", want, plain)
		}
	}
}

func TestRender_FilterHintTabsNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(80, []string{"Library"}, SortChunk{},
		FilterChunk{Active: true, Hint: s.TextSecondary.Render("title\tartist")},
		CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

	if strings.Contains(plain, "\t") {
		t.Fatalf("active filter hint must not render tabs, got %q", plain)
	}
	if !strings.Contains(plain, `filter "title`) || !strings.Contains(plain, `artist"`) {
		t.Errorf("hint tabs should normalize to spaces, got %q", plain)
	}
}
