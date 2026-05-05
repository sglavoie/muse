package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// hasActiveOverlay reports whether any overlay or the help screen is
// currently displayed. Used to suppress the title bar when an overlay
// is active. Overlay priority (highest to lowest): help > debug > settings >
// nowPlaying > trackInfo > preview > metadataEditor > destPicker. When
// adding a new overlay, update this method and the View() rendering chain.
func (m Model) hasActiveOverlay() bool {
	return m.helpVisible ||
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
		case viewRecentlyAdded:
			body = m.recentlyAdded.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
		case viewHistory:
			body = m.history.render(*m.styles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash)
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
	if padding := m.termHeight - bodyLines - 2; padding > 0 {
		body += strings.Repeat("\n", padding)
	}
	content := body + "\n" + statusBar
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
		fullWidth += len(viewName(id))
		if i < len(viewOrder)-1 {
			fullWidth += len(sep)
		}
	}

	if m.termWidth >= fullWidth {
		// Wide layout: show all tabs with numbered badges.
		var parts []string
		for i, id := range viewOrder {
			name := viewName(id)
			var entry string
			if i < 10 {
				badge := m.styles.Badge.Render(fmt.Sprintf("%d", (i+1)%10))
				if id == m.activeView {
					entry = badge + " " + m.styles.Heading.Render(name)
				} else {
					entry = badge + " " + m.styles.TextTertiary.Render(name)
				}
			} else {
				if id == m.activeView {
					entry = m.styles.Heading.Render(name)
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
	shortNames := map[viewID]string{
		viewRecentlyAdded: "Recent",
	}
	getName := func(id viewID) string {
		if short, ok := shortNames[id]; ok {
			return short
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
			narrowParts = append(narrowParts, m.styles.Heading.Render(name))
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
	return sb.render(m.termWidth, *m.styles)
}
