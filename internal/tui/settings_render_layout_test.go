package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/settings"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestSettingsRenderHintFitsMinimumSupportedSize(t *testing.T) {
	o := settings.NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)

	raw := o.Render(minTerminalWidth, minTerminalHeight, s)
	if issues := DetectLayoutIssues(raw, PlainScreen(raw), minTerminalWidth, minTerminalHeight); len(issues) != 0 {
		t.Fatalf("settings overlay should fit at minimum supported size, got %+v:\n%s", issues, PlainScreen(raw))
	}
}
