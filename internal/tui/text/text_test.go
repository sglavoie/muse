package text

import (
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
		{"longer than maxLen", "hello world", 5, "hel…"},
		{"wide chars CJK", "你好世界", 5, "你…"},
		{"emoji truncated", "🎵🎶🎸🎹", 5, "🎵…"},
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
		{"long string truncated", "hello world", 5, "hel… "},
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
		{"long string truncated", "hello world", 5, " hel…"},
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
