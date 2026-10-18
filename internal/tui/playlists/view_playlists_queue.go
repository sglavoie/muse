package playlists

import (
	tea "charm.land/bubbletea/v2"

	libtui "github.com/sglavoie/muse/internal/tui/library"
)

func (v PlaylistsView) EnqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.FocusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, v.sourcePlaylistName(), -1)
}

func (v PlaylistsView) sourcePlaylistName() string {
	if v.ListCursor >= 0 && v.ListCursor < len(v.Playlists) {
		return v.Playlists[v.ListCursor].Name
	}
	return ""
}
