package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestTagPickerRenderHintFitsMinimumSupportedSize(t *testing.T) {
	tags := []db.TagRow{
		{ID: 1, Name: "chill"},
		{ID: 2, Name: "focus"},
		{ID: 3, Name: "energetic"},
	}
	tp, _ := NewTagPicker(tags, "h1", map[int64]struct{}{})
	s := styles.NewStyles(true)

	raw := tp.Render(minTerminalWidth, minTerminalHeight, s)
	if issues := DetectLayoutIssues(raw, PlainScreen(raw), minTerminalWidth, minTerminalHeight); len(issues) != 0 {
		t.Fatalf("tag picker should fit at minimum supported size, got %+v:\n%s", issues, PlainScreen(raw))
	}
}
