package text

import (
	"github.com/charmbracelet/x/ansi"
	"testing"
)

func TestFavPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxWidth int
		isFav    bool
		want     string
	}{
		{"not favorite, normal width", "hello", 7, false, "hello  "},
		{"favorite, normal width", "hello", 7, true, "★ hello"},
		{"favorite, width 2", "hello", 2, true, "★ "},
		{"favorite, width 1", "hello", 1, true, "★"},
		{"not favorite, width 1", "hello", 1, false, "…"},
		{"not favorite, width 0", "hello", 0, false, ""},
		{"favorite, width 0", "hello", 0, true, ""},
		{"favorite, wide chars", "你好", 5, true, "★ 你…"},
		{"not favorite, wide chars", "你好", 5, false, "你好 "},
		{"favorite, empty string", "", 4, true, "★   "},
		{"not favorite, empty string", "", 4, false, "    "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FavPrefix(tt.s, tt.maxWidth, tt.isFav)
			if got != tt.want {
				t.Errorf("FavPrefix(%q, %d, %v) = %q, want %q", tt.s, tt.maxWidth, tt.isFav, got, tt.want)
			}
			if tt.maxWidth > 0 {
				w := ansi.StringWidth(got)
				if w != tt.maxWidth {
					t.Errorf("FavPrefix(%q, %d, %v) display width = %d, want %d", tt.s, tt.maxWidth, tt.isFav, w, tt.maxWidth)
				}
			}
		})
	}
}
