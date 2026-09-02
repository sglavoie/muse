package settings

import (
	"fmt"

	"github.com/sglavoie/muse/internal/config"
)

// applyEQFieldChange returns the appropriate action after an h/l keypress on
// the Equalizer tab, and applies side-effects (snap bands on preset change,
// reset preset to sentinel on band change).
func (o *SettingsOverlay) applyEQFieldChange() Action {
	eq := o.Fields[TabEqualizer]
	switch o.cursor {
	case 0: // Enabled toggle
		return ActionEQLive
	case 1: // Preset — snap all 10 band fields to the selected preset's bands
		presetIdx := eq[1].Value
		if presetIdx >= 0 && presetIdx < len(o.EQPresets) {
			p := o.EQPresets[presetIdx]
			// The "(Custom)" sentinel has zero-valued bands; preserve the
			// current user-edited band values instead of zeroing them.
			if p.Key != "custom" {
				for i := range 10 {
					o.Fields[TabEqualizer][i+2].Value = p.Bands[i]
				}
			}
		}
		return ActionEQLive
	default: // Band field (cursor 2–11) — revert preset display to "(Custom)"
		o.Fields[TabEqualizer][1].Value = len(o.EQPresets) - 1
		return ActionEQLive
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
		for _, p := range o.EQPresets {
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
		bands[i] = o.Fields[TabEqualizer][i+2].Value
	}

	newPreset := config.EQPresetDef{Name: name, Key: key, Bands: bands}
	sentinel := len(o.EQPresets) - 1
	newPresets := make([]config.EQPresetDef, 0, len(o.EQPresets)+1)
	newPresets = append(newPresets, o.EQPresets[:sentinel]...)
	newPresets = append(newPresets, newPreset)
	newPresets = append(newPresets, o.EQPresets[sentinel])
	o.EQPresets = newPresets

	o.Fields[TabEqualizer][1].max = len(o.EQPresets) - 1
	o.Fields[TabEqualizer][1].Value = sentinel // new preset index
	o.dirty = true
	return true
}

// deleteCurrentPreset removes the currently selected custom preset and falls back to Flat.
func (o *SettingsOverlay) deleteCurrentPreset() {
	presetIdx := o.Fields[TabEqualizer][1].Value
	newPresets := make([]config.EQPresetDef, 0, len(o.EQPresets)-1)
	newPresets = append(newPresets, o.EQPresets[:presetIdx]...)
	newPresets = append(newPresets, o.EQPresets[presetIdx+1:]...)
	o.EQPresets = newPresets

	o.Fields[TabEqualizer][1].max = len(o.EQPresets) - 1
	o.Fields[TabEqualizer][1].Value = 0 // fall back to Flat
	flat := o.EQPresets[0]
	for i := range 10 {
		o.Fields[TabEqualizer][i+2].Value = flat.Bands[i]
	}
	o.dirty = true
}
