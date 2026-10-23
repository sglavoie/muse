package statusbar

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (s StatusBar) renderRightZone(ctx statusBarRenderContext) string {
	notif := ctx.styles.StatusNotification
	notifErr := ctx.styles.StatusNotificationError
	var segments []string
	if s.hasUnreadErrors {
		segments = append(segments, notifErr.Render("! errors · :"))
	}
	if top := s.TopNotification(); top.Msg != "" {
		style := notificationStyle(top.Severity, top.isFading(), notif, notifErr)
		segments = append(segments, style.Render(top.Msg))
	} else if s.WatchDegraded {
		segments = append(segments, notifErr.Render("Watcher offline · ctrl+r"))
	} else if s.ConfigCreated {
		segments = append(segments, notif.Render(fmt.Sprintf("Config created: %s", s.ConfigPath)))
	}
	// Full messages remain in Recent Messages. Bound this zone so a long
	// filesystem error cannot displace the song and its playback modes.
	if ctx.width > 0 && (s.Title != "" || s.Artist != "" || (s.QueueEmpty && s.EmptyQueueHint != "")) {
		return ansi.Truncate(strings.Join(segments, ctx.r(" · ")), max(12, ctx.width/3), "…")
	}

	return strings.Join(segments, ctx.r(" "))
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
	suffixCtx := ctx
	if ctx.width > 0 {
		suffixCtx.width = max(1, ctx.width-ansi.StringWidth(right)-2)
	}
	suffix, suffixWidth := s.renderSuffix(suffixCtx, cluster, clusterWidth)
	s = s.fitIdentity(ctx, stateIcon, stateIconWidth, suffixCtx.width-suffixWidth)

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
		parts = append(parts, ctx.barBold.Render(title))
	}
	parts = append(parts, suffix)
	return ctx.rightAlign(strings.Join(parts, ""), right)
}

// Song identity takes priority over queue position when modes and notices
// compete for space. The playback clock remains on its separate line.
func (s StatusBar) fitIdentity(ctx statusBarRenderContext, icon string, iconWidth, budget int) StatusBar {
	if ctx.width <= 0 {
		return s
	}
	positionWidth := ansi.StringWidth(s.QueuePosition)
	if s.BucketLabel != "" {
		positionWidth += 3 + ansi.StringWidth(s.BucketLabel)
	}
	if s.QueuePosition != "" && budget < 1+positionWidth+4+min(12, ansi.StringWidth(s.Title)) {
		s.QueuePosition, s.BucketLabel = "", ""
	}
	return s.fitArtist(ctx, icon, iconWidth, budget)
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

// Only enabled modes are shown; the all-off state is named explicitly.
// Text changes with state even in terminals with no color support.
func (s StatusBar) renderModeCluster(ctx statusBarRenderContext) (string, int) {
	var labels, compact []string
	indicators := playback.ModeIndicators(s.RepeatMode, s.Shuffle, s.StopAfterCurrent, s.RadioMode)
	for _, indicator := range indicators {
		if indicator.Active {
			labels = append(labels, indicator.Label())
			compact = append(compact, indicator.Compact)
		}
	}
	label := strings.Join(labels, " · ")
	if label == "" {
		label = "Sequential"
	}
	if ctx.width > 0 && ansi.StringWidth(label) > ctx.width/3 {
		label = strings.Join(compact, " ")
	}
	return ctx.barDim.Render(label), ansi.StringWidth(label)
}

func (s StatusBar) renderSuffix(ctx statusBarRenderContext, cluster string, clusterWidth int) (string, int) {
	suffix := ctx.r("  ") + cluster
	width := 2 + clusterWidth
	budget := ctx.width * 2 / 5
	if ctx.width <= 0 {
		budget = 10000
	}
	// Modes always survive. Other hints yield as whole labels, not half words.
	for _, label := range s.modeLabels(true) {
		if width+3+ansi.StringWidth(label) <= budget {
			suffix += ctx.r(" · ") + ctx.barDim.Render(label)
			width += 3 + ansi.StringWidth(label)
		}
	}
	if s.PlaylistSource != "" && budget-width >= 8 {
		source := text.Truncate(s.PlaylistSource, budget-width-4)
		suffix += ctx.barDim.Render("  [" + source + "]")
		width += 4 + ansi.StringWidth(source)
	}
	return suffix, width
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
		sep, sepWidth = " ", 1
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

// Keep a recognizable title even when artist metadata is unusually long.
func (s StatusBar) fitArtist(ctx statusBarRenderContext, icon string, iconWidth, budget int) StatusBar {
	_, prefixWidth, _, sepWidth := s.renderPrefix(ctx, icon, iconWidth)
	artistWidth := ansi.StringWidth(s.Artist)
	titleWidth := ansi.StringWidth(s.Title)
	if prefixWidth+sepWidth+titleWidth <= budget {
		return s
	}
	fixed := prefixWidth - artistWidth + sepWidth
	artistBudget := max(0, budget-fixed-min(titleWidth, max(12, (budget-fixed)/2)))
	s.Artist = text.Truncate(s.Artist, artistBudget)
	return s
}
