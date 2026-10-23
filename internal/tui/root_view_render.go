package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// View renders the active panel and a status bar, returning a tea.View.
func (m Model) View() tea.View {
	if m.terminalTooSmall() {
		v := tea.NewView(m.styles.RenderScreen(m.renderTerminalTooSmall(), m.termWidth, m.termHeight))
		v.AltScreen = true
		return m.finishView(v)
	}

	body := m.renderActiveBody()
	if !m.hasActiveOverlay() {
		tabSep := m.renderTitleDivider()
		body = m.renderTitleBar() + "\n" + tabSep + "\n" + body
	}

	statusBar := m.RenderStatusBar()
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
		detail := m.renderFocusedIdentity(padding - 1)
		if detail != "" && padding >= 3 {
			detailLines := strings.Count(detail, "\n") + 1
			body += strings.Repeat("\n", padding-detailLines) + "\n" + detail
		} else {
			body += strings.Repeat("\n", padding)
		}
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

	v := tea.NewView(m.styles.RenderScreen(content, m.termWidth, m.termHeight))
	v.AltScreen = true
	return m.finishView(v)
}

func (m Model) terminalTooSmall() bool {
	if m.termWidth <= 0 || m.termHeight <= 0 {
		return false
	}
	return m.termWidth < MinTerminalWidth || m.termHeight < MinTerminalHeight
}

func (m Model) renderTerminalTooSmall() string {
	width, height := m.termWidth, m.termHeight
	if width <= 0 || height <= 0 {
		return ""
	}

	lines := []string{
		terminalTooSmallTitle(width),
		fmt.Sprintf("Minimum size: %dx%d", MinTerminalWidth, MinTerminalHeight),
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
	case m.welcomeVisible():
		return m.renderWelcomeOverlay(), true
	case m.helpVisible:
		var filterBar string
		if m.helpSearchFilter.Active {
			filterBar = "Search: " + m.helpSearchFilter.View() + "\n"
		} else if m.helpSearchFilter.Text != "" {
			filterBar = "Search: " + m.helpSearchFilter.Text + "\n"
		}
		return filterBar + m.helpViewport.View() + "\n" + m.renderHelpFooter(), true
	case m.debugVisible:
		return m.debugViewport.View(), true
	case m.overlay.settings != nil:
		return m.overlay.settings.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.nowPlaying != nil:
		now := *m.overlay.nowPlaying
		now.State, now.Elapsed, now.Total = m.playback.state, m.playback.elapsedSecs, m.playback.totalSecs
		return now.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.messages != nil:
		return m.overlay.messages.Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.queueSummary != nil:
		return m.buildQueueSummary().Render(m.termWidth, overlayHeight, *m.styles), true
	case m.overlay.modesLegend != nil:
		return m.overlay.modesLegend.Render(m.termWidth, overlayHeight, *m.styles), true
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
	case m.overlay.seekTime != nil:
		prompt := m.overlay.seekTime.SeekTime
		prompt.CurrentSecs = m.playback.elapsedSecs
		prompt.DurationSecs = m.playback.totalSecs
		return prompt.Render(m.termWidth, overlayHeight, *m.styles), true
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
	case m.overlay.autoDJ != nil:
		return m.overlay.autoDJ.Render(m.termWidth, overlayHeight, *m.styles,
			m.autoDJPanelStatus(), m.autoDJPanelPreview()), true
	default:
		return "", false
	}
}

func (m Model) renderPrimaryBody() string {
	displayStyles := *m.styles
	if time.Now().Before(m.pulseUntil) {
		displayStyles = applyNowPlayingPulse(displayStyles)
	}
	nowPlayingIcon, _ := queue.NowPlayingIndicator(displayStyles, m.playback.state)
	return m.renderPrimaryViewBody(displayStyles, nowPlayingIcon)
}

func (m Model) renderPrimaryViewBody(display styles.Styles, nowPlayingIcon string) string {
	if body, ok := m.renderEmptyPrimary(display); ok {
		return body
	}
	switch m.ActiveView {
	case viewLibrary:
		if m.showsEmptyLibrary() {
			title, action := m.emptyLibraryGuidance()
			return display.RenderEmptyCard(m.library.Width, m.library.Height, title, []styles.EmptyAction{action})
		}
		return m.library.Render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewPlaylists:
		return m.playlists.Render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewQueue:
		m.queue.RemainingLabel = "· " + m.queueRemainingLabel()
		return m.queue.Render(display, m.playback.currentIndex, m.playback.state, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums)
	case viewBrowse:
		return m.browse.Render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewFavorites:
		return m.favorites.Render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewTags:
		return m.tagsView.Render(display, m.favoriteHashes, m.playback.currentHash, nowPlayingIcon)
	case viewMixes:
		return m.mixes.Render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	case viewInsights:
		return m.insights.Render(display, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, nowPlayingIcon)
	default:
		return ""
	}
}

func (m Model) renderFooterHint() string {
	if m.hasActiveOverlay() || !m.showFooterHints {
		return ""
	}
	if m.showsEmptyLibrary() {
		_, action := m.emptyLibraryGuidance()
		return m.styles.Dim.Render(text.CompactHints(action.Key+" "+action.Label+" · : commands · ? help", m.termWidth, action.Key, "?"))
	}
	if m.ActiveView == viewInsights {
		if finding := m.insights.FocusedHealthRow(); finding != nil {
			hint := finding.ActionHint() + " · enter inspect · j/k nav · `/~ panel · ? help"
			if strings.HasPrefix(finding.ActionHint(), "enter ") {
				hint = finding.ActionHint() + " · j/k nav · `/~ panel · ? help"
			}
			return m.styles.Dim.Render(text.CompactHints(hint, m.termWidth, strings.Fields(finding.ActionHint())[0], "?"))
		}
	}
	mode := keys.HintModeDefault
	switch {
	case m.activeFilterOps() != nil:
		mode = keys.HintModeFilter
	case m.visual.active:
		mode = keys.HintModeVisual
	case m.activeViewSelectionCount() > 0:
		mode = keys.HintModeSelection
		if m.atCollectionTrackLevel() {
			mode = keys.HintModeTrackSelection
		}
	case !m.hasFooterTarget():
		mode = m.emptyHintMode()
	case m.footerOpensCollectionOnly():
		mode = keys.HintModeCollections
	case m.ActiveView == viewMixes && m.mixes.Level == 1,
		m.atCollectionTrackLevel():
		mode = keys.HintModeTracks
	}
	_, _, _, track := m.focusedTrackForPalette()
	hint := keys.RenderFooterHints(m.termWidth, m.activeViewContext(), mode, m.registry, *m.styles, track && mode != keys.HintModeFilter && mode != keys.HintModeVisual, m.activeViewSelectionCount())
	if mode == keys.HintModeFilter {
		summary := m.filterQuerySummary()
		room := m.termWidth - ansi.StringWidth(hint) - 3
		if summary != "" && room >= 16 {
			hint += m.styles.Dim.Render(" · " + ansi.Truncate(summary, room, "…"))
		}
	}
	return hint
}
