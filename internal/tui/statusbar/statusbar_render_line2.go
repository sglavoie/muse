package statusbar

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (s StatusBar) renderLine2(ctx statusBarRenderContext) string {
	playbackActive := s.Elapsed != "" && s.Total != ""
	longOpActive := s.LongOp.Active
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
	if s.Album != "" {
		albumPart = ctx.styles.TextSecondary.Background(ctx.base.GetBackground()).Render(s.Album)
		albumWidth = ansi.StringWidth(s.Album)
	}
	timePart := ""
	timeWidth := 0
	if s.Elapsed != "" && s.Total != "" {
		timePart = ctx.r(s.Elapsed + " / " + s.Total)
		timeWidth = len(s.Elapsed) + 3 + len(s.Total)
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
	return left + renderProgressBar(s.ElapsedSecs, s.TotalSecs, progressWidth, ctx.styles) + ctx.r(strings.Repeat(" ", line2Pad))
}

func (s StatusBar) renderLongOpLine(ctx statusBarRenderContext, targetWidth int) string {
	const line2Pad = 1
	label := s.LongOp.Label
	if label == "" {
		label = "Working"
	}
	done := s.LongOp.done
	total := s.LongOp.total
	counts := text.FormatCount(done)
	if total > 0 {
		counts = fmt.Sprintf("%s/%s", text.FormatCount(done), text.FormatCount(total))
	}
	etaText := ""
	if eta := s.LongOp.etaSeconds(); eta > 0 {
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
