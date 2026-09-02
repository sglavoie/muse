package tui

import (
	"testing"

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

	raw := m.Render(minTerminalWidth, minTerminalHeight, s)
	if issues := DetectLayoutIssues(raw, PlainScreen(raw), minTerminalWidth, minTerminalHeight); len(issues) != 0 {
		t.Fatalf("go-to overlay should fit at minimum supported size, got %+v:\n%s", issues, PlainScreen(raw))
	}
}
