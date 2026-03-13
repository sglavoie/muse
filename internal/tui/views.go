package tui

// viewID identifies which primary view is active.
type viewID int

const (
	viewLibrary   viewID = 1
	viewPlaylists viewID = 2
	viewQueue     viewID = 3
)

// LibraryView is a placeholder for the music library panel.
type LibraryView struct{}

func (v LibraryView) render() string { return "Library" }

// PlaylistsView is a placeholder for the playlists panel.
type PlaylistsView struct{}

func (v PlaylistsView) render() string { return "Playlists" }

// QueueView is a placeholder for the playback queue panel.
type QueueView struct{}

func (v QueueView) render() string { return "Queue" }
