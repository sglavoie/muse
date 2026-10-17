package settings

import (
	"reflect"
	"slices"
)

// Compare opening values, not the historical dirty flag: restoring a value
// removes its marker, tab badge and contribution to the unsaved change count.
func (o SettingsOverlay) changedFields() [tabCount][]bool {
	initial := NewSettingsOverlay(o.initialConfig, o.initialConfig.MusicDirectories)
	var changes [tabCount][]bool
	changes[tabLibrary] = []bool{
		!slices.Equal(o.LibraryDirs, initial.LibraryDirs),
		!slices.Equal(o.ExcludedPaths, initial.ExcludedPaths),
	}
	for tab := TabPlayback; tab < tabCount; tab++ {
		changes[tab] = make([]bool, len(o.Fields[tab]))
		for i, field := range o.Fields[tab] {
			changes[tab][i] = field.Value != initial.Fields[tab][i].Value
		}
	}
	current := o.ToConfig(o.initialConfig)
	changes[TabEqualizer][1] = current.EQPreset != o.initialConfig.EQPreset ||
		!reflect.DeepEqual(current.EQCustomPresets, o.initialConfig.EQCustomPresets)
	changes[TabDisplay][displayPrimary] = o.primaryColor != o.initialConfig.PrimaryColor
	return changes
}

func changedCount(fields []bool) int {
	count := 0
	for _, changed := range fields {
		if changed {
			count++
		}
	}
	return count
}

func changedValueMark(value string, initial []string) string {
	if !slices.Contains(initial, value) {
		return " *"
	}
	return ""
}
