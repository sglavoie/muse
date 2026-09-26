package config

import (
	"strings"
	"unicode"
)

// EQFrequencies holds the 10 standard equalizer band center frequencies in Hz.
var EQFrequencies = [10]int{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// EQPresetDef describes an equalizer preset, either built-in or user-defined.
type EQPresetDef struct {
	Name    string
	Key     string
	Bands   [10]int
	Builtin bool
}

// EQCustomPreset is the on-disk representation of a user-defined preset stored
// in the TOML config as [[eq_custom_presets]].
type EQCustomPreset struct {
	Name  string  `toml:"name"`
	Key   string  `toml:"key"`
	Bands [10]int `toml:"bands"`
}

// BuiltinEQPresets is the ordered list of read-only factory presets.
var BuiltinEQPresets = []EQPresetDef{
	{Name: "Flat", Key: "flat", Bands: [10]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Builtin: true},
	{Name: "Bass Boost", Key: "bass_boost", Bands: [10]int{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}, Builtin: true},
	{Name: "Treble Boost", Key: "treble_boost", Bands: [10]int{0, 0, 0, 0, 0, 0, 2, 4, 5, 6}, Builtin: true},
	{Name: "Rock", Key: "rock", Bands: [10]int{5, 4, 2, 0, -1, 0, 2, 3, 4, 5}, Builtin: true},
	{Name: "Jazz", Key: "jazz", Bands: [10]int{4, 3, 1, 2, -1, -1, 0, 1, 3, 4}, Builtin: true},
	{Name: "Classical", Key: "classical", Bands: [10]int{0, 0, 0, 0, 0, 0, -2, -2, -2, -4}, Builtin: true},
	{Name: "Pop", Key: "pop", Bands: [10]int{-1, 1, 3, 4, 3, 0, -1, -1, 1, 2}, Builtin: true},
}

// eqCustomKey is the eq_preset value for hand-tuned eq_bands that match no
// named preset; the settings overlay lists it as the "(Custom)" entry.
const eqCustomKey = "custom"

// EQPresetIndex returns the index of the preset with the given key in presets,
// or -1 if not found.
func EQPresetIndex(key string, presets []EQPresetDef) int {
	for i, p := range presets {
		if p.Key == key {
			return i
		}
	}
	return -1
}

// SlugFromName converts a human-readable name to a lowercase slug suitable for
// use as a preset key: spaces and non-alphanumeric characters are replaced with
// underscores, leading/trailing underscores are trimmed, and consecutive
// underscores are collapsed.
func SlugFromName(name string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore && b.Len() > 0 {
			b.WriteRune('_')
			prevUnderscore = true
		}
	}
	return strings.TrimRight(b.String(), "_")
}
