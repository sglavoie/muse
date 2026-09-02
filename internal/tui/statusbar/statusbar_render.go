package statusbar

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

type statusBarRenderContext struct {
	width  int
	styles styles.Styles
	base   lipgloss.Style

	barText          lipgloss.Style
	barSuccess       lipgloss.Style
	barWarning       lipgloss.Style
	barError         lipgloss.Style
	barBold          lipgloss.Style
	barDim           lipgloss.Style
	barModeRepeat    lipgloss.Style
	barModeShuffle   lipgloss.Style
	barModeStopAfter lipgloss.Style
	barModeRadio     lipgloss.Style
}

func newStatusBarRenderContext(width int, st styles.Styles, playbackState playback.State) statusBarRenderContext {
	base := st.StatusBar
	switch playbackState {
	case playback.StatePlaying:
		base = st.StatusBarPlayingBg
	case playback.StateStopped:
		base = st.StatusBarStoppedBg
	}
	bg := base.GetBackground()
	fg := base.GetForeground()
	barText := lipgloss.NewStyle().Background(bg).Foreground(fg)
	return statusBarRenderContext{
		width:            width,
		styles:           st,
		base:             base,
		barText:          barText,
		barSuccess:       st.Success.Background(bg),
		barWarning:       st.Warning.Background(bg),
		barError:         st.Error.Background(bg),
		barBold:          barText.Bold(true),
		barDim:           barText.Faint(true),
		barModeRepeat:    st.ModeRepeat.Background(bg),
		barModeShuffle:   st.ModeShuffle.Background(bg),
		barModeStopAfter: st.ModeStopAfter.Background(bg),
		barModeRadio:     st.ModeRadio.Background(bg),
	}
}

func (c statusBarRenderContext) r(s string) string { return c.barText.Render(s) }

func (c statusBarRenderContext) pad(content string) string {
	if c.width > 0 {
		visibleWidth := ansi.StringWidth(content)
		if visibleWidth > c.width {
			content = ansi.Truncate(content, c.width, "…")
			visibleWidth = ansi.StringWidth(content)
		}
		if visibleWidth < c.width {
			content += c.r(strings.Repeat(" ", c.width-visibleWidth))
		}
	}
	return content
}

func (c statusBarRenderContext) rightAlign(left, right string) string {
	rightWidth := ansi.StringWidth(right)
	if rightWidth == 0 {
		return c.pad(left)
	}
	if c.width == 0 {
		if left == "" {
			return right
		}
		return left + c.r("  ") + right
	}
	leftWidth := ansi.StringWidth(left)
	gap := c.width - leftWidth - rightWidth
	if gap > 0 {
		return left + c.r(strings.Repeat(" ", gap)) + right
	}
	if rightWidth >= c.width {
		return c.pad(right)
	}
	leftBudget := c.width - rightWidth
	left = ansi.Truncate(left, leftBudget, "…")
	leftWidth = ansi.StringWidth(left)
	gap = c.width - leftWidth - rightWidth
	if gap > 0 {
		return left + c.r(strings.Repeat(" ", gap)) + right
	}
	return c.pad(left + right)
}

func (c statusBarRenderContext) dimLine() string {
	if c.width > 0 {
		return c.styles.Dim.Render(strings.Repeat("─", c.width))
	}
	return c.styles.Dim.Render("─")
}

// Render builds the full status bar string.
func (s StatusBar) Render(width int, st styles.Styles) string {
	ctx := newStatusBarRenderContext(width, st, s.PlaybackState)
	right := s.renderRightZone(ctx)
	if s.PlaybackState == playback.StateIdle && s.QueueEmpty {
		return ctx.rightAlign(ctx.r("Queue is empty — press 1 to browse Library"), right) + "\n" + ctx.dimLine()
	}
	if s.PlaybackState == playback.StateIdle && s.Artist == "" && s.Title == "" {
		return ctx.rightAlign("", right) + "\n" + ctx.dimLine()
	}
	return s.renderLine1(ctx, right) + "\n" + s.renderLine2(ctx)
}
