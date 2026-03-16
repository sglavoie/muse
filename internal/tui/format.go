package tui

import "fmt"

// FormatDurationSecs converts a duration in seconds to M:SS or H:MM:SS format.
// Returns "--:--" when secs is zero or negative (unknown duration).
func FormatDurationSecs(secs float64) string {
	if secs <= 0 {
		return "--:--"
	}
	total := int(secs)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatDurationMs converts a duration in milliseconds to M:SS or H:MM:SS
// format. Returns "--:--" when ms is zero or negative.
func FormatDurationMs(ms int64) string {
	if ms <= 0 {
		return "--:--"
	}
	totalSec := ms / 1000
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
