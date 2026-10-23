package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/text"
)

// legendRowFor returns the rendered line holding the legend row for name,
// along with the column at which name starts on that line.
func legendRowFor(t *testing.T, out, name string) (string, int) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, name); i >= 0 {
			return line, i
		}
	}
	t.Fatalf("no legend row for %q in output:\n%s", name, out)
	return "", 0
}

func TestModesLegendOverlay_RendersEveryIndicator(t *testing.T) {
	cases := []struct {
		name                  string
		termWidth, termHeight int
		overlay               ModesLegendOverlay
		wantGlyphs            [4]string
		wantStates            [4]string
	}{
		{
			name:       "wide terminal",
			termWidth:  100,
			termHeight: 30,
			overlay:    ModesLegendOverlay{Repeat: playback.RepeatAll, Shuffle: true, StopAfter: false, Radio: true},
			wantGlyphs: [4]string{"R", "S", "N", "D"},
			wantStates: [4]string{"all", "on", "off", "on"},
		},
		{
			name:       "narrow terminal",
			termWidth:  60,
			termHeight: 30,
			overlay:    ModesLegendOverlay{Repeat: playback.RepeatOne, Shuffle: false, StopAfter: true, Radio: false},
			wantGlyphs: [4]string{"1", "S", "N", "D"},
			wantStates: [4]string{"one", "off", "on", "off"},
		},
	}

	wantNames := [4]string{"Repeat", "Shuffle", "Stop after track", "Auto-DJ"}
	wantKeys := [4]string{"r", "z", "ctrl+n", "A"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := ansi.Strip(tc.overlay.Render(tc.termWidth, tc.termHeight, newTestStyles()))

			for i, name := range wantNames {
				line, _ := legendRowFor(t, out, name)

				// name, live state and key share one padded value zone.
				want := text.Pad(name, modesLegendNameWidth) + text.Pad(tc.wantStates[i], modesLegendStateWidth) + wantKeys[i]
				if !strings.Contains(line, want) {
					t.Errorf("row %d: line %q does not contain %q", i, line, want)
				}

			}

			if !strings.Contains(out, "Playback Modes") {
				t.Errorf("expected title in output:\n%s", out)
			}
			if !strings.Contains(out, "esc close") {
				t.Errorf("expected dismiss hint in output:\n%s", out)
			}

			for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if w := ansi.StringWidth(line); w > tc.termWidth {
					t.Errorf("line %q is %d columns wide, exceeds terminal width %d", line, w, tc.termWidth)
				}
			}
		})
	}
}

// TestModesLegendOverlay_StylesGlyphsWithSemanticPalette pins that active
// glyphs use the unblended styles.Mode* family. The status bar's
// background-blended variants would emit a background SGR the overlay must not
// carry, since it sits on the normal background.
func TestModesLegendOverlay_StylesGlyphsWithSemanticPalette(t *testing.T) {
	st := newTestStyles()
	o := ModesLegendOverlay{Repeat: playback.RepeatAll, Shuffle: true, StopAfter: true, Radio: true}
	out := o.Render(100, 30, st)

	want := map[string]string{
		"repeat":    st.ModeRepeat.Render(text.Pad("Repeat", modesLegendNameWidth)),
		"shuffle":   st.ModeShuffle.Render(text.Pad("Shuffle", modesLegendNameWidth)),
		"stopAfter": st.ModeStopAfter.Render(text.Pad("Stop after track", modesLegendNameWidth)),
		"radio":     st.ModeRadio.Render(text.Pad("Auto-DJ", modesLegendNameWidth)),
	}
	for mode, glyph := range want {
		if !strings.Contains(out, glyph) {
			t.Errorf("%s: expected semantic-palette glyph %q in output:\n%q", mode, glyph, out)
		}
	}
}

// TestModesLegendOverlay_DimsInactiveGlyphs mirrors the status-bar cluster's
// encoding: an off mode reads as tertiary text, not its accent color.
func TestModesLegendOverlay_DimsInactiveGlyphs(t *testing.T) {
	st := newTestStyles()
	o := ModesLegendOverlay{Repeat: playback.RepeatOff, Shuffle: false, StopAfter: false, Radio: false}
	out := o.Render(100, 30, st)

	for _, glyph := range []string{"Repeat", "Shuffle", "Stop after track", "Auto-DJ"} {
		if !strings.Contains(out, st.TextTertiary.Render(text.Pad(glyph, modesLegendNameWidth))) {
			t.Errorf("expected dimmed glyph %q for an inactive mode", glyph)
		}
	}
	if strings.Contains(out, st.ModeShuffle.Render(text.Pad("Shuffle", modesLegendNameWidth))) {
		t.Error("inactive shuffle glyph must not use the active accent style")
	}
}
