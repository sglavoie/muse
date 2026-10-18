package settings

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (o SettingsOverlay) editingText() bool {
	return o.LibDirInput != nil || o.ExclPathInput != nil || o.EQNameInput != nil || o.PrimaryInput != nil
}

// focusedDescription explains both the value and when the draft takes effect.
func (o SettingsOverlay) focusedDescription() string {
	if o.editingText() || o.confirmingDelete() {
		return ""
	}
	if o.Tab == tabLibrary {
		if o.libSubView == libSubViewExcluded {
			return "Excluded paths are skipped when scanning. Save to apply."
		}
		return "Music folders are scanned after saving. Press e to manage excluded paths."
	}
	if o.cursor < 0 || o.cursor >= len(o.Fields[o.Tab]) {
		return ""
	}
	f := o.Fields[o.Tab][o.cursor]
	if o.Tab == TabEqualizer && f.unit == "dB" {
		return "Boost or cut this frequency by up to 12 dB. Previews live when EQ is on; save to keep."
	}
	descriptions := map[string]string{
		"Volume step":    "Percentage points per volume key press. Applies after saving.",
		"Seek short":     "Seconds skipped with Left/Right. Applies after saving.",
		"Seek long":      "Seconds skipped with Shift+Left/Right. Applies after saving.",
		"Default volume": "Volume used when Muse starts, from 0 to 100%. Save for the next launch.",
		"Normalize":      "Balances loudness between songs using EBU R128. Applies after saving.",
		"Gapless":        "Preloads the next song for seamless transitions. Applies after saving.",
		"Stereo wide":    "Widens the stereo image. Previews live; save to keep.",
		"Width":          "Stereo width: 0x is mono, 1x is unchanged, up to 4x is wider. Previews live when enabled; save to keep.",
		"Enabled":        "Turns the ten-band equalizer on or off. Previews live; save to keep.",
		"Preset":         "Sets all ten EQ bands together. Previews live when EQ is on; save to keep.",
		"Notification":   "Seconds an action banner stays visible. Applies after saving.",
		"Vol feedback":   "Seconds the volume indicator stays visible. Applies after saving.",
		"Track notify":   "Desktop notifications when the song changes. Applies after saving.",
		"Theme":          "Auto follows the terminal background. Previews live; save to keep.",
		"Primary color":  "Accent color in #RGB or #RRGGBB; blank uses the theme default. Previews live; save to keep.",
	}
	return descriptions[f.label]
}

func (o SettingsOverlay) confirmingDelete() bool {
	return o.libConfirmDel || o.exclConfirmDel || o.EQConfirmDel
}

func (o SettingsOverlay) appendDescription(lines []string, contentWidth, room int, s styles.Styles) []string {
	description := o.focusedDescription()
	if room <= 0 || description == "" {
		return lines
	}
	wrapped := strings.Split(ansi.Wrap(description, contentWidth, ""), "\n")
	if len(wrapped) > min(3, room) {
		return lines
	}
	for _, line := range wrapped {
		lines = append(lines, overlay.BorderV+"  "+s.Dim.Render(text.Pad(line, contentWidth))+"  "+overlay.BorderV)
	}
	return lines
}
