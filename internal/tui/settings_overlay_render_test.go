package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestSettingsRenderContainsTitle(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := o.Render(80, 24, s)
	stripped := ansi.Strip(out)
	if !strings.Contains(stripped, "Settings") {
		t.Errorf("Render: expected title 'Settings', got:\n%s", stripped)
	}
}

func TestSettingsRenderContainsTabNames(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, name := range []string{"Playback", "Equalizer", "Display"} {
		if !strings.Contains(out, name) {
			t.Errorf("Render: expected tab name %q in output", name)
		}
	}
}

func TestSettingsRenderContainsFieldLabels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, label := range []string{"Volume step", "Seek short", "Seek long", "Default volume", "Normalize", "Gapless"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render: expected field label %q in output", label)
		}
	}
}

func TestSettingsRenderContainsBorderChars(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, ch := range []string{cornerTL, cornerBL, borderH, borderV} {
		if !strings.Contains(out, ch) {
			t.Errorf("Render: expected border char %q in output", ch)
		}
	}
}

func TestSettingsRenderContainsHint(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "ctrl+s save") {
		t.Errorf("Render: expected hint 'ctrl+s save' in output, got:\n%s", out)
	}
}

func TestSettingsRenderDisplayTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabDisplay
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "Notification") {
		t.Errorf("Render Display tab: expected 'Notification' field, got:\n%s", out)
	}
}

// At narrow terminal widths the tab bar, list rows, and field rows must be
// clamped to contentWidth so that the right border stays aligned with the
// top/bottom borders. Without truncation the tab row overflows the box.
func TestSettingsRenderClampsToBoxWidthAtNarrowTerm(t *testing.T) {
	cfg := config.Defaults()
	cfg.ExcludedPaths = []string{strings.Repeat("a-very-long-excluded-path-pattern/", 4)}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabLibrary
	o.libSubView = libSubViewExcluded
	s := styles.NewStyles(true)

	for _, w := range []int{24, 30, 40, 50} {
		out := ansi.Strip(o.Render(w, 24, s))
		// Find the top border line (begins with cornerTL) so we know the
		// expected box width, then assert every content line within the box
		// matches that width.
		var (
			boxWidth   int
			foundLines int
			mismatches []int
		)
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" {
				continue
			}
			lw := ansi.StringWidth(trimmed)
			if boxWidth == 0 && strings.HasPrefix(trimmed, cornerTL) {
				boxWidth = lw
				continue
			}
			// Skip lines that aren't part of the box (e.g., the hint line).
			if !strings.HasPrefix(trimmed, borderV) && !strings.HasPrefix(trimmed, cornerBL) && !strings.HasPrefix(trimmed, "├") {
				continue
			}
			foundLines++
			if lw != boxWidth {
				mismatches = append(mismatches, lw)
			}
		}
		if foundLines == 0 || boxWidth == 0 {
			t.Fatalf("termW=%d: failed to locate box lines in output:\n%s", w, out)
		}
		if len(mismatches) > 0 {
			t.Errorf("termW=%d: box width %d, but %d content line(s) have widths %v",
				w, boxWidth, len(mismatches), mismatches)
		}
	}
}

// The field-row rendering must also clamp at narrow widths (Equalizer has the
// longest tab content and many fields).
func TestSettingsRenderEqualizerTabClampsAtNarrowTerm(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)

	out := ansi.Strip(o.Render(28, 30, s))
	var (
		boxWidth   int
		mismatches []int
	)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		lw := ansi.StringWidth(trimmed)
		if boxWidth == 0 && strings.HasPrefix(trimmed, cornerTL) {
			boxWidth = lw
			continue
		}
		if !strings.HasPrefix(trimmed, borderV) && !strings.HasPrefix(trimmed, cornerBL) && !strings.HasPrefix(trimmed, "├") {
			continue
		}
		if lw != boxWidth {
			mismatches = append(mismatches, lw)
		}
	}
	if len(mismatches) > 0 {
		t.Errorf("equalizer tab at termW=28: box width %d, mismatching widths %v",
			boxWidth, mismatches)
	}
}

func TestStereoWideRenderShowsTenthUnit(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabPlayback
	for range 7 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "2.0x") {
		t.Errorf("Render: expected '2.0x' for stereo amount=20, got:\n%s", out)
	}
}
