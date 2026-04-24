package tui

import (
	"strings"

	"github.com/sglavoie/muse/internal/db"
)

// playlistTracksLoadedMsg carries the result of a background ReadPlaylistTracks
// call initiated by actionPlaylistOpen. Root handles it by transitioning the
// playlists view to TrackLevel with the loaded rows.
// When isRefresh is true the view is already at TrackLevel and the cursor is
// clamped in-place rather than reset to 0 (smart playlist refresh via 'r').
type playlistTracksLoadedMsg struct {
	tracks     []db.PlaylistTrackRow
	isRefresh  bool
	playlistID int64 // playlist that was loaded; used to discard stale results
}

// playlistTracksLoadErrorMsg carries an error from a failed ReadPlaylistTracks
// call. Root handles it by showing a transient error notification.
type playlistTracksLoadErrorMsg struct{ err error }

// PlaylistRenameMsg is dispatched when the user presses 'r' in the playlists
// view to rename the selected playlist.
type PlaylistRenameMsg struct{}

// playlistRenamedMsg is dispatched after a successful RenamePlaylist call.
// It carries the ID of the renamed playlist so root can reposition the cursor,
// and the refreshed playlist list so root can update the in-memory state.
type playlistRenamedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// playlistRenameErrorMsg is dispatched when RenamePlaylist returns an error.
// Root uses it to cancel the inline rename input and notify the user.
type playlistRenameErrorMsg struct {
	err error
}

// PlaylistCreateMsg is dispatched when the user presses 'c' in the playlists
// view to create a new playlist.
type PlaylistCreateMsg struct{}

// playlistCreatedMsg is dispatched after a successful CreatePlaylist call.
// It carries the refreshed playlist list and the ID of the newly created
// entry so that root can position the cursor on it by ID.
type playlistCreatedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// playlistCreateErrorMsg is dispatched when CreatePlaylist returns an error.
// Root uses it to cancel the inline create input and notify the user.
type playlistCreateErrorMsg struct {
	err error
}

// playlistAddErrorMsg is dispatched when AddTracksToPlaylist returns an error.
// Root uses it to display a transient error notification in the status bar.
type playlistAddErrorMsg struct {
	err error
}

// tracksAddedToPlaylistMsg is dispatched after a successful AddTracksToPlaylist
// call. It carries the refreshed playlist list (for updating track counts) and
// metadata for the status-bar notification.
type tracksAddedToPlaylistMsg struct {
	playlists    []db.PlaylistRow
	playlistName string
	count        int
}

// queueSavedAsPlaylistMsg is dispatched after successfully saving the queue as
// a new playlist. It carries the refreshed playlist list and metadata for the
// status-bar notification.
type queueSavedAsPlaylistMsg struct {
	playlists     []db.PlaylistRow
	playlistName  string
	count         int
	fromSelection bool // true when only bulk-selected entries were saved
}

// queueSaveAsPlaylistErrorMsg is dispatched when saving the queue as a
// playlist fails. Root cancels the inline input and shows an error notification.
type queueSaveAsPlaylistErrorMsg struct {
	err error
}

// playlistLister is a common interface satisfied by both playlistRenamer
// and playlistCreator for refreshing the playlist list after mutations.
type playlistLister interface {
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// refreshPlaylists reads the full playlist list using whichever lister is
// available. Returns nil rows (not an error) when all are nil.
func refreshPlaylists(listers ...playlistLister) ([]db.PlaylistRow, error) {
	for _, l := range listers {
		if l != nil {
			return l.ReadPlaylists()
		}
	}
	return nil, nil
}

// playlistNameExists checks whether a playlist with the given name already
// exists (case-insensitive), optionally excluding a playlist ID (for rename).
func playlistNameExists(playlists []db.PlaylistRow, name string, excludeID int64) bool {
	for _, p := range playlists {
		if p.ID != excludeID && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}
