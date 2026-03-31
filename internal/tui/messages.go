package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// undoKind distinguishes which type of destructive action is reversible.
type undoKind uint8

const (
	undoNone                         undoKind = iota
	undoKindEnqueue                           // single track added via TrackEnqueuedMsg
	undoKindDequeue                           // single track removed via DequeueTrackMsg
	undoKindClear                             // all tracks removed via ClearQueueMsg
	undoKindDeletePlaylist                    // playlist soft-deleted via actionPlaylistDelete at ListLevel
	undoKindRemovePlaylistTrack               // track soft-removed via actionPlaylistDelete at TrackLevel
	undoKindBatchEnqueue                      // batch of tracks added via actionPlaylistEnqueueAll
	undoKindBulkDequeue                       // multiple selected tracks removed via bulk dequeue
	undoKindBulkRemovePlaylistTracks          // multiple selected tracks removed from playlist via 'a'
	undoKindBulkEditMetadata                  // bulk metadata edit via 'M' with selection
	undoKindRemoveFavorite                    // single favorite removed (any view)
	undoKindBulkRemoveFavorites               // multiple favorites bulk-removed (Favorites view)
	undoKindAddFavorite                       // single favorite added (any non-Favorites view)
	undoKindBulkAddFavorites                  // multiple favorites bulk-added (any non-Favorites view)
)

// undoSlot holds the reversible state for the most recent destructive action.
// Only one slot is maintained; a new action replaces the prior one.
// action is nil when no undo is available.
type undoSlot struct {
	action    undoAction
	expiresAt time.Time

	// cancelPurge is closed when this slot is superseded by a new destructive
	// action or when undo is successfully performed. The time.AfterFunc purge
	// callback checks this channel before running to avoid purging rows that
	// belong to a newer undo slot (e.g. same-second dequeue followed by another
	// dequeue of the same DB row ID after an undo).
	cancelPurge chan struct{}
}

// tickMsg is dispatched every second by tickCmd. It drives elapsed-time
// display updates and transient-notification expiry checks.
type tickMsg struct{}

// sessionTickMsg is dispatched every sessionTickInterval by sessionTickCmd.
// On each tick the current playback position is persisted to the session table,
// but only when playback is active (PlaybackPlaying).
type sessionTickMsg struct{}

// syncPositionMsg is dispatched every positionSyncInterval to trigger a
// playback-position update on the player.
type syncPositionMsg struct{}

// MissingTrackMsg is dispatched by the player when it attempts to play a
// queued track whose file cannot be found. The TUI marks that entry as missing
// in the queue view and displays a transient error in the status bar.
type MissingTrackMsg struct {
	Path string
}

// PlayerStatusMsg carries a transient status string from the player backend
// (e.g. "Connecting to mpv…" or "mpv disconnected, restarting…") into the
// TUI status bar. Wire it via the OnStatus callback in mgr.SetEventCallbacks.
type PlayerStatusMsg struct {
	Text string
}

// PlayQueueIndexMsg is dispatched by the queue view when the user presses
// Enter on a track. Root handles it by starting playback from that index.
type PlayQueueIndexMsg struct {
	Index int
}

// MissingTrackSelectedMsg is dispatched by the queue view when the user
// presses Enter on a missing track. Root handles it by showing a transient
// notification explaining why playback cannot start.
type MissingTrackSelectedMsg struct{}

// PlayerDisconnectedMsg is dispatched when the mpv process crashes or its IPC
// socket disconnects unexpectedly. Root handles it by resetting the TUI
// playback state so the play/pause icon and progress bar don't remain stale
// during the reconnection window.
type PlayerDisconnectedMsg struct{}

// PlaybackEndedMsg is dispatched by the player when auto-advance exhausts the
// queue (no more playable entries). Root handles it by resetting the playback
// state to idle so the space-bar toggle stays synchronized with mpv's state.
type PlaybackEndedMsg struct{}

// PauseChangedMsg is dispatched by the player when mpv's pause property
// changes externally (e.g. via AirPods hardware button). Root handles it by
// syncing the TUI playback state with mpv's actual pause state.
type PauseChangedMsg struct {
	Paused bool
}

// RemoteActionMsg is sent when an external signal (SIGUSR1) requests a
// playback action, e.g. from a global keybinding. ActionName maps to an
// action constant via remoteAction(); an empty name defaults to play/pause
// for backward compatibility with bare `kill -USR1`.
type RemoteActionMsg struct{ ActionName string }

// remoteAction maps a subcommand name to the corresponding action constant.
// An empty or unrecognized name defaults to actionPlayPause for backward
// compatibility with bare SIGUSR1.
func remoteAction(name string) action {
	switch name {
	case "toggle-play":
		return actionPlayPause
	case "next":
		return actionNext
	case "previous":
		return actionPrevious
	case "volume-up":
		return actionVolumeUp
	case "volume-down":
		return actionVolumeDown
	default:
		return actionPlayPause
	}
}

// previewStartedMsg is dispatched when the preview player has been launched
// and is ready for playback.
type previewStartedMsg struct {
	player  player.Previewer
	overlay PreviewOverlay
}

// previewStartErrorMsg is dispatched when the preview player fails to start.
type previewStartErrorMsg struct{ err error }

// UndoMsg is dispatched when the user presses 'u'.
type UndoMsg struct{}

// FilterBarMsg is dispatched when the user presses '/' to activate the
// filter bar in the active view.
type FilterBarMsg struct{}

// BatchEnqueuedMsg carries the result of a batch enqueue operation (all tracks
// from a playlist). Root handles it by appending all entries to the queue,
// syncing the player, and setting up the undo slot.
type BatchEnqueuedMsg struct {
	Entries      []QueueEntry
	QueueIDs     []int64
	WasEmpty     bool
	PlaylistName string
	Err          error
}

// DequeueTrackMsg is dispatched when the user presses 'd' in the queue view
// to remove the entry at Index from the playback queue.
type DequeueTrackMsg struct {
	Index int
}

// BulkSelectionToggleMsg is dispatched when the user presses 'x' in the queue
// view to toggle bulk-selection on the entry at Index.
type BulkSelectionToggleMsg struct {
	Index int
}

// ClearQueueMsg is dispatched when the user presses 'C' (Shift+c) in the
// queue view to remove all entries from the playback queue.
type ClearQueueMsg struct{}

// MoveTrackUpMsg is dispatched when the user presses 'K' (Shift+k) in the
// queue view to move the entry at Index one position earlier.
type MoveTrackUpMsg struct {
	Index int
}

// MoveTrackDownMsg is dispatched when the user presses 'J' (Shift+j) in the
// queue view to move the entry at Index one position later.
type MoveTrackDownMsg struct {
	Index int
}

// TransientNotifyMsg carries a short message to display in the status bar as a
// transient notification. Any component can dispatch it to surface a brief hint
// or warning without going through root's internal notification helpers.
type TransientNotifyMsg string

// RepeatMode controls how the queue cycles at the end of play.
type RepeatMode uint8

const (
	RepeatOff RepeatMode = iota // no repeat; stop after the last track
	RepeatAll                   // repeat the entire queue from the beginning
	RepeatOne                   // repeat the current track indefinitely
)

// String returns the session-table value for this RepeatMode. The values match
// the CHECK constraint in the session schema: 'none', 'all', or 'one'.
func (r RepeatMode) String() string {
	switch r {
	case RepeatAll:
		return "all"
	case RepeatOne:
		return "one"
	default:
		return "none"
	}
}

// DisplayString returns a user-friendly label for the repeat mode, suitable
// for status bar notifications (e.g. "off" instead of the DB value "none").
func (r RepeatMode) DisplayString() string {
	switch r {
	case RepeatAll:
		return "all"
	case RepeatOne:
		return "one"
	default:
		return "off"
	}
}

// PlaybackState represents the current playback status shown in the status bar.
type PlaybackState int

const (
	PlaybackIdle    PlaybackState = iota // no track loaded; no indicator shown
	PlaybackPlaying                      // actively playing; shows ▶
	PlaybackPaused                       // paused; shows ⏸
	PlaybackStopped                      // stopped at track start; shows ■
)

// TrackChangedMsg is dispatched when the active track changes (e.g. via
// auto-advance, Next, or Previous). Root handles it by updating the status bar
// track info and transitioning to the playing state.
type TrackChangedMsg struct {
	Artist         string
	Title          string
	PlaylistSource string // "Queue" or a named playlist name; defaults to "Queue" if empty
	Hash           string // content hash of the new track; used for session persistence
}

// favoriteToggledMsg carries the result of a single favorite toggle operation.
type favoriteToggledMsg struct {
	typ   string // "artist", "album", or "track"
	value string
	added bool // true if added, false if removed
	count int  // new total count for the type (for status bar updates)
	err   error

	removedIdx     int             // pre-removal index in unfiltered slice (-1 if N/A)
	removedTrack   library.Track   // the removed Track (for track type)
	removedValues  []string        // all removed values (bulk)
	removedTracks  []library.Track // all removed Track objects (bulk, track type)
	removedIndices []int           // original unfiltered indices (bulk)
	addedValues    []string        // all added values (bulk add, for undo tracking)
	favLevel       favtui.Level
	favCategory    favtui.Category
}

// favoritesLoadedMsg carries the favorites loaded for a specific category.
type favoritesLoadedMsg struct {
	category int // 0=artists, 1=albums, 2=tracks
	artists  []string
	albums   []string
	tracks   []library.Track
	err      error
}

// watchBatchMsg carries tracks discovered by the filesystem watcher and paths
// of files deleted/renamed-away. Unlike ScanBatchMsg, receiving this message
// does not reset the library index: modified tracks are upserted by path and
// deleted paths are evicted.
type watchBatchMsg struct {
	tracks  []library.Track
	deleted []string
}

// smartPlaylistCreatedMsg is dispatched after a successful CreateSmartPlaylist
// call. It carries the new playlist's ID and the refreshed list so root can
// reposition the cursor on the new entry.
type smartPlaylistCreatedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// smartPlaylistEditedMsg is dispatched after a successful
// UpdateSmartPlaylistQuery call. It carries the refreshed playlist list.
type smartPlaylistEditedMsg struct {
	playlists []db.PlaylistRow
}

// smartPlaylistCountsUpdatedMsg carries actual track counts for smart
// playlists, evaluated in the background by evalSmartPlaylistCountsCmd.
// The counts map is keyed by playlist ID.
type smartPlaylistCountsUpdatedMsg struct {
	counts map[int64]int
}

// listeningStatsMsg is dispatched when the background DB read for listening
// statistics completes. err is non-nil when the query failed.
type listeningStatsMsg struct {
	stats db.ListeningStats
	err   error
}
