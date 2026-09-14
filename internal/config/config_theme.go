package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// NormalizePrimaryColor accepts an empty preset default, #RGB, or #RRGGBB.
// Nonempty values are expanded to lowercase #rrggbb for storage and display.
func NormalizePrimaryColor(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", true
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
		*warnings = append(*warnings, "primary_color must be #RGB or #RRGGBB; using preset default")
	}
}
