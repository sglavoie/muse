package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/config"
)

// applyEQFieldChange returns the appropriate action after an h/l keypress on
// the Equalizer tab, and applies side-effects (snap bands on preset change,
// reset preset to sentinel on band change).
func (o *SettingsOverlay) applyEQFieldChange() settingsAction {
	eq := o.fields[settingsTabEqualizer]
	switch o.cursor {
	case 0: // Enabled toggle
		return settingsEQLive
	case 1: // Preset — snap all 10 band fields to the selected preset's bands
		presetIdx := eq[1].Value
		if presetIdx >= 0 && presetIdx < len(o.eqPresets) {
			p := o.eqPresets[presetIdx]
			// The "(Custom)" sentinel has zero-valued bands; preserve the
			// current user-edited band values instead of zeroing them.
			if p.Key != "custom" {
				for i := range 10 {
					o.fields[settingsTabEqualizer][i+2].Value = p.Bands[i]
				}
			}
		}
		return settingsEQLive
	default: // Band field (cursor 2–11) — revert preset display to "(Custom)"
		o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
		return settingsEQLive
	}
}

// addCustomPreset adds a new custom preset with the given name using the current band values.
// It inserts before the "(Custom)" sentinel and updates the preset field accordingly.
func (o *SettingsOverlay) addCustomPreset(name string) bool {
	base := config.SlugFromName(name)
	if base == "" {
		return false
	}
	key := base
	for n := 2; ; n++ {
		found := false
		for _, p := range o.eqPresets {
			if p.Key == key {
				found = true
				break
			}
		}
		if !found {
			break
		}
		key = fmt.Sprintf("%s_%d", base, n)
	}

	var bands [10]int
	for i := range 10 {
		bands[i] = o.fields[settingsTabEqualizer][i+2].Value
	}

	newPreset := config.EQPresetDef{Name: name, Key: key, Bands: bands}
	sentinel := len(o.eqPresets) - 1
	newPresets := make([]config.EQPresetDef, 0, len(o.eqPresets)+1)
	newPresets = append(newPresets, o.eqPresets[:sentinel]...)
	newPresets = append(newPresets, newPreset)
	newPresets = append(newPresets, o.eqPresets[sentinel])
	o.eqPresets = newPresets

	o.fields[settingsTabEqualizer][1].Max = len(o.eqPresets) - 1
	o.fields[settingsTabEqualizer][1].Value = sentinel // new preset index
	o.dirty = true
	return true
}

// deleteCurrentPreset removes the currently selected custom preset and falls back to Flat.
func (o *SettingsOverlay) deleteCurrentPreset() {
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	newPresets := make([]config.EQPresetDef, 0, len(o.eqPresets)-1)
	newPresets = append(newPresets, o.eqPresets[:presetIdx]...)
	newPresets = append(newPresets, o.eqPresets[presetIdx+1:]...)
	o.eqPresets = newPresets

	o.fields[settingsTabEqualizer][1].Max = len(o.eqPresets) - 1
	o.fields[settingsTabEqualizer][1].Value = 0 // fall back to Flat
	flat := o.eqPresets[0]
	for i := range 10 {
		o.fields[settingsTabEqualizer][i+2].Value = flat.Bands[i]
	}
	o.dirty = true
}
