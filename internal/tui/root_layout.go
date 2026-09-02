package tui

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/layout"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// activeViewContext maps the current activeView to its bindingContext so that
// the registry can check view-local bindings before falling back to global.
// For viewBrowse, the real sub-context (contextArtists/Albums/Genres) is resolved
// based on the active browse mode so that sub-view keybindings work correctly.
func (m Model) activeViewContext() keys.Context {
	if m.activeView == viewBrowse {
		switch m.browse.mode {
		case BrowseModeArtists:
			return keys.ContextArtists
		case BrowseModeAlbums:
			return keys.ContextAlbums
		case BrowseModeGenres:
			return keys.ContextGenres
		}
	}
	ctx := viewToContext(m.activeView)
	if ctx == keys.ContextGlobal {
		slog.Warn("activeViewContext: unhandled viewID, falling back to global bindings",
			"viewID", int(m.activeView))
	}
	return ctx
}

// resizable is implemented by every primary view that tracks its own
// terminal dimensions.
type resizable interface {
	SetSize(w, h int)
}

// setViewDimensions updates the width and height on all primary views.
// height is reduced by 4 to leave room for the status bar, separator, and
// header rows. When the footer hint strip is enabled it steals one more row.
func (m *Model) setViewDimensions(width, height int) {
	reserve := 4
	if m.showFooterHints {
		reserve = 5
	}
	viewHeight := max(height-reserve, 0)
	for _, view := range []resizable{&m.library, &m.playlists, &m.queue, &m.browse, &m.favorites, &m.tagsView, &m.mixes, &m.insights} {
		view.SetSize(width, viewHeight)
	}
}

// clampAllScrollOffsets adjusts the scroll offset for every view so that the
// cursor remains visible after a terminal resize.
func (m *Model) clampAllScrollOffsets() {
	libReserve := libtui.VisibleReserve(m.library)
	libMaxRows := layout.VisibleRows(m.library.Height, libReserve)
	m.library.ScrollOffset = sorting.AdjustScroll(m.library.Cursor, m.library.ScrollOffset, libMaxRows)

	queueMaxRows := m.queue.visibleRows()
	m.queue.scrollOffset = sorting.AdjustScroll(m.queue.cursor, m.queue.scrollOffset, queueMaxRows)

	plReserve := playlistsVisibleReserve(m.playlists)
	plMaxRows := layout.VisibleRows(m.playlists.height, plReserve)
	m.playlists.scrollOffset = sorting.AdjustScroll(m.playlists.cursor, m.playlists.scrollOffset, plMaxRows)

	artReserve := artistsVisibleReserve(m.browse.artists)
	artMaxRows := layout.VisibleRows(m.browse.artists.height, artReserve)
	m.browse.artists.scrollOffset = sorting.AdjustScroll(m.browse.artists.cursor, m.browse.artists.scrollOffset, artMaxRows)

	albReserve := albumsVisibleReserve(m.browse.albums)
	albMaxRows := layout.VisibleRows(m.browse.albums.height, albReserve)
	m.browse.albums.scrollOffset = sorting.AdjustScroll(m.browse.albums.cursor, m.browse.albums.scrollOffset, albMaxRows)

	favReserve := favoritesVisibleReserve(m.favorites)
	favMaxRows := layout.VisibleRows(m.favorites.height, favReserve)
	m.favorites.scrollOffset = sorting.AdjustScroll(m.favorites.cursor, m.favorites.scrollOffset, favMaxRows)

	genReserve := genresVisibleReserve(m.browse.genres)
	genMaxRows := layout.VisibleRows(m.browse.genres.height, genReserve)
	m.browse.genres.scrollOffset = sorting.AdjustScroll(m.browse.genres.cursor, m.browse.genres.scrollOffset, genMaxRows)

	tagReserve := tagsVisibleReserve(m.tagsView)
	tagMaxRows := layout.VisibleRows(m.tagsView.height, tagReserve)
	m.tagsView.scrollOffset = sorting.AdjustScroll(m.tagsView.cursor, m.tagsView.scrollOffset, tagMaxRows)

	if m.mixes.level == 1 {
		mixReserve := libtui.VisibleReserve(m.mixes.trackView)
		mixMaxRows := layout.VisibleRows(m.mixes.height, mixReserve)
		m.mixes.trackView.ScrollOffset = sorting.AdjustScroll(
			m.mixes.trackView.Cursor, m.mixes.trackView.ScrollOffset, mixMaxRows)
	}

	if m.insights.atTracks() {
		insReserve := libtui.VisibleReserve(m.insights.trackView)
		insMaxRows := layout.VisibleRows(m.insights.height, insReserve)
		m.insights.trackView.ScrollOffset = sorting.AdjustScroll(
			m.insights.trackView.Cursor, m.insights.trackView.ScrollOffset, insMaxRows)
	} else {
		m.insights.clampCursor()
	}
}

// tagsVisibleReserve returns the number of header rows to reserve in the tags view.
func tagsVisibleReserve(v TagsView) int {
	return drilldownVisibleReserve(
		v.level == tagtui.TagLevel,
		v.filter.Active || v.filter.Text != "",
		v.sortState.Field != sorting.SortDefault,
	)
}
