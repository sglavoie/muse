package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// Informational screens retain the global transport bindings independently of
// the underlying view. Editors and preview playback keep their own input routing.
func (m Model) handleInfoTransport(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	action := m.registry.Resolve(keys.ContextGlobal, msg.String())
	switch action {
	case keys.ActionPlayPause, keys.ActionStop, keys.ActionNext, keys.ActionPrevious,
		keys.ActionSeekBack, keys.ActionSeekForward, keys.ActionSeekBackLong, keys.ActionSeekForwardLong,
		keys.ActionVolumeUp, keys.ActionVolumeDown:
		next, cmd, handled := m.handlePlaybackKeyAction(action)
		return next.(Model), cmd, handled
	default:
		return m, nil, false
	}
}
