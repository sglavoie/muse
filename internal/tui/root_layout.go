package tui

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/tui/layout"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// activeViewContext maps the current activeView to its bindingContext so that
// the registry can check view-local bindings before falling back to global.
func (m Model) activeViewContext() bindingContext {
	ctx := viewToContext(m.activeView)
	if ctx == contextGlobal {
		slog.Warn("activeViewContext: unhandled viewID, falling back to global bindings",
			"viewID", int(m.activeView))
	}
	return ctx
}

// resizable is implemented by every primary view that tracks its own
// terminal dimensions.
type resizable interface {
	setSize(w, h int)
}

// setViewDimensions updates the width and height on all primary views.
// height is reduced by 3 to leave room for the status bar and header rows.
func (m *Model) setViewDimensions(width, height int) {
	viewHeight := max(height-3, 0)
	for _, view := range []resizable{&m.library, &m.recentlyAdded, &m.history, &m.playlists, &m.queue, &m.artists, &m.albums, &m.favorites, &m.genres, &m.tagsView} {
		view.setSize(width, viewHeight)
	}
}

// clampAllScrollOffsets adjusts the scroll offset for every view so that the
// cursor remains visible after a terminal resize.
func (m *Model) clampAllScrollOffsets() {
	libMaxRows := layout.VisibleRows(m.library.height, 1)
	m.library.scrollOffset = adjustScroll(m.library.cursor, m.library.scrollOffset, libMaxRows)

	histMaxRows := layout.VisibleRows(m.history.height, 1)
	m.history.scrollOffset = adjustScroll(m.history.cursor, m.history.scrollOffset, histMaxRows)

	queueMaxRows := m.queue.visibleRows()
	m.queue.scrollOffset = adjustScroll(m.queue.cursor, m.queue.scrollOffset, queueMaxRows)

	plReserve := 1
	if m.playlists.level == pltui.TrackLevel {
		plReserve = 4
	}
	plMaxRows := layout.VisibleRows(m.playlists.height, plReserve)
	m.playlists.scrollOffset = adjustScroll(m.playlists.cursor, m.playlists.scrollOffset, plMaxRows)

	artReserve := artistsVisibleReserve(m.artists)
	artMaxRows := layout.VisibleRows(m.artists.height, artReserve)
	m.artists.scrollOffset = adjustScroll(m.artists.cursor, m.artists.scrollOffset, artMaxRows)

	albReserve := albumsVisibleReserve(m.albums)
	albMaxRows := layout.VisibleRows(m.albums.height, albReserve)
	m.albums.scrollOffset = adjustScroll(m.albums.cursor, m.albums.scrollOffset, albMaxRows)

	favReserve := favoritesVisibleReserve(m.favorites)
	favMaxRows := layout.VisibleRows(m.favorites.height, favReserve)
	m.favorites.scrollOffset = adjustScroll(m.favorites.cursor, m.favorites.scrollOffset, favMaxRows)

	genReserve := genresVisibleReserve(m.genres)
	genMaxRows := layout.VisibleRows(m.genres.height, genReserve)
	m.genres.scrollOffset = adjustScroll(m.genres.cursor, m.genres.scrollOffset, genMaxRows)

	tagReserve := tagsVisibleReserve(m.tagsView)
	tagMaxRows := layout.VisibleRows(m.tagsView.height, tagReserve)
	m.tagsView.scrollOffset = adjustScroll(m.tagsView.cursor, m.tagsView.scrollOffset, tagMaxRows)
}

// tagsVisibleReserve returns the number of header rows to reserve in the tags view.
func tagsVisibleReserve(v TagsView) int {
	var reserve int
	switch v.level {
	case tagtui.TagLevel:
		if v.filter.Active || v.filter.Text != "" {
			reserve = 1
		}
	default: // TrackLevel
		reserve = 3 // heading + separator + column header
	}
	if v.sortState.Field != SortDefault {
		reserve++
	}
	return reserve
}
