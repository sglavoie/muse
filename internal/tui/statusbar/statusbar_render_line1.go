package statusbar

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (s StatusBar) renderRightZone(ctx statusBarRenderContext) string {
	notif := ctx.styles.StatusNotification
	notifErr := ctx.styles.StatusNotificationError
	var segments []string
	if s.ConfigCreated {
		segments = append(segments, notif.Render(fmt.Sprintf(" config created: %s ", s.ConfigPath)))
	}
	if s.WatchDegraded {
		segments = append(segments, notifErr.Render(" File watcher unavailable — press ctrl+r to rescan manually "))
	}
	if top := s.TopNotification(); top.Msg != "" {
		msg := s.withUndoCountdown(top)
		style := notificationStyle(top.Severity, top.isFading(), notif, notifErr)
		segments = append(segments, style.Render(" "+msg+" "))
	}
	return strings.Join(segments, ctx.r(" "))
}

// withUndoCountdown appends the seconds left on an open undo window to msg.
func (s StatusBar) withUndoCountdown(n TransientNotification) string {
	if n.undoExpiresAt.IsZero() || !n.undoExpiresAt.Equal(s.UndoExpiresAt) {
		return n.Msg
	}
	remaining := time.Until(s.UndoExpiresAt)
	if remaining <= 0 {
		return n.Msg
	}
	return fmt.Sprintf("%s (%ds)", n.Msg, int(math.Ceil(remaining.Seconds())))
}

// notificationStyle picks the error or ordinary notification style, faded
// while the notification is on its way out.
func notificationStyle(severity Severity, fading bool, notif, notifErr lipgloss.Style) lipgloss.Style {
	style := notif
	if severity == SeverityError {
		style = notifErr
	}
	if fading {
		style = style.Faint(true)
	}
	return style
}

func (s StatusBar) renderLine1(ctx statusBarRenderContext, right string) string {
	stateIcon, stateIconWidth := s.playbackStateIcon(ctx)
	cluster, clusterWidth := s.renderModeCluster(ctx)
	suffix, suffixWidth := s.renderSuffix(ctx, cluster, clusterWidth)
	prefix, prefixWidth, sep, sepWidth := s.renderPrefix(ctx, stateIcon, stateIconWidth)
	title := s.Title
	if ctx.width > 0 && s.Title != "" {
		const minGap = 2
		fixedWidth := prefixWidth + sepWidth + suffixWidth + minGap + ansi.StringWidth(right)
		title = text.Truncate(s.Title, ctx.width-fixedWidth)
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
	switch s.PlaybackState {
	case playback.StatePlaying:
		return ctx.barSuccess.Bold(true).Render("▶"), 1
	case playback.StatePaused:
		return ctx.barWarning.Bold(true).Render("⏸"), 1
	case playback.StateStopped:
		return ctx.barError.Bold(true).Render("■"), 1
	default:
		return "", 0
	}
}

func (s StatusBar) renderModeCluster(ctx statusBarRenderContext) (string, int) {
	clusterDim := ctx.styles.TextTertiary.Background(ctx.base.GetBackground())
	activeStyles := []lipgloss.Style{ctx.barModeRepeat, ctx.barModeShuffle, ctx.barModeStopAfter, ctx.barModeRadio}
	indicators := playback.ModeIndicators(s.RepeatMode, s.Shuffle, s.StopAfterCurrent, s.RadioMode)
	var cluster strings.Builder
	for i, ind := range indicators {
		style := clusterDim
		if ind.Active && i < len(activeStyles) {
			style = activeStyles[i]
		}
		cluster.WriteString(style.Render(ind.Glyph))
	}
	return cluster.String(), len(indicators)
}

func (s StatusBar) renderSuffix(ctx statusBarRenderContext, cluster string, clusterWidth int) (string, int) {
	suffix := ""
	suffixWidth := 0
	if s.PlaylistSource != "" {
		suffix += ctx.barDim.Render("  [" + s.PlaylistSource + "]")
		suffixWidth += 2 + ansi.StringWidth(s.PlaylistSource) + 2
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
	if s.QueuePosition != "" {
		posText := s.QueuePosition
		if s.BucketLabel != "" {
			posText += " · " + s.BucketLabel
		}
		qp = ctx.barDim.Render(" (" + posText + ")")
		qpWidth = ansi.StringWidth(" (" + posText + ")")
	}
	prefix := ""
	prefixWidth := 0
	sep := ""
	sepWidth := 0
	switch {
	case stateIcon != "" && s.Artist != "":
		prefix = stateIcon + qp + ctx.r(" "+s.Artist)
		prefixWidth = stateIconWidth + qpWidth + 1 + ansi.StringWidth(s.Artist)
		sep = " — "
		sepWidth = 3
	case stateIcon != "":
		prefix = stateIcon + qp
		prefixWidth = stateIconWidth + qpWidth
	case s.Artist != "":
		prefix = ctx.r(s.Artist)
		prefixWidth = ansi.StringWidth(s.Artist)
		sep = " — "
		sepWidth = 3
	}
	if s.Title == "" {
		sep = ""
		sepWidth = 0
	}
	return prefix, prefixWidth, sep, sepWidth
}
