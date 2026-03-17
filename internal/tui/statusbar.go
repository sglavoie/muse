package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// transientDuration is how long a transient notification stays visible before
// the 1-second tick clears it.
const transientDuration = 5 * time.Second

// transientNotification holds a single pending notification for the status bar
// right zone. expiresAt is the time after which the notification should be
// cleared by the tick handler. A zero expiresAt means no notification is set.
type transientNotification struct {
	msg       string
	expiresAt time.Time
}

// StatusBar manages the persistent status bar at the bottom of the screen.
// notification holds a temporary message (e.g. scan progress, error) that
// auto-dismisses after transientDuration via the 1-second tick handler.
//
// The persistent left zone shows playback state, track artist/title, active
// playlist source, and repeat/shuffle mode indicators. The right zone shows
// auto-dismissing transient notifications.
type StatusBar struct {
	configCreated bool
	configPath    string
	notification  transientNotification

	// Persistent left zone fields — set during Update when track/state changes.
	artist         string
	title          string
	playlistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	playbackState PlaybackState
	repeatMode    RepeatMode
	shuffle       bool

	// elapsed and total are pre-formatted MM:SS strings injected by
	// renderStatusBar. Both must be non-empty to show the time display.
	elapsed string
	total   string
}

// handleScanProgress sets the transient notification to a formatted progress
// string with a 5-second expiry. New calls overwrite the previous notification.
// When msg.Total is known (> 0) the format is "Scanning… X / Y tracks";
// before the count arrives it falls back to "Scanning… X tracks".
func (s *StatusBar) handleScanProgress(msg library.ScanProgressMsg) {
	var text string
	if msg.Total > 0 {
		text = fmt.Sprintf("Scanning… %s / %s tracks", formatCount(msg.Scanned), formatCount(msg.Total))
	} else {
		text = fmt.Sprintf("Scanning… %s tracks", formatCount(msg.Scanned))
	}
	s.notification = transientNotification{msg: text, expiresAt: time.Now().Add(transientDuration)}
}

// handleMissingTrack sets the transient notification to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) handleMissingTrack(path string) {
	s.notification = transientNotification{
		msg:       fmt.Sprintf("Error: track not found, skipped: %s", path),
		expiresAt: time.Now().Add(transientDuration),
	}
}

// formatTime converts a duration in seconds to M:SS or H:MM:SS format.
// Unlike FormatDurationSecs, zero is treated as a valid position (start of
// playback) and returns "0:00".
func formatTime(secs float64) string {
	if secs < 0 {
		secs = 0
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
func (s StatusBar) render(width int, st styles.Styles) string {
	// --- Right side (always shown) ---
	var rightSegments []string
	if s.configCreated {
		rightSegments = append(rightSegments, fmt.Sprintf("config created: %s", s.configPath))
	}
	if s.notification.msg != "" {
		msg := s.notification.msg
		if strings.HasPrefix(msg, "Error:") {
			msg = st.Error.Render(msg)
		}
		rightSegments = append(rightSegments, msg)
	}
	right := strings.Join(rightSegments, "  |  ")

	// padAndWrap pads result to full width and wraps with StatusBar style.
	padAndWrap := func(result string) string {
		if width > 0 {
			visibleWidth := ansi.StringWidth(result)
			if visibleWidth < width {
				result = result + strings.Repeat(" ", width-visibleWidth)
			}
		}
		return st.StatusBar.Render(result)
	}

	// --- Persistent left zone ---
	if s.playbackState == PlaybackIdle && s.artist == "" && s.title == "" {
		return padAndWrap(right)
	}

	stateIcon := ""
	stateIconWidth := 0
	switch s.playbackState {
	case PlaybackPlaying:
		stateIcon = st.Success.Render("▶")
		stateIconWidth = 1
	case PlaybackPaused:
		stateIcon = st.Warning.Render("⏸")
		stateIconWidth = 1
	case PlaybackStopped:
		stateIcon = st.Error.Render("■")
		stateIconWidth = 1
	}

	// Build the suffix that follows the title: playlist source, repeat, shuffle.
	suffix := ""
	suffixWidth := 0
	if s.playlistSource != "" {
		suffix += "  " + s.playlistSource
		suffixWidth += 2 + len([]rune(s.playlistSource))
	}
	switch s.repeatMode {
	case RepeatAll:
		styled := st.Bold.Render("R")
		suffix += "  " + styled
		suffixWidth += 2 + 1
	case RepeatOne:
		styled := st.Bold.Render("R1")
		suffix += "  " + styled
		suffixWidth += 2 + 2
	}
	if s.shuffle {
		styled := st.Bold.Render("S")
		suffix += "  " + styled
		suffixWidth += 2 + 1
	}

	// Build the time display appended after the suffix.
	timeDisplay := ""
	timeDisplayWidth := 0
	if s.elapsed != "" && s.total != "" {
		timeDisplay = "  " + s.elapsed + " / " + s.total
		timeDisplayWidth = 2 + len(s.elapsed) + 3 + len(s.total)
	}

	// Build the prefix that precedes the title: glyph and artist.
	// Format: "[glyph] artist – title" or subsets when fields are absent.
	prefix := ""
	prefixWidth := 0
	sep := ""
	switch {
	case stateIcon != "" && s.artist != "":
		prefix = stateIcon + " " + s.artist
		prefixWidth = stateIconWidth + 1 + len([]rune(s.artist))
		sep = " – "
	case stateIcon != "":
		prefix = stateIcon
		prefixWidth = stateIconWidth
	case s.artist != "":
		prefix = s.artist
		prefixWidth = len([]rune(s.artist))
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
		fixedWidth := prefixWidth + len([]rune(sep)) +
			suffixWidth + timeDisplayWidth +
			len([]rune(sepStr)) + ansi.StringWidth(right)
		available := width - fixedWidth
		title = truncateTitle(s.title, available)
	}

	left := prefix + sep + title + suffix + timeDisplay
	// Trim leading/trailing spaces that can arise when prefix or title is empty.
	left = strings.TrimSpace(left)

	if left == "" {
		return padAndWrap(right)
	}
	return padAndWrap(left + "  |  " + right)
}
