package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// View renders the active panel and a status bar, returning a tea.View.
func (m Model) View() tea.View {
	if m.terminalTooSmall() {
		v := tea.NewView(m.renderTerminalTooSmall())
		v.AltScreen = true
		return v
	}

	body := m.renderActiveBody()
	if !m.hasActiveOverlay() {
		tabSep := m.styles.TextTertiary.Render(strings.Repeat("─", m.termWidth))
		body = m.renderTitleBar() + "\n" + tabSep + "\n" + body
	}

	statusBar := m.renderStatusBar()
	footerHint := m.renderFooterHint()
	bodyLines := strings.Count(body, "\n") + 1
	confirmExtra := 0
	if m.confirmationBar != "" {
		confirmExtra = 1
	}
	hintExtra := 0
	if footerHint != "" {
		hintExtra = 1
	}
	if padding := m.termHeight - bodyLines - 2 - confirmExtra - hintExtra; padding > 0 {
		body += strings.Repeat("\n", padding)
	}

	content := body + "\n" + statusBar
	switch {
	case m.confirmationBar != "" && footerHint != "":
		confirmLine := m.styles.Warning.Width(m.termWidth).Render("  " + m.confirmationBar)
		content = body + "\n" + confirmLine + "\n" + footerHint + "\n" + statusBar
	case m.confirmationBar != "":
		confirmLine := m.styles.Warning.Width(m.termWidth).Render("  " + m.confirmationBar)
		content = body + "\n" + confirmLine + "\n" + statusBar
	case footerHint != "":
		content = body + "\n" + footerHint + "\n" + statusBar
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) terminalTooSmall() bool {
	if m.termWidth <= 0 || m.termHeight <= 0 {
		return false
	}
	return m.termWidth < minTerminalWidth || m.termHeight < minTerminalHeight
}

func (m Model) renderTerminalTooSmall() string {
	width, height := m.termWidth, m.termHeight
	if width <= 0 || height <= 0 {
		return ""
	}

	lines := []string{
		terminalTooSmallTitle(width),
		fmt.Sprintf("Minimum size: %dx%d", minTerminalWidth, minTerminalHeight),
		fmt.Sprintf("Current size: %dx%d", width, height),
	}
	if height < len(lines) {
		lines = lines[:height]
	}

	topPad := max(0, (height-len(lines))/2)
	out := make([]string, 0, topPad+len(lines))
	for range topPad {
		out = append(out, strings.Repeat(" ", width))
	}
	for _, line := range lines {
		out = append(out, centerTerminalMessageLine(line, width))
	}
	return strings.Join(out, "\n")
}

func terminalTooSmallTitle(width int) string {
	for _, candidate := range []string{
		"Terminal window is too small",
		"Terminal too small",
		"Too small",
	} {
		if ansi.StringWidth(candidate) <= width {
			return candidate
		}
	}
	return "Too small"
}

func centerTerminalMessageLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = text.Truncate(line, width)
	lineWidth := ansi.StringWidth(line)
	leftPad := 0
	if width > lineWidth {
		leftPad = (width - lineWidth) / 2
	}
	return text.Pad(strings.Repeat(" ", leftPad)+line, width)
}

func (m Model) renderActiveBody() string {
	if body, ok := m.renderOverlayBody(); ok {
		return body
	}
	return m.renderPrimaryBody()
}

func (m Model) renderOverlayBody() (string, bool) {
	overlayHeight := max(m.termHeight-3, 1)
	switch {
	case m.configCreated && !m.welcomeDismissed:
		return m.renderWelcomeOverlay(), true
	case m.helpVisible:
		var filterBar string
		if m.helpSearchFilter.Active {
			filterBar = "Search: " + m.helpSearchFilter.View() + "\n"
		} else if m.helpSearchFilter.Text != "" {
			filterBar = "Search: " + m.helpSearchFilter.Text + "\n"
		}
		return filterBar + m.helpViewport.View(), true
	case m.debugVisible:
		return m.debugViewport.View(), true
	case m.overlay.settings != nil:
		return m.overlay.settings.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.nowPlaying != nil:
		return m.overlay.nowPlaying.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.queueSummary != nil:
		return m.overlay.queueSummary.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.trackInfo != nil:
		return m.overlay.trackInfo.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.preview != nil:
		return m.overlay.preview.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.metadataEditor != nil:
		return m.overlay.metadataEditor.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.metadataImport != nil:
		return m.overlay.metadataImport.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.destPicker != nil:
		return m.overlay.destPicker.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.goTo != nil:
		return m.overlay.goTo.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.commandPalette != nil:
		return m.overlay.commandPalette.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.smartRuleEditor != nil:
		return m.overlay.smartRuleEditor.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.tagPicker != nil:
		return m.overlay.tagPicker.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.tagMergePicker != nil:
		return m.overlay.tagMergePicker.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.tagFilterPicker != nil:
		return m.overlay.tagFilterPicker.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.favoriteTargetPicker != nil:
		return m.overlay.favoriteTargetPicker.Render(m.termWidth, overlayHeight, *m.styles), true
	default:
		return "", false
	}
}

func (m Model) renderPrimaryBody() string {
	displayStyles := *m.styles
	if time.Now().Before(m.pulseUntil) {
		displayStyles = applyNowPlayingPulse(displayStyles)
	}
	nowPlayingIcon, _ := nowPlayingIndicator(displayStyles, m.playback.state)
	return m.renderPrimaryViewBody(displayStyles, nowPlayingIcon)
}

func (m Model) renderPrimaryViewBody(display styles.Styles, nowPlayingIcon string) string {
	switch m.activeView {
	case viewLibrary:
		return m.library.render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewPlaylists:
		return m.playlists.render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewQueue:
		return m.queue.render(display, m.playback.currentIndex, m.playback.state, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums)
	case viewBrowse:
		return m.browse.render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewFavorites:
		return m.favorites.render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewTags:
		return m.tagsView.render(display, m.favoriteHashes, m.playback.currentHash, nowPlayingIcon)
	case viewMixes:
		return m.mixes.render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewInsights:
		return m.insights.render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	default:
		return ""
	}
}

func (m Model) renderFooterHint() string {
	if m.hasActiveOverlay() || !m.showFooterHints {
		return ""
	}
	mode := HintModeDefault
	switch {
	case m.visual.active:
		mode = HintModeVisual
	case m.activeViewSelectionCount() > 0:
		mode = HintModeSelection
	}
	return RenderFooterHints(m.termWidth, m.activeViewContext(), mode, m.registry, *m.styles)
}
