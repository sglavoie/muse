package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (s StatusBar) renderRightZone(ctx statusBarRenderContext) string {
	notif := ctx.styles.StatusNotification
	notifErr := ctx.styles.StatusNotificationError
	var segments []string
	if s.configCreated {
		segments = append(segments, notif.Render(fmt.Sprintf(" config created: %s ", s.configPath)))
	}
	if s.watchDegraded {
		segments = append(segments, notifErr.Render(" File watcher unavailable — press r to rescan manually "))
	}
	if top := s.topNotification(); top.msg != "" {
		msg := top.msg
		if !s.undoExpiresAt.IsZero() {
			if remaining := time.Until(s.undoExpiresAt); remaining > 0 {
				msg = fmt.Sprintf("%s (%ds)", msg, int(math.Ceil(remaining.Seconds())))
			}
		}
		style := notif
		if strings.HasPrefix(msg, "Error:") {
			style = notifErr
		}
		if top.isFading() {
			style = style.Faint(true)
		}
		segments = append(segments, style.Render(" "+msg+" "))
	}
	return strings.Join(segments, ctx.r(" "))
}

func (s StatusBar) renderLine1(ctx statusBarRenderContext, right string) string {
	stateIcon, stateIconWidth := s.playbackStateIcon(ctx)
	cluster, clusterWidth := s.renderModeCluster(ctx)
	suffix, suffixWidth := s.renderSuffix(ctx, cluster, clusterWidth)
	prefix, prefixWidth, sep, sepWidth := s.renderPrefix(ctx, stateIcon, stateIconWidth)
	title := s.title
	if ctx.width > 0 && s.title != "" {
		const minGap = 2
		fixedWidth := prefixWidth + sepWidth + suffixWidth + minGap + ansi.StringWidth(right)
		title = truncateTitle(s.title, ctx.width-fixedWidth)
	}

	var parts []string
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if sep != "" {
		parts = append(parts, ctx.r(sep))
	}
	if title != "" {
		parts = append(parts, ctx.r(title))
	}
	parts = append(parts, suffix)
	return ctx.rightAlign(strings.Join(parts, ""), right)
}

func (s StatusBar) playbackStateIcon(ctx statusBarRenderContext) (string, int) {
	switch s.playbackState {
	case PlaybackPlaying:
		return ctx.barSuccess.Bold(true).Render("▶"), 1
	case PlaybackPaused:
		return ctx.barWarning.Bold(true).Render("⏸"), 1
	case PlaybackStopped:
		return ctx.barError.Bold(true).Render("■"), 1
	default:
		return "", 0
	}
}

// modeIndicator describes one glyph of the status-bar mode cluster. It is the
// single source of truth for what a letter means, so the cluster, the legend
// overlay and the toggle notifications can never disagree.
type modeIndicator struct {
	glyph  string // "R"/"1", "S", "N", "D"
	name   string // "Repeat", "Shuffle", "Stop after current", "Radio (Auto-DJ)"
	state  string // "all"/"one"/"off" for repeat, "on"/"off" for the rest
	key    string // the global keybinding that toggles this mode
	active bool
}

// modeState renders a boolean mode state to match RepeatMode.DisplayString's
// lowercase style.
func modeState(active bool) string {
	if active {
		return "on"
	}
	return "off"
}

// modeIndicators returns the mode cluster in render order. The order is
// contractual: renderModeCluster pairs it positionally with the per-mode
// active styles of statusBarRenderContext.
func modeIndicators(repeat RepeatMode, shuffle, stopAfter, radio bool) []modeIndicator {
	repeatGlyph := "R"
	if repeat == RepeatOne {
		repeatGlyph = "1"
	}
	return []modeIndicator{
		{glyph: repeatGlyph, name: "Repeat", state: repeat.DisplayString(), key: "r", active: repeat != RepeatOff},
		{glyph: "S", name: "Shuffle", state: modeState(shuffle), key: "z", active: shuffle},
		{glyph: "N", name: "Stop after current", state: modeState(stopAfter), key: "ctrl+n", active: stopAfter},
		{glyph: "D", name: "Radio (Auto-DJ)", state: modeState(radio), key: "A", active: radio},
	}
}

func (s StatusBar) renderModeCluster(ctx statusBarRenderContext) (string, int) {
	clusterDim := ctx.styles.TextTertiary.Background(ctx.base.GetBackground())
	activeStyles := []lipgloss.Style{ctx.barModeRepeat, ctx.barModeShuffle, ctx.barModeStopAfter, ctx.barModeRadio}
	indicators := modeIndicators(s.repeatMode, s.shuffle, s.stopAfterCurrent, s.radioMode)
	var cluster strings.Builder
	for i, ind := range indicators {
		style := clusterDim
		if ind.active && i < len(activeStyles) {
			style = activeStyles[i]
		}
		cluster.WriteString(style.Render(ind.glyph))
	}
	return cluster.String(), len(indicators)
}

func (s StatusBar) renderSuffix(ctx statusBarRenderContext, cluster string, clusterWidth int) (string, int) {
	suffix := ""
	suffixWidth := 0
	if s.playlistSource != "" {
		suffix += ctx.barDim.Render("  [" + s.playlistSource + "]")
		suffixWidth += 2 + ansi.StringWidth(s.playlistSource) + 2
	}
	suffix += ctx.r("  ") + cluster
	suffixWidth += 2 + clusterWidth

	fullLabels := s.modeLabels(false)
	abbrevLabels := s.modeLabels(true)
	if len(fullLabels) == 0 {
		return suffix, suffixWidth
	}
	const sepText = " │ "
	const sepWidth = 3
	fullText := strings.Join(fullLabels, " ")
	fullDisplayWidth := sepWidth + ansi.StringWidth(fullText)
	if ctx.width > 0 && suffixWidth+fullDisplayWidth > ctx.width*2/5 {
		abbrevText := strings.Join(abbrevLabels, " ")
		var rendered []string
		for _, label := range abbrevLabels {
			rendered = append(rendered, ctx.barBold.Render(label))
		}
		suffix += ctx.r(sepText) + strings.Join(rendered, ctx.r(" "))
		suffixWidth += sepWidth + ansi.StringWidth(abbrevText)
		return suffix, suffixWidth
	}
	var rendered []string
	for _, label := range fullLabels {
		rendered = append(rendered, ctx.barBold.Render(label))
	}
	suffix += ctx.r(sepText) + strings.Join(rendered, ctx.r(" "))
	suffixWidth += fullDisplayWidth
	return suffix, suffixWidth
}

func (s StatusBar) renderPrefix(ctx statusBarRenderContext, stateIcon string, stateIconWidth int) (string, int, string, int) {
	qp := ""
	qpWidth := 0
	if s.queuePosition != "" {
		posText := s.queuePosition
		if s.bucketLabel != "" {
			posText += " · " + s.bucketLabel
		}
		qp = ctx.barDim.Render(" (" + posText + ")")
		qpWidth = ansi.StringWidth(" (" + posText + ")")
	}
	prefix := ""
	prefixWidth := 0
	sep := ""
	sepWidth := 0
	switch {
	case stateIcon != "" && s.artist != "":
		prefix = stateIcon + qp + ctx.r(" "+s.artist)
		prefixWidth = stateIconWidth + qpWidth + 1 + ansi.StringWidth(s.artist)
		sep = " — "
		sepWidth = 3
	case stateIcon != "":
		prefix = stateIcon + qp
		prefixWidth = stateIconWidth + qpWidth
	case s.artist != "":
		prefix = ctx.r(s.artist)
		prefixWidth = ansi.StringWidth(s.artist)
		sep = " — "
		sepWidth = 3
	}
	if s.title == "" {
		sep = ""
		sepWidth = 0
	}
	return prefix, prefixWidth, sep, sepWidth
}
