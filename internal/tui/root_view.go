package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// applyNowPlayingPulse returns a copy of s with Reverse(true) applied to every
// lipgloss.Style used by per-view renders to mark the now-playing row. The
// inversion lasts ~250 ms after a track change (see pulseDuration) so the user
// notices the transition.
func applyNowPlayingPulse(s styles.Styles) styles.Styles {
	s.NowPlaying = s.NowPlaying.Reverse(true)
	s.NowPlayingFavorite = s.NowPlayingFavorite.Reverse(true)
	s.NowPlayingFavArtist = s.NowPlayingFavArtist.Reverse(true)
	s.NowPlayingFavAlbum = s.NowPlayingFavAlbum.Reverse(true)
	s.NowPlayingTitle = s.NowPlayingTitle.Reverse(true)
	s.NowPlayingRow = s.NowPlayingRow.Reverse(true)
	s.NowPlayingRowFavorite = s.NowPlayingRowFavorite.Reverse(true)
	s.NowPlayingRowFavArtist = s.NowPlayingRowFavArtist.Reverse(true)
	s.NowPlayingRowFavAlbum = s.NowPlayingRowFavAlbum.Reverse(true)
	return s
}

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
		displayStyles := *m.styles
		if time.Now().Before(m.pulseUntil) {
			displayStyles = applyNowPlayingPulse(displayStyles)
		}
		npIcon, _ := nowPlayingIndicator(displayStyles, m.playback.state)
		switch m.activeView {
		case viewLibrary:
			body = m.library.render(displayStyles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, npIcon)
		case viewPlaylists:
			body = m.playlists.render(displayStyles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, npIcon)
		case viewQueue:
			body = m.queue.render(displayStyles, m.playback.currentIndex, m.playback.state, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums)
		case viewBrowse:
			body = m.browse.render(displayStyles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, npIcon)
		case viewFavorites:
			body = m.favorites.render(displayStyles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, npIcon)
		case viewTags:
			body = m.tagsView.render(displayStyles, m.favoriteHashes, m.playback.currentHash, npIcon)
		case viewMixes:
			body = m.mixes.render(displayStyles, m.favoriteHashes, m.favoriteArtists, m.favoriteAlbums, m.playback.currentHash, npIcon)
		}
	}

	// Prepend the centralized title bar when showing a primary view.
	if !m.hasActiveOverlay() {
		tabSep := m.styles.TextTertiary.Render(strings.Repeat("─", m.termWidth))
		body = m.renderTitleBar() + "\n" + tabSep + "\n" + body
	}

	statusBar := m.renderStatusBar()
	// Render the context-aware footer hint strip above the status bar when
	// enabled, the active view is a primary view (no overlay), and the
	// context has registered primaries. The hint set is selection-aware:
	// visual mode wins, then non-empty selection, then default.
	footerHint := ""
	if !m.hasActiveOverlay() && m.showFooterHints {
		mode := HintModeDefault
		switch {
		case m.visual.active:
			mode = HintModeVisual
		case m.activeViewSelectionCount() > 0:
			mode = HintModeSelection
		}
		footerHint = RenderFooterHints(m.termWidth, m.activeViewContext(), mode, m.registry, *m.styles)
	}
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
	var content string
	switch {
	case m.confirmationBar != "" && footerHint != "":
		confirmLine := m.styles.Warning.Width(m.termWidth).Render("  " + m.confirmationBar)
		content = body + "\n" + confirmLine + "\n" + footerHint + "\n" + statusBar
	case m.confirmationBar != "":
		confirmLine := m.styles.Warning.Width(m.termWidth).Render("  " + m.confirmationBar)
		content = body + "\n" + confirmLine + "\n" + statusBar
	case footerHint != "":
		content = body + "\n" + footerHint + "\n" + statusBar
	default:
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

	// Queue now-playing indicator (2 display cols; shown only when Queue is not active).
	queueIndicator := ""
	if m.activeView != viewQueue {
		switch {
		case m.playback.state == PlaybackPlaying && m.playback.currentIndex >= 0:
			queueIndicator = " ♪"
		case m.playback.state == PlaybackPaused:
			queueIndicator = " ⏸"
		}
	}

	// groupAfter marks which tabs close a logical group (separator inserted after them).
	groupAfter := map[viewID]bool{viewQueue: true, viewFavorites: true}

	fullWidth := chevronWidth
	for i, id := range viewOrder {
		if i < 10 {
			fullWidth += badgeWidth
		}
		nameLen := len(m.viewDisplayName(id))
		if id == m.activeView {
			nameLen += 2 // " " + name + " " padding in ActiveTab style
		}
		if id == viewQueue && queueIndicator != "" {
			nameLen += 2 // " ♪" or " ⏸" (1 space + 1 display-column symbol)
		}
		fullWidth += nameLen
		if i < len(viewOrder)-1 {
			if groupAfter[id] {
				fullWidth += 3 // " │ "
			} else {
				fullWidth += len(sep)
			}
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
					entry = badge + " " + m.styles.ActiveTab.Render(" "+name+" ") + filterBadge
				} else {
					entry = badge + " " + m.styles.TextTertiary.Render(name)
				}
			} else {
				if id == m.activeView {
					entry = m.styles.ActiveTab.Render(" "+name+" ") + filterBadge
				} else {
					entry = m.styles.TextTertiary.Render(name)
				}
			}
			if id == viewQueue && queueIndicator != "" {
				if m.playback.state == PlaybackPlaying {
					entry += m.styles.Success.Render(queueIndicator)
				} else {
					entry += m.styles.Warning.Render(queueIndicator)
				}
			}
			parts = append(parts, entry)
		}
		groupSep := m.styles.Dim.Render(" │ ")
		var b strings.Builder
		for i, part := range parts {
			if i > 0 {
				if groupAfter[viewOrder[i-1]] {
					b.WriteString(groupSep)
				} else {
					b.WriteString(sep)
				}
			}
			b.WriteString(part)
		}
		return m.styles.TextTertiary.Render("‹ ") + b.String() + m.styles.TextTertiary.Render(" ›")
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
	used := len(getName(viewOrder[activeIdx])) + 2 // +2 for ActiveTab " " padding

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
			narrowParts = append(narrowParts, m.styles.ActiveTab.Render(" "+name+" ")+filterBadge)
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
	sb.queueEmpty = len(m.queue.entries) == 0
	if m.playback.state == PlaybackPlaying && m.playback.currentIndex >= 0 && len(m.queue.entries) > 0 {
		sb.queuePosition = fmt.Sprintf("%d of %d", m.playback.currentIndex+1, len(m.queue.entries))
	} else {
		sb.queuePosition = ""
	}
	if m.playback.elapsedSecs > 0 || m.playback.totalSecs > 0 {
		sb.elapsed = formatTime(m.playback.elapsedSecs)
		sb.total = formatTime(m.playback.totalSecs)
	}
	sb.elapsedSecs = m.playback.elapsedSecs
	sb.totalSecs = m.playback.totalSecs
	sb.yankCount = len(m.yankBuffer)
	sb.yankIsCut = m.yankIsCut
	sb.selectionCount = m.activeViewSelectionCount()
	if m.undo.slot.action != nil {
		sb.undoExpiresAt = m.undo.slot.expiresAt
	}
	sb.bucketLabel = m.focusedBucketLabel()
	sb.longOp = m.longOp
	return sb.render(m.termWidth, *m.styles)
}

// activeViewSelectionCount returns the bulk-selection size for the active
// view. Used by both the status bar (right-edge "N sel" indicator) and the
// state strip (◉ N selected chip) so they share a single source of truth.
// Views without a bulk-selection concept (e.g. Playlists, Tags, Mixes)
// return 0 even when underlying selection maps are populated, mirroring
// the existing status-bar behavior.
func (m Model) activeViewSelectionCount() int {
	switch m.activeView {
	case viewLibrary:
		return len(m.library.selectedHashes)
	case viewQueue:
		return len(m.queue.selectedHashes())
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return len(m.browse.artists.selectedHashes)
		case BrowseModeAlbums:
			return len(m.browse.albums.selectedHashes)
		case BrowseModeGenres:
			return len(m.browse.genres.selectedHashes)
		}
	case viewFavorites:
		return len(m.favorites.selectedItems)
	}
	return 0
}

// focusedBucketLabel returns the bucket label for the focused row in the
// active view, or "" when no bucketed sort is in effect. Used by the status
// bar to append a chunk hint to the queue position display.
func (m Model) focusedBucketLabel() string {
	switch m.activeView {
	case viewLibrary:
		field := m.library.sortState.Field
		if !SortFieldHasBuckets(field) {
			return ""
		}
		t, ok := m.library.view().FocusedTrack()
		if !ok {
			return ""
		}
		return TrackBucket(field, t, time.Now())
	case viewQueue:
		field := m.queue.sortState.Field
		if !SortFieldHasBuckets(field) {
			return ""
		}
		entries := m.queue.filteredEntries()
		if m.queue.cursor < 0 || m.queue.cursor >= len(entries) {
			return ""
		}
		return QueueEntryBucket(field, entries[m.queue.cursor])
	}
	return ""
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
	case viewBrowse:
		switch m.browse.mode {
		case BrowseModeArtists:
			return committed(m.browse.artists.filter.Text, m.browse.artists.filter.Active)
		case BrowseModeAlbums:
			return committed(m.browse.albums.filter.Text, m.browse.albums.filter.Active)
		case BrowseModeGenres:
			return committed(m.browse.genres.filter.Text, m.browse.genres.filter.Active)
		}
	case viewFavorites:
		return committed(m.favorites.filter.Text, m.favorites.filter.Active)
	case viewTags:
		return committed(m.tagsView.filter.Text, m.tagsView.filter.Active)
	}
	return false
}
