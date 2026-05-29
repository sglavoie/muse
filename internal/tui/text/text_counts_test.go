package text

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

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
				requireDisplayWidth(t, gotLabel+gotCount, tt.width)
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
	requireDisplayWidth(t, l1+c1, width)
	requireDisplayWidth(t, l2+c2, width)
}

func TestRowLabelWidthAndCountNonPositiveWidth(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		counts map[string]int
	}{
		{"zero width with counts", 0, map[string]int{"Beatles": 3}},
		{"negative width with counts", -4, map[string]int{"Beatles": 3}},
		{"negative width without counts", -2, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotCount := RowLabelWidthAndCount(tt.width, tt.counts, "Beatles")
			if gotWidth != 0 {
				t.Fatalf("RowLabelWidthAndCount(%d, counts, %q) width = %d, want 0",
					tt.width, "Beatles", gotWidth)
			}
			if gotCount != "" {
				t.Fatalf("RowLabelWidthAndCount(%d, counts, %q) count = %q, want empty",
					tt.width, "Beatles", gotCount)
			}
		})
	}
}

func TestRowLabelWidthAndCountEmptyCountsUsesFullWidth(t *testing.T) {
	const width = 20

	gotWidth, gotCount := RowLabelWidthAndCount(width, map[string]int{}, "Beatles")
	if gotWidth != width {
		t.Fatalf("RowLabelWidthAndCount(%d, empty counts, %q) width = %d, want %d",
			width, "Beatles", gotWidth, width)
	}
	if gotCount != "" {
		t.Fatalf("RowLabelWidthAndCount(%d, empty counts, %q) count = %q, want empty",
			width, "Beatles", gotCount)
	}
}
