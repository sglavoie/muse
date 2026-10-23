package settings

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestSettingsRenderContainsTitle(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := o.Render(80, 24, s)
	stripped := ansi.Strip(out)
	if !strings.Contains(stripped, "Settings") {
		t.Errorf("Render: expected title 'Settings', got:\n%s", stripped)
	}
}

func TestSettingsRenderContainsTabNames(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, name := range []string{"Playback", "Equalizer", "Display"} {
		if !strings.Contains(out, name) {
			t.Errorf("Render: expected tab name %q in output", name)
		}
	}
}

func TestSettingsRenderContainsFieldLabels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	o.Tab = TabPlayback
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, label := range []string{"Volume step", "Seek short", "Seek long", "Default volume", "Normalize", "Gapless"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render: expected field label %q in output", label)
		}
	}
}

func TestSettingsRenderContainsBorderChars(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, ch := range []string{overlay.CornerTL, overlay.CornerBL, overlay.BorderH, overlay.BorderV} {
		if !strings.Contains(out, ch) {
			t.Errorf("Render: expected border char %q in output", ch)
		}
	}
}

func TestSettingsRenderContainsHint(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "ctrl+s save") {
		t.Errorf("Render: expected hint 'ctrl+s save' in output, got:\n%s", out)
	}
}

// TestSettingsRenderAnchorsToTop pins the vertical placement: every tab starts
// on the first line, so switching between tabs of different heights (Library
// has two rows, Equalizer a dozen) changes only how far down the box reaches.
func TestSettingsRenderAnchorsToTop(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	for _, tab := range []Tab{tabLibrary, TabPlayback, TabEqualizer, TabDisplay} {
		o.Tab = tab
		first := ansi.Strip(strings.Split(o.Render(80, 40, s), "\n")[0])
		if !strings.HasPrefix(strings.TrimLeft(first, " "), overlay.CornerTL) {
			t.Errorf("tab %d: first rendered line = %q, want the top border", tab, first)
		}
	}
}

func TestSettingsRenderDisplayTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	o.Tab = TabDisplay
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
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab = tabLibrary
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
			if boxWidth == 0 && strings.HasPrefix(trimmed, overlay.CornerTL) {
				boxWidth = lw
				continue
			}
			// Skip lines that aren't part of the box (e.g., the hint line).
			if !strings.HasPrefix(trimmed, overlay.BorderV) && !strings.HasPrefix(trimmed, overlay.CornerBL) && !strings.HasPrefix(trimmed, "├") {
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
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab = TabEqualizer
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
		if boxWidth == 0 && strings.HasPrefix(trimmed, overlay.CornerTL) {
			boxWidth = lw
			continue
		}
		if !strings.HasPrefix(trimmed, overlay.BorderV) && !strings.HasPrefix(trimmed, overlay.CornerBL) && !strings.HasPrefix(trimmed, "├") {
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
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	o.Tab = TabPlayback
	for range 7 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "2.0x") {
		t.Errorf("Render: expected '2.0x' for stereo amount=20, got:\n%s", out)
	}
}

// settingsHintLine returns the key-hint line printed under the overlay box.
func settingsHintLine(t *testing.T, o SettingsOverlay) string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(ansi.Strip(o.Render(200, 40, styles.NewStyles(true))), "\n"), "\n")
	var hints []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "╭") && !strings.HasPrefix(line, "╰") && !strings.HasPrefix(line, "├") {
			hints = append(hints, line)
		}
	}
	return strings.Join(hints, " · ")
}

// TestSettingsHintsShareWordingAndOrder guards that every Settings tab words
// its shared keys the same way ("j/k navigate", "h/l adjust") and lists them
// in the same order, and that the Equalizer tells its preset keys apart from
// ctrl+s, which saves the settings.
func TestSettingsHintsShareWordingAndOrder(t *testing.T) {
	base := NewSettingsOverlay(config.Defaults(), config.Defaults().MusicDirectories)
	tabNames := [tabCount]string{"Library", "Playback", "Equalizer", "Display"}
	cases := map[string]SettingsOverlay{}
	for tab := tabLibrary; tab < tabCount; tab++ {
		o := base
		o.Tab = tab
		cases[tabNames[tab]] = o
	}
	excluded := base
	excluded.libSubView = libSubViewExcluded
	cases["Library excluded"] = excluded
	primary := base
	primary.Tab = TabDisplay
	primary.cursor = displayPrimary
	cases["Display primary color"] = primary

	for name, o := range cases {
		hint := o.keyHint()
		order := []string{"tab/shift+tab tabs", "j/k navigate", "ctrl+s save", "esc "}
		if strings.Contains(hint, "h/l") {
			order = []string{"tab/shift+tab tabs", "j/k navigate", "h/l adjust", "ctrl+s save", "esc "}
		}
		last := -1
		for _, want := range order {
			i := strings.Index(hint, want)
			if i < 0 {
				t.Errorf("%s: hint missing %q: %q", name, want, hint)
				continue
			}
			if i < last {
				t.Errorf("%s: %q out of order in %q", name, want, hint)
			}
			last = i
		}
	}

	eq := settingsHintLine(t, cases[tabNames[TabEqualizer]])
	for _, want := range []string{"s save preset", "d delete"} {
		if !strings.Contains(eq, want) {
			t.Errorf("Equalizer hint missing %q: %q", want, eq)
		}
	}
}
