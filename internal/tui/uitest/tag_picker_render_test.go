package uitest

import (
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/styles"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestTagPickerRenderHintFitsMinimumSupportedSize(t *testing.T) {
	tags := []db.TagRow{
		{ID: 1, Name: "chill"},
		{ID: 2, Name: "focus"},
		{ID: 3, Name: "energetic"},
	}
	tp, _ := tagtui.NewTagPicker(tags, "h1", map[int64]struct{}{})
	s := styles.NewStyles(true)

	raw := tp.Render(tui.MinTerminalWidth, tui.MinTerminalHeight, s)
	if issues := detectLayoutIssues(raw, plainScreen(raw), tui.MinTerminalWidth, tui.MinTerminalHeight); len(issues) != 0 {
		t.Fatalf("tag picker should fit at minimum supported size, got %+v:\n%s", issues, plainScreen(raw))
	}
}

func TestTagPickerLongListFitsMinimumOverlayHeight(t *testing.T) {
	items := make([]db.TagRow, 50)
	for i := range items {
		items[i] = db.TagRow{ID: int64(i + 1), Name: fmt.Sprintf("Tag %02d 音楽", i)}
	}
	tp, _ := tagtui.NewTagPickerBulk(items, []string{"a", "b"}, nil)
	tp.Cursor = 49
	const height = tui.MinTerminalHeight - 3
	raw := tp.Render(tui.MinTerminalWidth, height, styles.NewStyles(true))
	if issues := detectLayoutIssues(raw, plainScreen(raw), tui.MinTerminalWidth, height); len(issues) != 0 {
		t.Fatalf("long picker overflows: %+v:\n%s", issues, plainScreen(raw))
	}
}
