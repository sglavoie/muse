package text

import (
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

func TestTruncateNormalizesTabs(t *testing.T) {
	got := Truncate("ab\tcd", 10)
	if strings.Contains(got, "\t") {
		t.Fatalf("Truncate preserved a tab character: %q", got)
	}
}

func TestTruncateWidthOnePreservesANSI(t *testing.T) {
	const red = "\x1b[31m"
	const reset = "\x1b[0m"

	got := Truncate(red+"hello"+reset, 1)
	want := red + "…" + reset
	if got != want {
		t.Fatalf("Truncate styled string to width 1 = %q, want %q", got, want)
	}
	if width := ansi.StringWidth(got); width != 1 {
		t.Fatalf("display width = %d, want 1 (got %q)", width, got)
	}
}
