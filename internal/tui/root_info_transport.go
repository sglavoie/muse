package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// Informational screens retain the global transport bindings independently of
// the underlying view. Editors and preview playback keep their own input routing.
func (m Model) handleInfoTransport(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	action := m.registry.Resolve(keys.ContextGlobal, msg.String())
	// Seek keys preview their target before mpv seeks; see root_seek_preview.go.
	if next, cmd, ok := m.handleSeekKey(action); ok {
		return next, cmd, true
	}
	switch action {
	case keys.ActionPlayPause, keys.ActionStop, keys.ActionNext, keys.ActionPrevious,
		keys.ActionVolumeUp, keys.ActionVolumeDown:
		next, cmd, handled := m.handlePlaybackKeyAction(action)
		return next.(Model), cmd, handled
	default:
		return m, nil, false
	}
}
