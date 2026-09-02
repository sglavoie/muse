package uitest

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestGoToRenderHintFitsMinimumSupportedSize(t *testing.T) {
	m := overlay.NewGoToModal([]overlay.GoToOption{
		{Kind: overlay.GoToAlbum, Value: "Alpha"},
		{Kind: overlay.GoToArtist, Value: "Alice"},
		{Kind: overlay.GoToGenre, Value: "Ambient"},
	})
	s := styles.NewStyles(true)

	raw := m.Render(tui.MinTerminalWidth, tui.MinTerminalHeight, s)
	if issues := detectLayoutIssues(raw, plainScreen(raw), tui.MinTerminalWidth, tui.MinTerminalHeight); len(issues) != 0 {
		t.Fatalf("go-to overlay should fit at minimum supported size, got %+v:\n%s", issues, plainScreen(raw))
	}
}
