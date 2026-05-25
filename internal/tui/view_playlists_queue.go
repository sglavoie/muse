package tui

import (
	tea "charm.land/bubbletea/v2"

	libtui "github.com/sglavoie/muse/internal/tui/library"
)

func (v PlaylistsView) enqueueFocusedTrack(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, v.sourcePlaylistName(), -1)
}

func (v PlaylistsView) playNextFocusedTrack(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	track, ok := v.focusedTrack()
	return libtui.EnqueueTrackCmd(track, ok, qa, prevQueueLen, v.sourcePlaylistName(), insertAfter)
}

func (v PlaylistsView) sourcePlaylistName() string {
	if v.listCursor >= 0 && v.listCursor < len(v.playlists) {
		return v.playlists[v.listCursor].Name
	}
	return ""
}
