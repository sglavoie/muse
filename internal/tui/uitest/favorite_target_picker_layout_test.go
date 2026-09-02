package uitest

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestFavoriteTargetPickerCancelAndRenderNarrow(t *testing.T) {
	p := favtui.NewTargetPicker(
		[]library.Track{{Hash: "h1", Title: strings.Repeat("界", 40), Artist: "A", Album: "B"}},
		map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, false,
		favtui.PickerOrigin{},
	)
	for _, key := range []string{"esc", "F"} {
		_, cmd := p.HandleKey(key)
		if _, ok := cmd().(favtui.TargetDismissedMsg); !ok {
			t.Fatalf("%s did not dismiss picker", key)
		}
	}
	raw := p.Render(tui.MinTerminalWidth, tui.MinTerminalHeight, styles.NewStyles(true))
	if issues := detectLayoutIssues(raw, plainScreen(raw), tui.MinTerminalWidth, tui.MinTerminalHeight); len(issues) != 0 {
		t.Fatalf("favorite target picker should fit at minimum supported size, got %+v:\n%s", issues, plainScreen(raw))
	}
}
