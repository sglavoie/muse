package config

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// AccentPresets lists the named accents primary_color accepts besides a hex
// color, in the order Settings cycles through them. Each has a tuned shade for
// the light and the dark preset. The first, blue, is the preset default and
// is stored as "".
var AccentPresets = []string{
	"blue", "teal", "green", "olive", "yellow", "sand", "orange", "brown",
	"red", "pink", "purple", "grey", "white",
}

// NormalizePrimaryColor accepts an empty preset default, an AccentPresets
// name, #RGB, or #RRGGBB. Hex values are expanded to lowercase #rrggbb and
// names are lowercased; the default accent's name normalizes to "".
func NormalizePrimaryColor(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == AccentPresets[0] {
		return "", true
	}
	if slices.Contains(AccentPresets, value) {
		return value, true
	}
	if (len(value) != 4 && len(value) != 7) || value[0] != '#' {
		return "", false
	}
	for _, c := range value[1:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", false
		}
	}
	if len(value) == 4 {
		value = fmt.Sprintf("#%c%c%c%c%c%c", value[1], value[1], value[2], value[2], value[3], value[3])
	}
	return value, true
}

func validateTheme(raw rawConfig, cfg *Config, warnings *[]string) {
	name := strings.ToLower(strings.TrimSpace(raw.Theme))
	switch name {
	case "":
	case "auto", "dark", "light":
		cfg.Theme = name
	default:
		slog.Warn("invalid theme, using auto", "theme", raw.Theme)
		*warnings = append(*warnings, fmt.Sprintf("theme %q is not recognized (use auto/dark/light), using auto", raw.Theme))
	}
	if primary, ok := NormalizePrimaryColor(raw.PrimaryColor); ok {
		cfg.PrimaryColor = primary
	} else {
		slog.Warn("invalid primary_color, using preset default", "primary_color", raw.PrimaryColor)
		*warnings = append(*warnings, "primary_color must be #RGB, #RRGGBB or an accent name ("+strings.Join(AccentPresets, ", ")+"); using preset default")
	}
}
