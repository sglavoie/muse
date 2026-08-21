package tui

import "fmt"

// formatListeningTime formats a duration in seconds as "Xh Ym" or "Ym" or "Xs".
func formatListeningTime(secs int) string {
	if secs <= 0 {
		return "0s"
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

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
