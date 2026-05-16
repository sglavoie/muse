package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// notifPriority controls which notification wins when multiple are queued.
// Higher values win; the queue evicts the lowest-priority entry when full.
type notifPriority uint8

const (
	notifInfo   notifPriority = iota // scan progress, sort, volume, mode toggles
	notifNormal                      // action results, errors, undo prompts
	notifUndo                        // reserved: never-overwritten slot (unused — kept for future use)
)

// transientNotification holds a single pending notification for the status bar
// right zone. expiresAt is the time after which the notification should be
// cleared by the tick handler. A zero expiresAt means no notification is set.
type transientNotification struct {
	msg       string
	expiresAt time.Time
	priority  notifPriority
}

// pushMsg enqueues a notification with an explicit duration and priority.
// The queue is capped at 5 entries; when full the lowest-priority oldest entry
// is evicted. If all existing entries outrank the incoming one it is dropped.
func (s *StatusBar) pushMsg(msg string, dur time.Duration, priority notifPriority) {
	const maxQueue = 5
	n := transientNotification{msg: msg, expiresAt: time.Now().Add(dur), priority: priority}
	// Prune expired entries first to make room.
	s.pruneNotifications()
	if len(s.notifications) < maxQueue {
		s.notifications = append(s.notifications, n)
		return
	}
	// At capacity — evict the lowest-priority oldest entry (lowest index wins tie).
	evictIdx := -1
	var evictPrio notifPriority
	for i, existing := range s.notifications {
		if evictIdx < 0 || existing.priority < evictPrio {
			evictIdx = i
			evictPrio = existing.priority
		}
	}
	if evictIdx >= 0 && evictPrio <= priority {
		s.notifications = append(s.notifications[:evictIdx], s.notifications[evictIdx+1:]...)
		s.notifications = append(s.notifications, n)
	}
	// If all existing entries outrank the incoming one, drop it silently.
}

// isExpired reports whether n has expired. A zero expiresAt is treated as
// "no expiry" and is never expired (used for direct-assignment test helpers).
func (n transientNotification) isExpired() bool {
	return !n.expiresAt.IsZero() && time.Now().After(n.expiresAt)
}

// pruneNotifications removes expired entries from the notifications queue.
func (s *StatusBar) pruneNotifications() {
	i := 0
	for _, notif := range s.notifications {
		if !notif.isExpired() {
			s.notifications[i] = notif
			i++
		}
	}
	s.notifications = s.notifications[:i]
}

// topNotification returns the highest-priority non-expired notification, or
// a zero value when the queue is empty or all entries have expired.
// Among entries with equal priority, the last-pushed one wins.
func (s StatusBar) topNotification() transientNotification {
	var top transientNotification
	for _, notif := range s.notifications {
		if notif.isExpired() {
			continue
		}
		if top.msg == "" || notif.priority >= top.priority {
			top = notif
		}
	}
	return top
}

// notify enqueues a notifNormal notification using the configured transient
// duration. Falls back to the default 5s constant when transientDur is zero
// (e.g. in tests that do not set config).
func (s *StatusBar) notify(msg string) {
	dur := s.transientDur
	if dur == 0 {
		dur = transientDuration
	}
	s.pushMsg(msg, dur, notifNormal)
}

// notifyInfo enqueues a notifInfo notification (scan, sort, volume, mode
// toggles). These yield to notifNormal when both are queued.
func (s *StatusBar) notifyInfo(msg string) {
	dur := s.transientDur
	if dur == 0 {
		dur = transientDuration
	}
	s.pushMsg(msg, dur, notifInfo)
}

// StatusBar manages the persistent status bar at the bottom of the screen.
// notification holds a temporary message (e.g. scan progress, error) that
// auto-dismisses after transientDur via the 1-second tick handler.
//
// The persistent left zone shows playback state, track artist/title, active
// playlist source, and repeat/shuffle mode indicators. The right zone shows
// auto-dismissing transient notifications.
type StatusBar struct {
	configCreated bool
	configPath    string
	watchDegraded bool
	notifications []transientNotification
	transientDur  time.Duration // configurable notification duration

	// undoExpiresAt is set by renderStatusBar when an undo slot is active.
	// The render() method appends a seconds countdown to the undo notification.
	undoExpiresAt time.Time

	// Persistent left zone fields — set during Update when track/state changes.
	artist         string
	title          string
	album          string
	playlistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	playbackState    PlaybackState
	repeatMode       RepeatMode
	shuffle          bool
	stopAfterCurrent bool
	visualMode       bool
	queueEmpty       bool

	// elapsed and total are pre-formatted MM:SS strings injected by
	// renderStatusBar. Both must be non-empty to show the time display.
	elapsed string
	total   string

	// elapsedSecs and totalSecs are the raw float64 values injected by
	// renderStatusBar for use in renderProgressBar.
	elapsedSecs float64
	totalSecs   float64

	// activeTagName is non-empty when a cross-view tag filter is active.
	// Injected by renderStatusBar from the root Model.
	activeTagName string

	// radioMode is true when Auto-DJ / radio mode is active.
	// Injected by renderStatusBar from the root Model.
	radioMode bool

	// queuePosition is the formatted "N of M" queue position string injected
	// by renderStatusBar. Empty when nothing is playing.
	queuePosition string

	// yankCount, yankIsCut, and selectionCount are injected by renderStatusBar.
	yankCount      int
	yankIsCut      bool
	selectionCount int
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
	s.notifyInfo(text)
}

// handleMissingTrack sets the transient notification to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) handleMissingTrack(path string) {
	s.notify(fmt.Sprintf("Error: track not found, skipped: %s", path))
}

// formatTime converts a duration in seconds to M:SS or H:MM:SS format.
// Unlike FormatDurationSecs, zero is treated as a valid position (start of
// playback) and returns "0:00". NaN and infinite values are clamped to zero.
func formatTime(secs float64) string {
	if secs < 0 || math.IsNaN(secs) || math.IsInf(secs, 0) {
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

// renderProgressBar builds a full-width progress bar string. The filled portion
// uses "█" styled with ProgressFilled; the empty portion uses "░" with
// ProgressEmpty. elapsed/total are seconds; fraction is clamped to [0,1].
func renderProgressBar(elapsed, total float64, width int, st styles.Styles) string {
	frac := 0.0
	if total > 0 {
		frac = elapsed / total
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
	}
	filled := int(math.Round(frac * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return st.ProgressFilled.Render(strings.Repeat("█", filled)) +
		st.ProgressEmpty.Render(strings.Repeat("░", empty))
}

// truncateTitle shortens s so its display width (measured in terminal columns)
// does not exceed maxWidth. When truncation occurs an ellipsis "…" (width 1)
// is appended. Returns an empty string when maxWidth <= 0.
func truncateTitle(s string, maxWidth int) string {
	return text.Truncate(s, maxWidth)
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
	// The base style shifts slightly for playing (warm) and stopped (cool).
	var baseSb lipgloss.Style
	switch s.playbackState {
	case PlaybackPlaying:
		baseSb = st.StatusBarPlayingBg
	case PlaybackStopped:
		baseSb = st.StatusBarStoppedBg
	default:
		baseSb = st.StatusBar
	}
	bg := baseSb.GetBackground()
	fg := baseSb.GetForeground()
	barText := lipgloss.NewStyle().Background(bg).Foreground(fg)
	barSuccess := st.Success.Background(bg)
	barWarning := st.Warning.Background(bg)
	barError := st.Error.Background(bg)
	barBold := barText.Bold(true)
	barDim := barText.Faint(true)
	barModeRepeat := st.ModeRepeat.Background(bg)
	barModeShuffle := st.ModeShuffle.Background(bg)
	barModeStopAfter := st.ModeStopAfter.Background(bg)
	barModeRadio := st.ModeRadio.Background(bg)

	// modeStyle returns the colored bold style for a given mode label text.
	modeStyle := func(label string) lipgloss.Style {
		switch label {
		case "Repeat", "Repeat 1", "R", "R1":
			return barModeRepeat
		case "Shuffle", "S":
			return barModeShuffle
		case "Stop Next", "N":
			return barModeStopAfter
		case "Auto-DJ", "DJ":
			return barModeRadio
		default:
			return barBold
		}
	}

	// r renders plain text with the StatusBar background+foreground.
	r := func(s string) string { return barText.Render(s) }

	// dimLine returns a full-width dim separator used as the line-2 fallback.
	dimLine := func() string {
		if width > 0 {
			return st.Dim.Render(strings.Repeat("─", width))
		}
		return st.Dim.Render("─")
	}

	// --- Right side (notification badge, right-aligned) ---
	notif := st.StatusNotification
	notifErr := st.StatusNotificationError
	var rightSegments []string
	if s.configCreated {
		rightSegments = append(rightSegments, notif.Render(fmt.Sprintf(" config created: %s ", s.configPath)))
	}
	if s.watchDegraded {
		rightSegments = append(rightSegments, notifErr.Render(" File watcher unavailable — press r to rescan manually "))
	}
	if top := s.topNotification(); top.msg != "" {
		msg := top.msg
		if !s.undoExpiresAt.IsZero() {
			if remaining := time.Until(s.undoExpiresAt); remaining > 0 {
				msg = fmt.Sprintf("%s (%ds)", msg, int(math.Ceil(remaining.Seconds())))
			}
		}
		if strings.HasPrefix(msg, "Error:") {
			rightSegments = append(rightSegments, notifErr.Render(" "+msg+" "))
		} else {
			rightSegments = append(rightSegments, notif.Render(" "+msg+" "))
		}
	}
	right := strings.Join(rightSegments, r(" "))

	// rightWidth is the visible width of the right zone for truncation math.
	rightWidth := ansi.StringWidth(right)

	// pad fills the remaining terminal width with background-colored spaces.
	pad := func(content string) string {
		if width > 0 {
			visibleWidth := ansi.StringWidth(content)
			if visibleWidth < width {
				content += r(strings.Repeat(" ", width-visibleWidth))
			}
		}
		return content
	}

	// rightAlign builds a full-width line with left content and right-aligned notification.
	rightAlign := func(left string) string {
		if rightWidth == 0 {
			return pad(left)
		}
		if width == 0 {
			// Unconstrained: append notification after a small gap.
			if left == "" {
				return right
			}
			return left + r("  ") + right
		}
		leftW := ansi.StringWidth(left)
		gap := width - leftW - rightWidth
		if gap > 0 {
			return left + r(strings.Repeat(" ", gap)) + right
		}
		// Terminal too narrow — just concatenate and pad.
		return pad(left + right)
	}

	// --- Persistent left zone ---
	if s.playbackState == PlaybackIdle && s.queueEmpty {
		hint := r("Queue is empty — press 1 to browse Library")
		return rightAlign(hint) + "\n" + dimLine()
	}
	if s.playbackState == PlaybackIdle && s.artist == "" && s.title == "" {
		return rightAlign("") + "\n" + dimLine()
	}

	stateIcon := ""
	stateIconWidth := 0
	switch s.playbackState {
	case PlaybackPlaying:
		stateIcon = barSuccess.Bold(true).Render("▶")
		stateIconWidth = 1
	case PlaybackPaused:
		stateIcon = barWarning.Bold(true).Render("⏸")
		stateIconWidth = 1
	case PlaybackStopped:
		stateIcon = barError.Bold(true).Render("■")
		stateIconWidth = 1
	}

	// Build the suffix that follows the title: playlist source, mode indicators.
	suffix := ""
	suffixWidth := 0
	if s.playlistSource != "" {
		suffix += barDim.Render("  [" + s.playlistSource + "]")
		suffixWidth += 2 + ansi.StringWidth(s.playlistSource) + 2
	}

	playback, editorial := s.modeLabels(false)
	abbrev, _ := s.modeLabels(true)

	if len(playback) > 0 || len(editorial) > 0 {
		var groups []string
		if len(playback) > 0 {
			groups = append(groups, strings.Join(playback, " "))
		}
		if len(editorial) > 0 {
			groups = append(groups, strings.Join(editorial, " "))
		}
		modesText := strings.Join(groups, " │ ")
		modesDisplayWidth := 2 + ansi.StringWidth(modesText) // 2 = leading "  "

		// Fall back to abbreviated single-letter codes when readable labels would
		// exceed 40% of terminal width, avoiding crowding on narrow terminals.
		if width > 0 && modesDisplayWidth > width*2/5 {
			abbrevText := strings.Join(abbrev, " ")
			var renderedAbbrev []string
			for _, a := range abbrev {
				renderedAbbrev = append(renderedAbbrev, modeStyle(a).Render(a))
			}
			suffix += r("  ") + strings.Join(renderedAbbrev, r(" "))
			suffixWidth += 2 + ansi.StringWidth(abbrevText)
		} else {
			var renderedPlayback []string
			for _, p := range playback {
				renderedPlayback = append(renderedPlayback, modeStyle(p).Render(p))
			}
			var renderedEditorial []string
			for _, e := range editorial {
				renderedEditorial = append(renderedEditorial, barBold.Render(e))
			}
			var renderedGroups []string
			if len(renderedPlayback) > 0 {
				renderedGroups = append(renderedGroups, strings.Join(renderedPlayback, r(" ")))
			}
			if len(renderedEditorial) > 0 {
				renderedGroups = append(renderedGroups, strings.Join(renderedEditorial, r(" ")))
			}
			suffix += r("  ") + strings.Join(renderedGroups, r(" │ "))
			suffixWidth += modesDisplayWidth
		}
	}

	// Build the prefix that precedes the title: glyph, queue position, and artist.
	// Format: "[glyph] (N of M) artist – title" or subsets when fields are absent.
	qp := ""
	qpWidth := 0
	if s.queuePosition != "" {
		qp = barDim.Render(" (" + s.queuePosition + ")")
		qpWidth = ansi.StringWidth(" (" + s.queuePosition + ")")
	}
	prefix := ""
	prefixWidth := 0
	sep := ""
	sepWidth := 0
	switch {
	case stateIcon != "" && s.artist != "":
		prefix = stateIcon + qp + r(" "+s.artist)
		prefixWidth = stateIconWidth + qpWidth + 1 + ansi.StringWidth(s.artist)
		sep = " — "
		sepWidth = 3
	case stateIcon != "":
		prefix = stateIcon + qp
		prefixWidth = stateIconWidth + qpWidth
	case s.artist != "":
		prefix = r(s.artist)
		prefixWidth = ansi.StringWidth(s.artist)
		sep = " — "
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
		// Full line: prefix + sep + title + suffix + gap(min 2) + right
		const minGap = 2
		fixedWidth := prefixWidth + sepWidth +
			suffixWidth +
			minGap + rightWidth
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
	parts = append(parts, suffix)

	left := strings.Join(parts, "")

	var line1 string
	if ansi.StringWidth(left) == 0 {
		line1 = rightAlign("")
	} else {
		line1 = rightAlign(left)
	}

	// --- Line 2: album · time + progress bar ---
	const line2Pad = 1
	var line2 string
	if s.elapsed != "" && s.total != "" {
		albumPart := ""
		albumW := 0
		if s.album != "" {
			albumPart = st.TextSecondary.Background(bg).Render(s.album)
			albumW = ansi.StringWidth(s.album)
		}
		timePart := ""
		timeW := 0
		if s.elapsed != "" && s.total != "" {
			timePart = r(s.elapsed + " / " + s.total)
			timeW = len(s.elapsed) + 3 + len(s.total)
		}
		dotPart := ""
		dotW := 0
		if albumW > 0 && timeW > 0 {
			dotPart = r(" · ")
			dotW = 3
		}
		left2 := r(strings.Repeat(" ", line2Pad)) + albumPart + dotPart + timePart + r(" ")
		leftW2 := line2Pad + albumW + dotW + timeW + 1
		if width > 0 {
			pbWidth := width - leftW2 - line2Pad
			if pbWidth < 0 {
				pbWidth = 0
			}
			line2 = left2 + renderProgressBar(s.elapsedSecs, s.totalSecs, pbWidth, st) + r(strings.Repeat(" ", line2Pad))
		} else {
			line2 = left2
		}
	} else {
		line2 = dimLine()
	}

	return line1 + "\n" + line2
}

// modeLabels returns the active mode label strings in two groups.
// When abbreviated is false the labels are human-readable ("Repeat", "Shuffle",
// "Auto-DJ", …); when true they use compact codes ("R", "S", "DJ", …).
// playback contains repeat/shuffle/stop-next/radio indicators.
// editorial contains visual-mode, tag-filter, yank/cut, and selection indicators.
// In the abbreviated case all labels are in playback and editorial is always nil.
func (s StatusBar) modeLabels(abbreviated bool) (playback, editorial []string) {
	if abbreviated {
		switch s.repeatMode {
		case RepeatAll:
			playback = append(playback, "R")
		case RepeatOne:
			playback = append(playback, "R1")
		}
		if s.shuffle {
			playback = append(playback, "S")
		}
		if s.stopAfterCurrent {
			playback = append(playback, "N")
		}
		if s.radioMode {
			playback = append(playback, "DJ")
		}
		if s.visualMode {
			playback = append(playback, "V")
		}
		if s.activeTagName != "" {
			playback = append(playback, "Tag:"+s.activeTagName)
		}
		if s.yankCount > 0 {
			if s.yankIsCut {
				playback = append(playback, fmt.Sprintf("Cut:%d", s.yankCount))
			} else {
				playback = append(playback, fmt.Sprintf("Y:%d", s.yankCount))
			}
		}
		if s.selectionCount > 0 {
			playback = append(playback, fmt.Sprintf("Sel:%d", s.selectionCount))
		}
		return playback, nil
	}

	switch s.repeatMode {
	case RepeatAll:
		playback = append(playback, "Repeat")
	case RepeatOne:
		playback = append(playback, "Repeat 1")
	}
	if s.shuffle {
		playback = append(playback, "Shuffle")
	}
	if s.stopAfterCurrent {
		playback = append(playback, "Stop Next")
	}
	if s.radioMode {
		playback = append(playback, "Auto-DJ")
	}

	if s.visualMode {
		editorial = append(editorial, "Visual")
	}
	if s.activeTagName != "" {
		editorial = append(editorial, "Tag: "+s.activeTagName)
	}
	if s.yankCount > 0 {
		if s.yankIsCut {
			editorial = append(editorial, fmt.Sprintf("Cut: %d", s.yankCount))
		} else {
			editorial = append(editorial, fmt.Sprintf("Yank: %d", s.yankCount))
		}
	}
	if s.selectionCount > 0 {
		editorial = append(editorial, fmt.Sprintf("Sel: %d", s.selectionCount))
	}
	return playback, editorial
}
