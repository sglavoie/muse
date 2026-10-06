package uitest

import (
	"testing"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/settings"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestSettingsRenderHintFitsMinimumSupportedSize(t *testing.T) {
	o := settings.NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)

	raw := o.Render(tui.MinTerminalWidth, tui.MinTerminalHeight, s)
	if issues := detectLayoutIssues(raw, plainScreen(raw), tui.MinTerminalWidth, tui.MinTerminalHeight); len(issues) != 0 {
		t.Fatalf("settings overlay should fit at minimum supported size, got %+v:\n%s", issues, plainScreen(raw))
	}
}
