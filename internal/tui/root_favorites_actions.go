package tui

import (
	tea "charm.land/bubbletea/v2"
)

// handleFavoriteKeyAction handles favorites-specific key actions.
// Returns (model, cmd, true) when the action was handled.
func (m Model) handleFavoriteKeyAction(action action) (Model, tea.Cmd, bool) {
	switch action {
	case actionFavoritesOpen:
		return m.handleFavoritesOpen()
	case actionFavoritesBack:
		if m.activeView == viewFavorites {
			m.favorites = m.favorites.back()
			return m, nil, true
		}
		return m, nil, false
	case actionToggleFavorite:
		return m.handleToggleFavorite()
	case actionBulkAction:
		if m.activeView == viewFavorites {
			return m.handleFavoritesBulkAction()
		}
		return m, nil, false
	default:
		return m, nil, false
	}
}
