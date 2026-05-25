package text

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBuildFilterSeparatorNormalizesTabs(t *testing.T) {
	out := BuildFilterSeparator("Sort: Name\tIgnored", false, "alpha\tbeta", "", "", 40, false)
	if strings.Contains(out, "\t") {
		t.Fatalf("separator must not contain tab characters; got %q", out)
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
	line := trimmedLine(out)
	if w := ansi.StringWidth(line); w > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", w, width, line)
	}
	requireDisplayWidth(t, line, width)
	if !strings.HasSuffix(strings.TrimRight(line, " "), "…") {
		t.Errorf("expected trailing ellipsis on truncated filter, got %q", line)
	}
}

func TestBuildFilterSeparatorCommittedFilterClampedToNarrowWidth(t *testing.T) {
	const width = 5
	out := BuildFilterSeparator("", false, "abcdefghijklmnopqrstuvwxyz", "", "", width, false)
	line := trimmedLine(out)
	if got := ansi.StringWidth(line); got > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", got, width, line)
	}
}

func TestBuildFilterSeparatorActiveFilterClampedToWidth(t *testing.T) {
	const width = 12
	out := BuildFilterSeparator("", true, "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz", "", width, false)
	line := trimmedLine(out)
	if got := ansi.StringWidth(line); got > width {
		t.Fatalf("line width = %d, want <= %d (got %q)", got, width, line)
	}
}

func TestBuildFilterSeparatorActiveHintClampedToWidth(t *testing.T) {
	const width = 12
	out := BuildFilterSeparator("", true, "", "", "type to filter", width, false)
	line := trimmedLine(out)
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
			line := trimmedLine(out)
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
	line := trimmedLine(out)
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
