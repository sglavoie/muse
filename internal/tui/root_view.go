package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// hasActiveOverlay reports whether any overlay or the help screen is
// currently displayed. Used to suppress the title bar when an overlay
// is active. Overlay priority (highest to lowest): welcome > help > debug >
// settings > nowPlaying > trackInfo > preview > metadataEditor > destPicker.
// When adding a new overlay, update this method and the View() rendering chain.
func (m Model) hasActiveOverlay() bool {
	return (m.configCreated && !m.welcomeDismissed) ||
		m.helpVisible ||
		m.debugVisible ||
		m.statsVisible ||
		m.overlay.settings != nil ||
		m.overlay.nowPlaying != nil ||
		m.overlay.queueSummary != nil ||
		m.overlay.trackInfo != nil ||
		m.overlay.preview != nil ||
		m.overlay.metadataEditor != nil ||
		m.overlay.destPicker != nil ||
		m.overlay.goTo != nil ||
		m.overlay.smartRuleEditor != nil ||
		m.overlay.commandPalette != nil ||
		m.overlay.tagPicker != nil ||
		m.overlay.tagMergePicker != nil ||
		m.overlay.tagFilterPicker != nil
}

// View renders the active panel and a status bar, returning a tea.View.
// When the help overlay is visible it replaces the body content entirely.
func (m Model) View() tea.View {
	var body string
	// Render overlays in priority order (highest first). See hasActiveOverlay().
	switch {
	case m.configCreated && !m.welcomeDismissed:
		body = m.renderWelcomeOverlay()
	case m.helpVisible:
		var filterBar string
		if m.helpSearchFilter.Active {
			filterBar = "Search: " + m.helpSearchFilter.View() + "\n"
		} else if m.helpSearchFilter.Text != "" {
			filterBar = "Search: " + m.helpSearchFilter.Text + "\n"
		}
		body = filterBar + m.helpViewport.View()
	case m.debugVisible:
		body = m.debugViewport.View()
	case m.statsVisible:
		body = m.renderStatsOverlay()
	case m.overlay.settings != nil:
		body = m.overlay.settings.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.nowPlaying != nil:
		body = m.overlay.nowPlaying.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.queueSummary != nil:
		body = m.overlay.queueSummary.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.trackInfo != nil:
		body = m.overlay.trackInfo.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.preview != nil:
		body = m.overlay.preview.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.metadataEditor != nil:
		body = m.overlay.metadataEditor.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.destPicker != nil:
		body = m.overlay.destPicker.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.goTo != nil:
		body = m.overlay.goTo.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.commandPalette != nil:
		body = m.overlay.commandPalette.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.smartRuleEditor != nil:
		body = m.overlay.smartRuleEditor.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.tagPicker != nil:
		body = m.overlay.tagPicker.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.tagMergePicker != nil:
		body = m.overlay.tagMergePicker.Render(m.termWidth, m.termHeight, *m.styles)
	case m.overlay.tagFilterPicker != nil:
		body = m.overlay.tagFilterPicker.Render(m.termWidth, m.termHeight, *m.styles)
	default:
		switch m.activeView {
		case viewLibrary:
			body = m.library.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewPlaylists:
			body = m.playlists.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewQueue:
			body = m.queue.render(*m.styles, m.playback.currentIndex, m.playback.state, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums)
		case viewArtists:
			body = m.artists.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewAlbums:
			body = m.albums.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewFavorites:
			body = m.favorites.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewGenres:
			body = m.genres.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewTags:
			body = m.tagsView.render(*m.styles, m.favoriteHashes, m.playback.currentHash)
		case viewMixes:
			body = m.mixes.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		}
	}

	// Prepend the centralized title bar when showing a primary view.
	if !m.hasActiveOverlay() {
		tabSep := m.styles.TextTertiary.Render(strings.Repeat("─", m.termWidth))
		body = m.renderTitleBar() + "\n" + tabSep + "\n" + body
	}

	statusBar := m.renderStatusBar()
	bodyLines := strings.Count(body, "\n") + 1
	confirmExtra := 0
	if m.confirmationBar != "" {
		confirmExtra = 1
	}
	if padding := m.termHeight - bodyLines - 2 - confirmExtra; padding > 0 {
		body += strings.Repeat("\n", padding)
	}
	var content string
	if m.confirmationBar != "" {
		confirmLine := m.styles.Warning.Width(m.termWidth).Render("  " + m.confirmationBar)
		content = body + "\n" + confirmLine + "\n" + statusBar
	} else {
		content = body + "\n" + statusBar
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderTitleBar builds the centralized title bar showing view names with
// chevrons and numbered badges. When the terminal is wide enough all views
// are shown; otherwise a centered window of as many tabs as fit is displayed,
// with abbreviated labels for long view names.
func (m Model) renderTitleBar() string {
	// Compute the width needed to show all tabs:
	// "‹ " + "1 Name1  2 Name2  ...  N NameN" + " ›"
	// Each tab: badge digit (1) + space (1) + name length.
	const chevronWidth = 4 // "‹ " + " ›"
	const badgeWidth = 2   // digit + space before name
	sep := "  "
	fullWidth := chevronWidth
	for i, id := range viewOrder {
		if i < 10 {
			fullWidth += badgeWidth
		}
		fullWidth += len(m.viewDisplayName(id))
		if i < len(viewOrder)-1 {
			fullWidth += len(sep)
		}
	}

	filterBadge := ""
	if m.hasCommittedFilter() {
		filterBadge += m.styles.Dim.Render(" [filtered]")
	}
	if m.activeTagFilter != nil {
		filterBadge += m.styles.Dim.Render(" [tag: " + m.tagNames[*m.activeTagFilter] + "]")
	}

	if m.termWidth >= fullWidth {
		// Wide layout: show all tabs with numbered badges.
		var parts []string
		for i, id := range viewOrder {
			name := m.viewDisplayName(id)
			var entry string
			if i < 10 {
				badge := m.styles.Badge.Render(fmt.Sprintf("%d", (i+1)%10))
				if id == m.activeView {
					entry = badge + " " + m.styles.Heading.Render(name) + filterBadge
				} else {
					entry = badge + " " + m.styles.TextTertiary.Render(name)
				}
			} else {
				if id == m.activeView {
					entry = m.styles.Heading.Render(name) + filterBadge
				} else {
					entry = m.styles.TextTertiary.Render(name)
				}
			}
			parts = append(parts, entry)
		}
		return m.styles.TextTertiary.Render("‹ ") + strings.Join(parts, sep) + m.styles.TextTertiary.Render(" ›")
	}

	// Narrow layout: show as many tabs as fit, centered on the active view.
	// Use abbreviated labels for views with long names.
	getName := func(id viewID) string {
		if id == viewLibrary {
			switch m.library.mode {
			case LibraryModeRecent:
				return "Recent"
			case LibraryModeHistory:
				return "History"
			}
		}
		return viewName(id)
	}

	activeIdx := 0
	for i, id := range viewOrder {
		if id == m.activeView {
			activeIdx = i
			break
		}
	}

	available := m.termWidth - chevronWidth
	left := activeIdx
	right := activeIdx
	used := len(getName(viewOrder[activeIdx]))

	for {
		tryLeft := left > 0
		tryRight := right < len(viewOrder)-1
		if !tryLeft && !tryRight {
			break
		}
		expanded := false
		if tryRight {
			w := len(sep) + len(getName(viewOrder[right+1]))
			if used+w <= available {
				right++
				used += w
				expanded = true
			}
		}
		if tryLeft {
			w := len(sep) + len(getName(viewOrder[left-1]))
			if used+w <= available {
				left--
				used += w
				expanded = true
			}
		}
		if !expanded {
			break
		}
	}

	var narrowParts []string
	for i := left; i <= right; i++ {
		id := viewOrder[i]
		name := getName(id)
		if id == m.activeView {
			narrowParts = append(narrowParts, m.styles.Heading.Render(name)+filterBadge)
		} else {
			narrowParts = append(narrowParts, m.styles.TextTertiary.Render(name))
		}
	}
	return m.styles.TextTertiary.Render("‹ ") + strings.Join(narrowParts, sep) + m.styles.TextTertiary.Render(" ›")
}

// renderStatusBar builds the status bar line shown at the bottom of the view.
// It syncs the model-owned playbackState, repeat, shuffle, and configCreated
// into a local StatusBar copy so that the render logic has a complete picture.
func (m Model) renderStatusBar() string {
	sb := m.statusBar
	sb.playbackState = m.playback.state
	sb.repeatMode = m.playback.repeatMode
	sb.shuffle = m.playback.shuffle
	sb.stopAfterCurrent = m.playback.stopAfterCurrent
	sb.visualMode = m.visual.active
	sb.radioMode = m.radio.enabled
	sb.configCreated = m.configCreated
	sb.watchDegraded = m.watchDegraded
	if m.activeTagFilter != nil {
		sb.activeTagName = m.tagNames[*m.activeTagFilter]
	}
	sb.queueEmpty = len(m.queue.entries) == 0
	if m.playback.elapsedSecs > 0 || m.playback.totalSecs > 0 {
		sb.elapsed = formatTime(m.playback.elapsedSecs)
		sb.total = formatTime(m.playback.totalSecs)
	}
	sb.elapsedSecs = m.playback.elapsedSecs
	sb.totalSecs = m.playback.totalSecs
	sb.yankCount = len(m.yankBuffer)
	sb.yankIsCut = m.yankIsCut
	switch m.activeView {
	case viewLibrary:
		sb.selectionCount = len(m.library.selectedHashes)
	case viewQueue:
		sb.selectionCount = len(m.queue.selectedHashes())
	case viewArtists:
		sb.selectionCount = len(m.artists.selectedHashes)
	case viewAlbums:
		sb.selectionCount = len(m.albums.selectedHashes)
	case viewGenres:
		sb.selectionCount = len(m.genres.selectedHashes)
	case viewFavorites:
		sb.selectionCount = len(m.favorites.selectedItems)
	}
	if m.undo.slot.action != nil {
		sb.undoExpiresAt = m.undo.slot.expiresAt
	}
	return sb.render(m.termWidth, *m.styles)
}

// hasCommittedFilter reports whether the active view has a committed
// (non-empty, non-active) text filter applied.
func (m Model) hasCommittedFilter() bool {
	committed := func(text string, active bool) bool { return text != "" && !active }
	switch m.activeView {
	case viewLibrary:
		return committed(m.library.filter.Text, m.library.filter.Active)
	case viewQueue:
		return committed(m.queue.filter.Text, m.queue.filter.Active)
	case viewArtists:
		return committed(m.artists.filter.Text, m.artists.filter.Active)
	case viewAlbums:
		return committed(m.albums.filter.Text, m.albums.filter.Active)
	case viewGenres:
		return committed(m.genres.filter.Text, m.genres.filter.Active)
	case viewFavorites:
		return committed(m.favorites.filter.Text, m.favorites.filter.Active)
	case viewTags:
		return committed(m.tagsView.filter.Text, m.tagsView.filter.Active)
	}
	return false
}
