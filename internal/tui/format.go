package tui

import "github.com/sglavoie/muse/internal/tui/text"

// trackDisplay returns a human-readable label for a track. It uses
// "Artist – Title" when both are present, falls back to whichever is
// available, and uses path as a last resort.
func trackDisplay(artist, title, path string) string {
	switch {
	case artist != "" && title != "":
		return artist + " – " + title
	case title != "":
		return title
	case artist != "":
		return artist
	default:
		return path
	}
}

// FormatDurationSecs converts a duration in seconds to M:SS or H:MM:SS format.
// Returns "--:--" when secs is zero or negative (unknown duration).
func FormatDurationSecs(secs float64) string { return text.FormatDurationSecs(secs) }

// FormatDurationMs converts a duration in milliseconds to M:SS or H:MM:SS
// format. Returns "--:--" when ms is zero or negative.
func FormatDurationMs(ms int64) string { return text.FormatDurationMs(ms) }
