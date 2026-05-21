package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

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

	// radioMode is true when Auto-DJ / radio mode is active.
	// Injected by renderStatusBar from the root Model.
	radioMode bool

	// queuePosition is the formatted "N of M" queue position string injected
	// by renderStatusBar. Empty when nothing is playing.
	queuePosition string

	// bucketLabel is an optional chunk label for the active view's focused
	// row (e.g. "A", "Today"). When non-empty alongside queuePosition the
	// line-1 position display reads "N of M · LABEL". Dropped in narrow
	// terminals if room is tight.
	bucketLabel string

	// yankCount, yankIsCut, and selectionCount are injected by renderStatusBar.
	yankCount      int
	yankIsCut      bool
	selectionCount int

	// longOp is the current long-running operation snapshot injected by
	// renderStatusBar. When longOp.Active is true the status bar takes over
	// line 2 (or shares it with the playback bar) to show progress + ETA.
	longOp longOpState
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
		isError := strings.HasPrefix(msg, "Error:")
		// Dim the final fade frame (last notificationFadeWindow of lifetime) so
		// the notification softens before vanishing rather than snapping out.
		notifStyle := notif
		if isError {
			notifStyle = notifErr
		}
		if top.isFading() {
			notifStyle = notifStyle.Faint(true)
		}
		rightSegments = append(rightSegments, notifStyle.Render(" "+msg+" "))
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

	// Build the always-on 4-glyph RSND cluster. Active letters use their mode
	// color; inactive letters are dimmed via TextTertiary. The cluster is
	// always 4 columns wide so its position stays stable across mode toggles.
	clusterDim := st.TextTertiary.Background(bg)
	clusterGlyph := func(active bool, label string, activeStyle lipgloss.Style) string {
		if active {
			return activeStyle.Render(label)
		}
		return clusterDim.Render(label)
	}
	rLabel := "R"
	if s.repeatMode == RepeatOne {
		rLabel = "1"
	}
	cluster := clusterGlyph(s.repeatMode != RepeatOff, rLabel, barModeRepeat) +
		clusterGlyph(s.shuffle, "S", barModeShuffle) +
		clusterGlyph(s.stopAfterCurrent, "N", barModeStopAfter) +
		clusterGlyph(s.radioMode, "D", barModeRadio)
	const clusterWidth = 4

	// Build the suffix that follows the title: playlist source, mode cluster,
	// editorial chips (Visual, Yank/Cut, Sel).
	suffix := ""
	suffixWidth := 0
	if s.playlistSource != "" {
		suffix += barDim.Render("  [" + s.playlistSource + "]")
		suffixWidth += 2 + ansi.StringWidth(s.playlistSource) + 2
	}
	suffix += r("  ") + cluster
	suffixWidth += 2 + clusterWidth

	editorialFull := s.modeLabels(false)
	editorialAbbrev := s.modeLabels(true)
	if len(editorialFull) > 0 {
		const sepText = " │ "
		const sepWidth = 3
		fullText := strings.Join(editorialFull, " ")
		fullDisplayWidth := sepWidth + ansi.StringWidth(fullText)
		// Fall back to abbreviated forms when verbose labels would crowd the bar.
		if width > 0 && (suffixWidth+fullDisplayWidth) > width*2/5 {
			abbrevText := strings.Join(editorialAbbrev, " ")
			var renderedAbbrev []string
			for _, a := range editorialAbbrev {
				renderedAbbrev = append(renderedAbbrev, barBold.Render(a))
			}
			suffix += r(sepText) + strings.Join(renderedAbbrev, r(" "))
			suffixWidth += sepWidth + ansi.StringWidth(abbrevText)
		} else {
			var renderedFull []string
			for _, e := range editorialFull {
				renderedFull = append(renderedFull, barBold.Render(e))
			}
			suffix += r(sepText) + strings.Join(renderedFull, r(" "))
			suffixWidth += fullDisplayWidth
		}
	}

	// Build the prefix that precedes the title: glyph, queue position, and artist.
	// Format: "[glyph] (N of M) artist – title" or subsets when fields are absent.
	qp := ""
	qpWidth := 0
	if s.queuePosition != "" {
		posText := s.queuePosition
		if s.bucketLabel != "" {
			posText += " · " + s.bucketLabel
		}
		qp = barDim.Render(" (" + posText + ")")
		qpWidth = ansi.StringWidth(" (" + posText + ")")
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

	// --- Line 2: album · time + progress bar; or long-op widget; or both ---
	const line2Pad = 1
	playbackActive := s.elapsed != "" && s.total != ""
	longOpActive := s.longOp.Active

	renderPlayback := func(targetWidth int) string {
		albumPart := ""
		albumW := 0
		if s.album != "" {
			albumPart = st.TextSecondary.Background(bg).Render(s.album)
			albumW = ansi.StringWidth(s.album)
		}
		timePart := ""
		timeW := 0
		if playbackActive {
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
		if targetWidth > 0 {
			pbWidth := targetWidth - leftW2 - line2Pad
			if pbWidth < 0 {
				pbWidth = 0
			}
			return left2 + renderProgressBar(s.elapsedSecs, s.totalSecs, pbWidth, st) + r(strings.Repeat(" ", line2Pad))
		}
		return left2
	}

	renderLongOp := func(targetWidth int) string {
		label := s.longOp.Label
		if label == "" {
			label = "Working"
		}
		done := s.longOp.Done
		total := s.longOp.Total
		counts := ""
		if total > 0 {
			counts = fmt.Sprintf("%s/%s", formatCount(done), formatCount(total))
		} else {
			counts = formatCount(done)
		}
		etaStr := ""
		if eta := s.longOp.etaSeconds(); eta > 0 {
			etaStr = "ETA " + formatTime(float64(eta))
		}
		cancelHint := "esc cancel"

		prefix := r(strings.Repeat(" ", line2Pad)) + barBold.Render(label) + r(" ")
		prefixW := line2Pad + ansi.StringWidth(label) + 1

		countsPart := r(" " + counts)
		countsW := 1 + ansi.StringWidth(counts)
		etaPart := ""
		etaW := 0
		if etaStr != "" {
			etaPart = r("  ") + barDim.Render(etaStr)
			etaW = 2 + ansi.StringWidth(etaStr)
		}
		cancelPart := r("  · ") + barDim.Render(cancelHint)
		cancelW := 4 + ansi.StringWidth(cancelHint)

		trailing := countsPart + etaPart + cancelPart
		trailingW := countsW + etaW + cancelW

		if targetWidth <= 0 {
			return prefix + renderProgressBar(float64(done), float64(max(total, 1)), 20, st) + trailing
		}
		pbWidth := targetWidth - prefixW - trailingW - line2Pad
		if pbWidth < 4 {
			// Not enough room for an ETA — drop it and try again.
			trailing = countsPart + cancelPart
			trailingW = countsW + cancelW
			pbWidth = targetWidth - prefixW - trailingW - line2Pad
		}
		if pbWidth < 4 {
			// Still not enough — drop the cancel hint.
			trailing = countsPart
			trailingW = countsW
			pbWidth = targetWidth - prefixW - trailingW - line2Pad
		}
		if pbWidth < 0 {
			pbWidth = 0
		}
		bar := renderProgressBar(float64(done), float64(max(total, 1)), pbWidth, st)
		return prefix + bar + trailing + r(strings.Repeat(" ", line2Pad))
	}

	var line2 string
	switch {
	case longOpActive && playbackActive && width > 0:
		half := width / 2
		left2 := renderPlayback(half)
		right2 := renderLongOp(width - half)
		// Pad the playback half if it came up short.
		if w := ansi.StringWidth(left2); w < half {
			left2 += r(strings.Repeat(" ", half-w))
		}
		line2 = left2 + right2
	case longOpActive:
		line2 = renderLongOp(width)
	case playbackActive:
		line2 = renderPlayback(width)
	default:
		line2 = dimLine()
	}

	return line1 + "\n" + line2
}
