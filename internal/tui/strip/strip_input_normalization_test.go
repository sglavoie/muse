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

func TestRender_StyledFilterFieldsPreserveANSIAndRemoveControlBytes(t *testing.T) {
	s := newTestStyles()
	inputView := lipgloss.NewStyle().Reverse(true).Render("a") + "\x07lice\b"

	got := Render(80, []string{"Library"}, SortChunk{},
		FilterChunk{Active: true, Text: "alice", InputView: inputView},
		CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

	if !strings.Contains(got, lipgloss.NewStyle().Reverse(true).Render("a")) {
		t.Errorf("active filter input view should preserve caller-rendered ANSI, got %q", got)
	}
	if strings.ContainsAny(plain, "\x07\b") {
		t.Fatalf("styled filter fields must not leak raw control bytes, got %q", plain)
	}
	if !strings.Contains(plain, `filter "a lice "`) {
		t.Errorf("control bytes should normalize to spaces in styled filter fields, got %q", plain)
	}
}

func TestRender_StyledFilterFieldsNeutralizeNonSGREscapes(t *testing.T) {
	s := newTestStyles()
	cursorA := lipgloss.NewStyle().Reverse(true).Render("a")
	inputView := cursorA + "\x1b[2Jlice\x1b"

	got := Render(80, []string{"Library"}, SortChunk{},
		FilterChunk{Active: true, Text: "alice", InputView: inputView},
		CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

	if !strings.Contains(got, cursorA) {
		t.Errorf("active filter input view should preserve SGR cursor ANSI, got %q", got)
	}
	if strings.Contains(got, "\x1b[2J") {
		t.Fatalf("styled filter fields must not preserve non-SGR terminal sequences, got %q", got)
	}
	if strings.Contains(got, "\x1b\"") {
		t.Fatalf("styled filter fields must not leak a lone ESC before the closing quote, got %q", got)
	}
	if !strings.Contains(plain, `filter "a [2Jlice "`) {
		t.Errorf("non-SGR escapes should be neutralized as visible text, got %q", plain)
	}
}

func TestRender_ControlRunesNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(120, []string{"Artists\u0085View"}, SortChunk{Label: "Album\u009bName", Indicator: "▼"},
		FilterChunk{Text: "alice\u0085bob"}, CountsChunk{Total: 7}, nil, "esc\u009bback", s)
	plain := requireRenderWidth(t, got, 120)

	for _, control := range []string{"\u0085", "\u009b"} {
		if strings.Contains(plain, control) {
			t.Fatalf("strip must not render Unicode control characters, got %q", plain)
		}
	}
	for _, want := range []string{"Artists View", "sort Album Name▼", `filter "alice bob"`, "esc back"} {
		if !strings.Contains(plain, want) {
			t.Errorf("Unicode controls should normalize to spaces; missing %q in %q", want, plain)
		}
	}
}

func TestRender_UnicodeLineSeparatorsNormalized(t *testing.T) {
	s := newTestStyles()
	got := Render(120, []string{"Artists\u2028View"}, SortChunk{},
		FilterChunk{Text: "alice\u2029bob"}, CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 120)

	if strings.ContainsAny(plain, "\u2028\u2029") {
		t.Fatalf("strip must not render Unicode line separators, got %q", plain)
	}
	for _, want := range []string{"Artists View", `filter "alice bob"`} {
		if !strings.Contains(plain, want) {
			t.Errorf("Unicode line separators should normalize to spaces; missing %q in %q", want, plain)
		}
	}
}

func TestRender_StyledFilterFieldsNormalizeControlRunes(t *testing.T) {
	s := newTestStyles()
	inputView := lipgloss.NewStyle().Reverse(true).Render("a") + "\u0085lice"

	got := Render(80, []string{"Library"}, SortChunk{},
		FilterChunk{Active: true, Text: "alice", InputView: inputView},
		CountsChunk{Total: 7}, nil, "", s)
	plain := requireRenderWidth(t, got, 80)

	if strings.Contains(plain, "\u0085") {
		t.Fatalf("styled filter fields must not leak Unicode control characters, got %q", plain)
	}
	if !strings.Contains(plain, `filter "a lice"`) {
		t.Errorf("Unicode controls should normalize to spaces in styled filter fields, got %q", plain)
	}
}
