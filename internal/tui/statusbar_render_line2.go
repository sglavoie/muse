package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (s StatusBar) renderLine2(ctx statusBarRenderContext) string {
	playbackActive := s.elapsed != "" && s.total != ""
	longOpActive := s.longOp.Active
	switch {
	case longOpActive && playbackActive && ctx.width > 0:
		half := ctx.width / 2
		left := s.renderPlaybackLine(ctx, half)
		right := s.renderLongOpLine(ctx, ctx.width-half)
		if width := ansi.StringWidth(left); width < half {
			left += ctx.r(strings.Repeat(" ", half-width))
		}
		return left + right
	case longOpActive:
		return s.renderLongOpLine(ctx, ctx.width)
	case playbackActive:
		return s.renderPlaybackLine(ctx, ctx.width)
	default:
		return ctx.dimLine()
	}
}

func (s StatusBar) renderPlaybackLine(ctx statusBarRenderContext, targetWidth int) string {
	const line2Pad = 1
	albumPart := ""
	albumWidth := 0
	if s.album != "" {
		albumPart = ctx.styles.TextSecondary.Background(ctx.base.GetBackground()).Render(s.album)
		albumWidth = ansi.StringWidth(s.album)
	}
	timePart := ""
	timeWidth := 0
	if s.elapsed != "" && s.total != "" {
		timePart = ctx.r(s.elapsed + " / " + s.total)
		timeWidth = len(s.elapsed) + 3 + len(s.total)
	}
	dotPart := ""
	dotWidth := 0
	if albumWidth > 0 && timeWidth > 0 {
		dotPart = ctx.r(" · ")
		dotWidth = 3
	}
	left := ctx.r(strings.Repeat(" ", line2Pad)) + albumPart + dotPart + timePart + ctx.r(" ")
	leftWidth := line2Pad + albumWidth + dotWidth + timeWidth + 1
	if targetWidth <= 0 {
		return left
	}
	progressWidth := targetWidth - leftWidth - line2Pad
	if progressWidth < 0 {
		progressWidth = 0
	}
	return left + renderProgressBar(s.elapsedSecs, s.totalSecs, progressWidth, ctx.styles) + ctx.r(strings.Repeat(" ", line2Pad))
}

func (s StatusBar) renderLongOpLine(ctx statusBarRenderContext, targetWidth int) string {
	const line2Pad = 1
	label := s.longOp.Label
	if label == "" {
		label = "Working"
	}
	done := s.longOp.Done
	total := s.longOp.Total
	counts := formatCount(done)
	if total > 0 {
		counts = fmt.Sprintf("%s/%s", formatCount(done), formatCount(total))
	}
	etaText := ""
	if eta := s.longOp.etaSeconds(); eta > 0 {
		etaText = "ETA " + text.FormatPositionSecs(float64(eta))
	}
	cancelHint := "esc cancel"

	prefix := ctx.r(strings.Repeat(" ", line2Pad)) + ctx.barBold.Render(label) + ctx.r(" ")
	prefixWidth := line2Pad + ansi.StringWidth(label) + 1
	countsPart := ctx.r(" " + counts)
	countsWidth := 1 + ansi.StringWidth(counts)
	etaPart := ""
	etaWidth := 0
	if etaText != "" {
		etaPart = ctx.r("  ") + ctx.barDim.Render(etaText)
		etaWidth = 2 + ansi.StringWidth(etaText)
	}
	cancelPart := ctx.r("  · ") + ctx.barDim.Render(cancelHint)
	cancelWidth := 4 + ansi.StringWidth(cancelHint)
	trailing := countsPart + etaPart + cancelPart
	trailingWidth := countsWidth + etaWidth + cancelWidth

	if targetWidth <= 0 {
		return prefix + renderProgressBar(float64(done), float64(max(total, 1)), 20, ctx.styles) + trailing
	}
	progressWidth := targetWidth - prefixWidth - trailingWidth - line2Pad
	if progressWidth < 4 {
		trailing = countsPart + cancelPart
		trailingWidth = countsWidth + cancelWidth
		progressWidth = targetWidth - prefixWidth - trailingWidth - line2Pad
	}
	if progressWidth < 4 {
		trailing = countsPart
		progressWidth = targetWidth - prefixWidth - countsWidth - line2Pad
	}
	if progressWidth < 0 {
		progressWidth = 0
	}
	bar := renderProgressBar(float64(done), float64(max(total, 1)), progressWidth, ctx.styles)
	return prefix + bar + trailing + ctx.r(strings.Repeat(" ", line2Pad))
}
