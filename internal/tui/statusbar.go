package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// StatusBar manages the persistent status bar at the bottom of the screen.
// transientRight holds a temporary message (e.g. scan progress) that is
// cleared when the scan completes.
//
// The persistent left zone shows playback state, track artist/title, active
// playlist source, and repeat/shuffle mode indicators. The right zone shows
// auto-dismissing transient notifications.
type StatusBar struct {
	logPath        string
	configCreated  bool
	configPath     string
	transientRight string

	// Persistent left zone fields — set during Update when track/state changes.
	artist         string
	title          string
	playlistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	playbackState PlaybackState
	repeat        bool
	shuffle       bool

	// elapsed and total are pre-formatted MM:SS strings injected by
	// renderStatusBar. Both must be non-empty to show the time display.
	elapsed string
	total   string
}

// handleScanProgress sets the transient right zone to a formatted progress string.
// When msg.Total is known (> 0) the format is "Scanning… X / Y tracks";
// before the count arrives it falls back to "Scanning… X tracks".
func (s *StatusBar) handleScanProgress(msg library.ScanProgressMsg) {
	if msg.Total > 0 {
		s.transientRight = fmt.Sprintf("Scanning… %s / %s tracks", formatCount(msg.Scanned), formatCount(msg.Total))
	} else {
		s.transientRight = fmt.Sprintf("Scanning… %s tracks", formatCount(msg.Scanned))
	}
}

// handleScanDone clears the transient right zone.
func (s *StatusBar) handleScanDone() {
	s.transientRight = ""
}

// handleMissingTrack sets the transient right zone to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) handleMissingTrack(path string) {
	s.transientRight = fmt.Sprintf("Error: track not found, skipped: %s", path)
}

// formatTime converts a duration in seconds to MM:SS format.
func formatTime(secs float64) string {
	s := int(secs)
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// truncateTitle shortens s to at most maxRunes runes, appending "…" when
// truncation occurs. Returns an empty string when maxRunes <= 0.
func truncateTitle(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

// render builds the full status bar string. The persistent left zone shows
// playback state indicator, track info, playlist source, and mode indicators.
// The right zone shows transient notifications.
//
// width is the available terminal width in columns. When width is 0 the
// content is rendered without any truncation. When width > 0 the track title
// is truncated first (with a trailing "…") to make the full line fit, while
// the artist name is always preserved.
func (s StatusBar) render(width int) string {
	// --- Right side (always shown) ---
	logPart := fmt.Sprintf("log: %s", s.logPath)
	if s.configCreated {
		logPart += fmt.Sprintf("  |  config created: %s", s.configPath)
	}
	rightSegments := []string{logPart}
	if s.transientRight != "" {
		rightSegments = append(rightSegments, s.transientRight)
	}
	right := strings.Join(rightSegments, "  |  ")

	// --- Persistent left zone ---
	if s.playbackState == PlaybackIdle && s.artist == "" && s.title == "" {
		return right
	}

	stateIcon := ""
	switch s.playbackState {
	case PlaybackPlaying:
		stateIcon = "▶"
	case PlaybackPaused:
		stateIcon = "⏸"
	case PlaybackStopped:
		stateIcon = "■"
	}

	// Build the suffix that follows the title: playlist source, repeat, shuffle.
	suffix := ""
	if s.playlistSource != "" {
		suffix += "  " + s.playlistSource
	}
	if s.repeat {
		suffix += "  R"
	}
	if s.shuffle {
		suffix += "  S"
	}

	// Build the time display appended after the suffix.
	timeDisplay := ""
	if s.elapsed != "" && s.total != "" {
		timeDisplay = "  " + s.elapsed + " / " + s.total
	}

	// Build the prefix that precedes the title: glyph and artist.
	// Format: "[glyph] artist – title" or subsets when fields are absent.
	prefix := ""
	sep := ""
	switch {
	case stateIcon != "" && s.artist != "":
		prefix = stateIcon + " " + s.artist
		sep = " – "
	case stateIcon != "":
		prefix = stateIcon
	case s.artist != "":
		prefix = s.artist
		sep = " – "
	}
	// sep is only meaningful when there is a title; clear it otherwise.
	if s.title == "" {
		sep = ""
	}

	title := s.title
	if width > 0 && s.title != "" {
		// Calculate how many runes are available for the title.
		// Full line: prefix + sep + title + suffix + timeDisplay + "  |  " + right
		const sepStr = "  |  "
		fixedRunes := len([]rune(prefix)) + len([]rune(sep)) +
			len([]rune(suffix)) + len([]rune(timeDisplay)) +
			len([]rune(sepStr)) + len([]rune(right))
		available := width - fixedRunes
		title = truncateTitle(s.title, available)
	}

	left := prefix + sep + title + suffix + timeDisplay
	// Trim leading/trailing spaces that can arise when prefix or title is empty.
	left = strings.TrimSpace(left)

	if left == "" {
		return right
	}
	return left + "  |  " + right
}
