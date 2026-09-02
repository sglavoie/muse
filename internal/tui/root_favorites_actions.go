package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// handleFavoriteKeyAction handles favorites-specific key actions.
// Returns (model, cmd, true) when the action was handled.
func (m Model) handleFavoriteKeyAction(action keys.Action) (Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionFavoritesOpen:
		return m.handleFavoritesOpen()
	case keys.ActionFavoritesBack:
		if m.activeView == viewFavorites {
			m = exitVisualMode(m)
			m.favorites = m.favorites.back()
			return m, nil, true
		}
		return m, nil, false
	case keys.ActionToggleFavorite:
		return m.handleToggleFavorite()
	case keys.ActionBulkAction:
		if m.activeView == viewFavorites {
			return m.handleFavoritesBulkAction()
		}
		return m, nil, false
	default:
		return m, nil, false
	}
}
