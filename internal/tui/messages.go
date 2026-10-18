package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// undoKind distinguishes which type of destructive action is reversible.
type undoKind uint8

const (
	undoNone                         undoKind = iota
	undoKindEnqueue                           // single track added via TrackEnqueuedMsg
	undoKindDequeue                           // single track removed via DequeueTrackMsg
	undoKindReplaceQueue                      // queue replaced transactionally
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
	undoKindPaste                             // tracks pasted into queue via ctrl+p
	undoKindCutPaste                          // tracks moved via cut (ctrl+x) then paste
	undoKindTagToggle                         // tag assigned to / removed from track(s)
	undoKindDeleteTag                         // tag soft-deleted from Tags view
	undoKindImportMetadata                    // metadata applied from a TSV import
)

// undoSlot holds the reversible state for the most recent destructive action.
// Only one slot is maintained; a new action replaces the prior one.
// action is nil when no undo is available.
type undoSlot struct {
	action    undoAction
	expiresAt time.Time // notification lifetime only; the action does not expire
	cleanup   func()    // hard-delete tombstones once they are no longer restorable
}

// tickMsg is dispatched every second by tickCmd. It drives elapsed-time
// display updates and transient-notification expiry checks.
type tickMsg struct{}

// pulseTickMsg is dispatched 250 ms after a TrackChangedMsg so the View()
// rebuilds once the pulseUntil window has elapsed and the now-playing row
// returns to its normal styling.
type pulseTickMsg struct{}

// notificationFadeMsg is dispatched at expiry-minus-250ms for the soonest
// pending status-bar notification so the View() rebuilds once the last frame
// should render dim. Receipt is a no-op — the render-time check on remaining
// time produces the dimmer style.
type notificationFadeMsg struct{}

// sessionTickMsg is dispatched every sessionTickInterval by sessionTickCmd.
// On each tick the current playback position is persisted to the session table,
// but only when playback is active (PlaybackPlaying).
type sessionTickMsg struct{}

// syncPositionMsg is dispatched every positionSyncInterval to trigger a
// playback-position update on the player.
type syncPositionMsg struct{}

// previewStartedMsg is dispatched when the preview player has been launched
// and is ready for playback.
type previewStartedMsg struct {
	player  player.Previewer
	overlay overlay.PreviewOverlay
	request uint64
}

// previewStartErrorMsg is dispatched when the preview player fails to start.
type previewStartErrorMsg struct {
	err     error
	request uint64
}

// previewPauseResultMsg carries the ordered main-player pause that must finish
// before a preview player is allowed to start.
type previewPauseResultMsg struct {
	request uint64
	track   library.Track
	err     error
	ran     bool
}

// UndoMsg is dispatched when the user presses 'u'.
type UndoMsg struct{}

// FilterBarMsg is dispatched when the user presses '/' to activate the
// filter bar in the active view.
type FilterBarMsg struct{}
