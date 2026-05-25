package text

import (
	"strings"
	"testing"
)

func TestPadNormalizesLineBreaks(t *testing.T) {
	got := Pad("first\nsecond", 15)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("Pad preserved a line break: %q", got)
	}
	if want := "first second   "; got != want {
		t.Fatalf("Pad with line break = %q, want %q", got, want)
	}
	requireDisplayWidth(t, got, 15)
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
	requireDisplayWidth(t, got, 10)
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
				requireDisplayWidth(t, got, tt.width)
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
				requireDisplayWidth(t, got, tt.width)
			}
		})
	}
}
