package text

import "testing"

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
		{"now-playing only", false, false, true, npIcon, " ♪"},
		{"focused and now-playing", false, true, true, npIcon, "▎♪"},
		{"selected only", true, false, false, npIcon, "● "},
		{"selected and focused", true, true, false, npIcon, "◉ "},
		{"selected and now-playing", true, false, true, npIcon, "●♪"},
		{"selected focused and now-playing", true, true, true, npIcon, "◉♪"},
		{"custom now-playing icon", false, false, true, "▶", " ▶"},
		{"empty icon falls back", false, false, true, "", " ♪"},
		{"wide icon leaves room for selection", true, true, true, "🎵", "◉♪"},
		{"overwide icon falls back", false, false, true, "🎵▶", " ♪"},
		{"zero-width icon falls back", false, false, true, "́", " ♪"},
		{"newline sanitized", false, false, true, "♪\n", " ♪"},
		{"tab sanitized", false, false, true, "♪\t", " ♪"},
		{"carriage return sanitized", false, false, true, "♪\r", " ♪"},
		{"whitespace falls back", false, false, true, "\n\t\r", " ♪"},
		{"multiline collapses", false, false, true, "a\nb", " a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RowMarker(tt.selected, tt.focused, tt.nowPlaying, tt.icon)
			if got != tt.want {
				t.Errorf("RowMarker(%v, %v, %v, %q) = %q, want %q",
					tt.selected, tt.focused, tt.nowPlaying, tt.icon, got, tt.want)
			}
			requireDisplayWidth(t, got, 2)
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
				requireDisplayWidth(t, got, tt.width)
			}
		})
	}
}
