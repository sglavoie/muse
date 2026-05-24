package text

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"empty string", "", 10, ""},
		{"maxLen zero", "hello", 0, ""},
		{"maxLen negative", "hello", -1, ""},
		{"maxLen 1", "hello", 1, "…"},
		{"shorter than maxLen", "hi", 10, "hi"},
		{"exactly maxLen", "hello", 5, "hello"},
		{"longer than maxLen", "hello world", 5, "hell…"},
		{"wide chars CJK", "你好世界", 5, "你好…"},
		{"emoji truncated", "🎵🎶🎸🎹", 5, "🎵🎶…"},
		{"line break is normalized", "hello\nworld", 20, "hello world"},
		{"carriage return is normalized", "hello\rworld", 20, "hello world"},
		{"crlf is normalized once", "hello\r\nworld", 20, "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestPadNormalizesLineBreaks(t *testing.T) {
	got := Pad("first\nsecond", 15)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("Pad preserved a line break: %q", got)
	}
	if want := "first second   "; got != want {
		t.Fatalf("Pad with line break = %q, want %q", got, want)
	}
	if w := ansi.StringWidth(got); w != 15 {
		t.Fatalf("Pad display width = %d, want 15", w)
	}
}

// TestPadNormalizesTabs guards against tabs leaking through Pad and breaking
// width arithmetic in the terminal. Terminal tab stops mean a literal '\t'
// occupies a variable number of display columns, so callers padding by
// ansi.StringWidth would render misaligned columns.
func TestPadNormalizesTabs(t *testing.T) {
	got := Pad("ab\tcd", 10)
	if strings.Contains(got, "\t") {
		t.Fatalf("Pad preserved a tab character: %q", got)
	}
	if want := "ab cd     "; got != want {
		t.Fatalf("Pad with tab = %q, want %q", got, want)
	}
	if w := ansi.StringWidth(got); w != 10 {
		t.Fatalf("Pad display width = %d, want 10", w)
	}
}

func TestTruncateNormalizesTabs(t *testing.T) {
	got := Truncate("ab\tcd", 10)
	if strings.Contains(got, "\t") {
		t.Fatalf("Truncate preserved a tab character: %q", got)
	}
}

func TestBuildFilterSeparatorNormalizesTabs(t *testing.T) {
	out := BuildFilterSeparator("Sort: Name\tIgnored", false, "alpha\tbeta", "", "", 40, false)
	if strings.Contains(out, "\t") {
		t.Fatalf("separator must not contain tab characters; got %q", out)
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{"width zero", "hello", 0, ""},
		{"width negative", "hello", -1, ""},
		{"short string padded", "hi", 6, "hi    "},
		{"exactly width", "hello", 5, "hello"},
		{"long string truncated", "hello world", 5, "hell…"},
		{"wide chars padded", "你好", 6, "你好  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pad(tt.s, tt.width)
			if got != tt.want {
				t.Errorf("Pad(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
			}
			if tt.width > 0 {
				w := ansi.StringWidth(got)
				if w != tt.width {
					t.Errorf("Pad(%q, %d) display width = %d, want %d", tt.s, tt.width, w, tt.width)
				}
			}
		})
	}
}

func TestPadLeft(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{"width zero", "hello", 0, ""},
		{"width negative", "hello", -1, ""},
		{"short string left-padded", "hi", 6, "    hi"},
		{"exactly width", "hello", 5, "hello"},
		{"long string truncated", "hello world", 5, "hell…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PadLeft(tt.s, tt.width)
			if got != tt.want {
				t.Errorf("PadLeft(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
			}
			if tt.width > 0 {
				w := ansi.StringWidth(got)
				if w != tt.width {
					t.Errorf("PadLeft(%q, %d) display width = %d, want %d", tt.s, tt.width, w, tt.width)
				}
			}
		})
	}
}

func TestFormatDurationSecs(t *testing.T) {
	tests := []struct {
		name string
		secs float64
		want string
	}{
		{"zero", 0, "--:--"},
		{"negative", -5.0, "--:--"},
		{"NaN", math.NaN(), "--:--"},
		{"positive infinity", math.Inf(1), "--:--"},
		{"negative infinity", math.Inf(-1), "--:--"},
		{"seconds only", 45.0, "0:45"},
		{"minutes and seconds", 125.0, "2:05"},
		{"hours", 3661.0, "1:01:01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDurationSecs(tt.secs)
			if got != tt.want {
				t.Errorf("FormatDurationSecs(%v) = %q, want %q", tt.secs, got, tt.want)
			}
		})
	}
}

func TestFormatTrackPos(t *testing.T) {
	tests := []struct {
		name     string
		trackNum int
		discNum  int
		fallback int
		want     string
	}{
		{"unknown zero uses fallback", 0, 1, 7, "7"},
		{"invalid negative uses fallback", -1, 1, 7, "7"},
		{"invalid negative with disc uses fallback", -1, 2, 7, "7"},
		{"single disc uses bare track", 3, 1, 7, "3"},
		{"multi disc prefixes disc", 3, 2, 7, "2.03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTrackPos(tt.trackNum, tt.discNum, tt.fallback)
			if got != tt.want {
				t.Errorf("FormatTrackPos(%d, %d, %d) = %q, want %q",
					tt.trackNum, tt.discNum, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestFormatTrackPosSimple(t *testing.T) {
	tests := []struct {
		name     string
		trackNum int
		fallback int
		want     string
	}{
		{"unknown zero uses fallback", 0, 7, "7"},
		{"invalid negative uses fallback", -1, 7, "7"},
		{"positive uses track number", 3, 7, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTrackPosSimple(tt.trackNum, tt.fallback)
			if got != tt.want {
				t.Errorf("FormatTrackPosSimple(%d, %d) = %q, want %q",
					tt.trackNum, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestRowMarker(t *testing.T) {
	const npIcon = "♪"
	tests := []struct {
		name       string
		selected   bool
		focused    bool
		nowPlaying bool
		icon       string
		want       string
	}{
		{"default", false, false, false, npIcon, "  "},
		{"focused only", false, true, false, npIcon, "▎ "},
		{"now-playing only", false, false, true, npIcon, "♪ "},
		{"focused and now-playing", false, true, true, npIcon, "♪ "},
		{"selected only", true, false, false, npIcon, "● "},
		{"selected and focused", true, true, false, npIcon, "● "},
		{"selected and now-playing", true, false, true, npIcon, "● "},
		{"selected focused and now-playing", true, true, true, npIcon, "● "},
		{"custom now-playing icon", false, false, true, "▶", "▶ "},
		{"empty now-playing icon falls back", false, false, true, "", "♪ "},
		{"wide now-playing icon is preserved", false, false, true, "🎵", "🎵"},
		{"overwide now-playing icon is clipped to marker width", false, false, true, "🎵▶", "🎵"},
		{"zero-width now-playing icon falls back", false, false, true, "\u0301", "♪ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RowMarker(tt.selected, tt.focused, tt.nowPlaying, tt.icon)
			if got != tt.want {
				t.Errorf("RowMarker(%v, %v, %v, %q) = %q, want %q",
					tt.selected, tt.focused, tt.nowPlaying, tt.icon, got, tt.want)
			}
			if w := ansi.StringWidth(got); w != 2 {
				t.Errorf("RowMarker(%v, %v, %v, %q) display width = %d, want 2",
					tt.selected, tt.focused, tt.nowPlaying, tt.icon, w)
			}
		})
	}
}

func TestDimRepeat(t *testing.T) {
	tests := []struct {
		name  string
		value string
		prev  string
		width int
		want  string
	}{
		{"nil previous", "Pink Floyd", "", 12, "Pink Floyd  "},
		{"different from previous", "Pink Floyd", "Radiohead", 12, "Pink Floyd  "},
		{"equal strings", "Pink Floyd", "Pink Floyd", 12, "·           "},
		{"equal strings minimal width", "Pink Floyd", "Pink Floyd", 1, "·"},
		{"width zero", "Pink Floyd", "Pink Floyd", 0, ""},
		{"width negative", "Pink Floyd", "Pink Floyd", -3, ""},
		{"equal with favorite-marked previous", "★ Pink Floyd", "★ Pink Floyd", 14, "·             "},
		{"narrower width truncates non-repeat", "Pink Floyd", "Radiohead", 5, "Pink…"},
		{"empty value with non-empty previous", "", "Pink Floyd", 5, "     "},
		{"empty value matches empty prev (no dim)", "", "", 5, "     "},
		{"wide chars equal", "你好世界", "你好世界", 8, "·       "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DimRepeat(tt.value, tt.prev, tt.width)
			if got != tt.want {
				t.Errorf("DimRepeat(%q, %q, %d) = %q, want %q",
					tt.value, tt.prev, tt.width, got, tt.want)
			}
			if tt.width > 0 {
				w := ansi.StringWidth(got)
				if w != tt.width {
					t.Errorf("DimRepeat(%q, %q, %d) display width = %d, want %d",
						tt.value, tt.prev, tt.width, w, tt.width)
				}
			}
		})
	}
}

func TestRightPadWithCount(t *testing.T) {
	tests := []struct {
		name      string
		label     string
		count     int
		width     int
		wantLabel string
		wantCount string
	}{
		{"small count fits", "Beatles", 3, 20, "Beatles       ", "     3"},
		{"large count fits", "Beatles", 9999, 20, "Beatles       ", "  9999"},
		{"zero count shows blank digits", "Beatles", 0, 20, "Beatles       ", "      "},
		{"negative count shows blank digits", "Beatles", -1, 20, "Beatles       ", "      "},
		{"label longer than label-zone truncates", "A Very Long Artist Name", 5, 20, "A Very Long A…", "     5"},
		{"width exactly at count threshold", "X", 7, MinLabelWithCount + 1 + CountColWidth, "X     ", "     7"},
		{"width just below threshold drops count", "X", 7, MinLabelWithCount + CountColWidth, "X          ", ""},
		{"narrow width drops count entirely", "Beatles", 12, 8, "Beatles ", ""},
		{"width zero returns empty", "Beatles", 12, 0, "", ""},
		{"width negative returns empty", "Beatles", 12, -3, "", ""},
		{"wide-char label", "你好世界", 42, 20, "你好世界      ", "    42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLabel, gotCount := RightPadWithCount(tt.label, tt.count, tt.width)
			if gotLabel != tt.wantLabel {
				t.Errorf("RightPadWithCount(%q,%d,%d) labelPart = %q, want %q",
					tt.label, tt.count, tt.width, gotLabel, tt.wantLabel)
			}
			if gotCount != tt.wantCount {
				t.Errorf("RightPadWithCount(%q,%d,%d) countPart = %q, want %q",
					tt.label, tt.count, tt.width, gotCount, tt.wantCount)
			}
			if tt.width > 0 {
				w := ansi.StringWidth(gotLabel + gotCount)
				if w != tt.width {
					t.Errorf("RightPadWithCount(%q,%d,%d) total display width = %d, want %d",
						tt.label, tt.count, tt.width, w, tt.width)
				}
			}
		})
	}
}

func TestRightPadWithCountVerticalAlignment(t *testing.T) {
	// Two rows with different counts and different label lengths must produce
	// the same total width and the same count-zone offset.
	const width = 30
	l1, c1 := RightPadWithCount("Short", 1, width)
	l2, c2 := RightPadWithCount("Much Longer Label", 9999, width)
	if ansi.StringWidth(l1) != ansi.StringWidth(l2) {
		t.Fatalf("label zone widths differ: %d vs %d", ansi.StringWidth(l1), ansi.StringWidth(l2))
	}
	if ansi.StringWidth(c1) != ansi.StringWidth(c2) {
		t.Fatalf("count zone widths differ: %d vs %d", ansi.StringWidth(c1), ansi.StringWidth(c2))
	}
	if ansi.StringWidth(l1+c1) != width || ansi.StringWidth(l2+c2) != width {
		t.Fatalf("total row width != %d", width)
	}
}

// TestBuildFilterSeparatorTruncationUsesFullBudget guards against wasting a
// column in committed filter rendering. The committed filter text portion
// ("Filter: " + ft) should fit the row's available budget while consuming
// every available column up to that budget.
func TestBuildFilterSeparatorTruncationUsesFullBudget(t *testing.T) {
	const width = 20
	longFilter := "abcdefghijklmnopqrstuvwxyz" // 26 chars, far longer than fits
	out := BuildFilterSeparator("", false, longFilter, "", "", width, false)
	// Strip trailing newline before measuring.
	line := strings.TrimRight(out, "\n")
	if w := ansi.StringWidth(line); w > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", w, width, line)
	}
	if w := ansi.StringWidth(line); w != width {
		t.Errorf("line width = %d, want %d (got %q) - off-by-one regression?", w, width, line)
	}
	if !strings.HasSuffix(strings.TrimRight(line, " "), "…") {
		t.Errorf("expected trailing ellipsis on truncated filter, got %q", line)
	}
}

func TestBuildFilterSeparatorCommittedFilterClampedToNarrowWidth(t *testing.T) {
	const width = 5
	out := BuildFilterSeparator("", false, "abcdefghijklmnopqrstuvwxyz", "", "", width, false)
	line := strings.TrimRight(out, "\n")
	if got := ansi.StringWidth(line); got > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", got, width, line)
	}
}

func TestBuildFilterSeparatorActiveFilterClampedToWidth(t *testing.T) {
	const width = 12
	out := BuildFilterSeparator("", true, "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz", "", width, false)
	line := strings.TrimRight(out, "\n")
	if got := ansi.StringWidth(line); got > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", got, width, line)
	}
}

func TestBuildFilterSeparatorActiveHintClampedToWidth(t *testing.T) {
	const width = 12
	out := BuildFilterSeparator("", true, "", "", "type to filter", width, false)
	line := strings.TrimRight(out, "\n")
	if got := ansi.StringWidth(line); got > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", got, width, line)
	}
}

func TestBuildFilterSeparatorNonPositiveWidthSuppressesFilterLine(t *testing.T) {
	tests := []struct {
		name         string
		filterActive bool
		filterText   string
		filterView   string
		renderedHint string
		width        int
	}{
		{"active filter width zero", true, "abc", "abc", "", 0},
		{"active hint width zero", true, "", "", "type to filter", 0},
		{"committed filter width zero", false, "abc", "", "", 0},
		{"active filter negative width", true, "abc", "abc", "", -1},
		{"committed filter negative width", false, "abc", "", "", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := BuildFilterSeparator("", tt.filterActive, tt.filterText, tt.filterView, tt.renderedHint, tt.width, false)
			line := strings.TrimRight(out, "\n")
			if line != "" {
				t.Fatalf("filter line = %q, want empty", line)
			}
		})
	}
}

// TestBuildFilterSeparatorMultiByteFilter verifies that ansi.StringWidth-based
// truncation handles multi-byte filter text without breaking width arithmetic
// or splitting graphemes.
func TestBuildFilterSeparatorMultiByteFilter(t *testing.T) {
	const width = 25
	// Mix of wide CJK and accented runes, each contributing a different
	// display width than its byte length.
	ft := "你好世界 naïve café日本語"
	out := BuildFilterSeparator("", false, ft, "", "", width, false)
	line := strings.TrimRight(out, "\n")
	if w := ansi.StringWidth(line); w > width {
		t.Errorf("multi-byte truncation overflow: width=%d > %d (line=%q)", w, width, line)
	}
}

func TestBuildFilterSeparatorNormalizesLineBreaks(t *testing.T) {
	out := BuildFilterSeparator("Sort: Name\nIgnored", false, "alpha\nbeta", "", "", 40, false)
	if strings.Count(out, "\n") != 2 {
		t.Fatalf("separator newline count = %d, want 2 (out=%q)", strings.Count(out, "\n"), out)
	}
	if !strings.Contains(out, "Sort: Name Ignored\nFilter: alpha beta\n") {
		t.Fatalf("separator did not normalize line breaks as expected: %q", out)
	}
}

func TestFormatDurationMs(t *testing.T) {
	tests := []struct {
		name string
		ms   int64
		want string
	}{
		{"zero", 0, "--:--"},
		{"negative", -1000, "--:--"},
		{"typical value", 125000, "2:05"},
		{"hours", 3661000, "1:01:01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDurationMs(tt.ms)
			if got != tt.want {
				t.Errorf("FormatDurationMs(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}
