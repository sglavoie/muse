package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// transientNotification holds a single pending notification for the status bar
// right zone. expiresAt is the time after which the notification should be
// cleared by the tick handler. A zero expiresAt means no notification is set.
type transientNotification struct {
	msg       string
	expiresAt time.Time
}

func newNotification(msg string) transientNotification {
	return transientNotification{msg: msg, expiresAt: time.Now().Add(transientDuration)}
}

func newNotificationFor(msg string, dur time.Duration) transientNotification {
	return transientNotification{msg: msg, expiresAt: time.Now().Add(dur)}
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
	watchDegraded bool
	notification  transientNotification

	// Persistent left zone fields — set during Update when track/state changes.
	artist         string
	title          string
	playlistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	playbackState PlaybackState
	repeatMode    RepeatMode
	shuffle       bool
	queueEmpty    bool

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
	s.notification = newNotification(text)
}

// handleMissingTrack sets the transient notification to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) handleMissingTrack(path string) {
	s.notification = newNotification(fmt.Sprintf("Error: track not found, skipped: %s", path))
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

// truncateTitle shortens s so its display width (measured in terminal columns)
// does not exceed maxWidth. When truncation occurs an ellipsis "…" (width 1)
// is appended. Returns an empty string when maxWidth <= 0.
func truncateTitle(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	// Accumulate display width rune-by-rune and truncate when the next
	// rune would exceed the budget (reserving 1 column for "…").
	budget := maxWidth - 1 // reserve space for ellipsis
	w := 0
	var out []rune
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if w+rw > budget {
			break
		}
		w += rw
		out = append(out, r)
	}
	return string(out) + "…"
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
	// Every text segment must carry the StatusBar background so that inner
	// ANSI resets (\e[0m) don't leave gaps with the terminal default bg.
	bg := st.StatusBar.GetBackground()
	fg := st.StatusBar.GetForeground()
	barText := lipgloss.NewStyle().Background(bg).Foreground(fg)
	barSuccess := st.Success.Background(bg)
	barWarning := st.Warning.Background(bg)
	barError := st.Error.Background(bg)
	barBold := barText.Bold(true)

	// r renders plain text with the StatusBar background+foreground.
	r := func(s string) string { return barText.Render(s) }

	// --- Right side (always shown) ---
	var rightSegments []string
	if s.configCreated {
		rightSegments = append(rightSegments, r(fmt.Sprintf("config created: %s", s.configPath)))
	}
	if s.watchDegraded {
		rightSegments = append(rightSegments, barWarning.Render("File watcher unavailable — press r to rescan manually"))
	}
	if s.notification.msg != "" {
		msg := s.notification.msg
		if strings.HasPrefix(msg, "Error:") {
			rightSegments = append(rightSegments, barError.Render(msg))
		} else {
			rightSegments = append(rightSegments, r(msg))
		}
	}
	right := strings.Join(rightSegments, r("  |  "))

	// rightWidth is the visible width of the right zone for truncation math.
	rightWidth := ansi.StringWidth(right)

	// pad fills the remaining terminal width with background-colored spaces.
	pad := func(content string) string {
		if width > 0 {
			visibleWidth := ansi.StringWidth(content)
			if visibleWidth < width {
				content = content + r(strings.Repeat(" ", width-visibleWidth))
			}
		}
		return content
	}

	// --- Persistent left zone ---
	if s.playbackState == PlaybackIdle && s.queueEmpty {
		hint := r("Queue is empty — press 1 to browse Library")
		if right != "" {
			return pad(hint + r("  |  ") + right)
		}
		return pad(hint)
	}
	if s.playbackState == PlaybackIdle && s.artist == "" && s.title == "" {
		return pad(right)
	}

	stateIcon := ""
	stateIconWidth := 0
	switch s.playbackState {
	case PlaybackPlaying:
		stateIcon = barSuccess.Render("▶")
		stateIconWidth = 1
	case PlaybackPaused:
		stateIcon = barWarning.Render("⏸")
		stateIconWidth = 1
	case PlaybackStopped:
		stateIcon = barError.Render("■")
		stateIconWidth = 1
	}

	// Build the suffix that follows the title: playlist source, repeat, shuffle.
	suffix := ""
	suffixWidth := 0
	if s.playlistSource != "" {
		suffix += r("  " + s.playlistSource)
		suffixWidth += 2 + ansi.StringWidth(s.playlistSource)
	}
	switch s.repeatMode {
	case RepeatAll:
		suffix += barBold.Render("  R")
		suffixWidth += 2 + 1
	case RepeatOne:
		suffix += barBold.Render("  R1")
		suffixWidth += 2 + 2
	}
	if s.shuffle {
		suffix += barBold.Render("  S")
		suffixWidth += 2 + 1
	}

	// Build the time display appended after the suffix.
	timeDisplay := ""
	timeDisplayWidth := 0
	if s.elapsed != "" && s.total != "" {
		timeDisplay = r("  " + s.elapsed + " / " + s.total)
		timeDisplayWidth = 2 + len(s.elapsed) + 3 + len(s.total)
	}

	// Build the prefix that precedes the title: glyph and artist.
	// Format: "[glyph] artist – title" or subsets when fields are absent.
	prefix := ""
	prefixWidth := 0
	sep := ""
	sepWidth := 0
	switch {
	case stateIcon != "" && s.artist != "":
		prefix = stateIcon + r(" "+s.artist)
		prefixWidth = stateIconWidth + 1 + ansi.StringWidth(s.artist)
		sep = " – "
		sepWidth = 3
	case stateIcon != "":
		prefix = stateIcon
		prefixWidth = stateIconWidth
	case s.artist != "":
		prefix = r(s.artist)
		prefixWidth = ansi.StringWidth(s.artist)
		sep = " – "
		sepWidth = 3
	}
	// sep is only meaningful when there is a title; clear it otherwise.
	if s.title == "" {
		sep = ""
		sepWidth = 0
	}

	title := s.title
	if width > 0 && s.title != "" {
		// Calculate how many runes are available for the title.
		// Full line: prefix + sep + title + suffix + timeDisplay + "  |  " + right
		const sepStr = "  |  "
		fixedWidth := prefixWidth + sepWidth +
			suffixWidth + timeDisplayWidth +
			ansi.StringWidth(sepStr) + rightWidth
		available := width - fixedWidth
		title = truncateTitle(s.title, available)
	}

	// Assemble left zone: each piece is already rendered with bg.
	var parts []string
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if sep != "" {
		parts = append(parts, r(sep))
	}
	if title != "" {
		parts = append(parts, r(title))
	}
	parts = append(parts, suffix, timeDisplay)

	left := strings.Join(parts, "")

	if ansi.StringWidth(left) == 0 {
		return pad(right)
	}
	return pad(left + r("  |  ") + right)
}
