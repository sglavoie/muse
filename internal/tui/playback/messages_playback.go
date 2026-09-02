package playback

import "github.com/sglavoie/muse/internal/tui/keys"

// PlayRecordedMsg is dispatched after a successful RecordListen call so the
// History view can be updated in realtime without a full DB reload.
type PlayRecordedMsg struct {
	Hash     string
	PlayedAt int64
	Counted  bool
}

// MissingTrackMsg is dispatched by the player when it attempts to play a
// queued track whose file cannot be found. The TUI marks that entry as missing
// in the queue view and displays a transient error in the status bar.
type MissingTrackMsg struct {
	Path string
}

// PlayerStatusMsg carries a transient status string from the player backend
// into the TUI status bar.
type PlayerStatusMsg struct {
	Text string
}

// PlayQueueIndexMsg is dispatched by the queue view when the user presses
// Enter on a track. Root handles it by starting playback from that index.
type PlayQueueIndexMsg struct {
	Index int
}

// MissingTrackSelectedMsg is dispatched by the queue view when the user
// presses Enter on a missing track.
type MissingTrackSelectedMsg struct {
	Index int
	Path  string
	Hash  string
}

// PlayFromIndexResultMsg carries the outcome of an asynchronous
// player.PlayFromIndex call dispatched by handlePlayQueueIndexMsg.
type PlayFromIndexResultMsg struct {
	Index int
	Path  string
	Hash  string
	Err   error
}

// PlayerDisconnectedMsg is dispatched when the mpv process crashes or its IPC
// socket disconnects unexpectedly.
type PlayerDisconnectedMsg struct{}

// PlaybackEndedMsg is dispatched by the player when auto-advance exhausts the
// queue.
type PlaybackEndedMsg struct{}

// PauseChangedMsg is dispatched by the player when mpv's pause property
// changes externally.
type PauseChangedMsg struct {
	Paused bool
}

// RemoteActionMsg is sent when an external signal (SIGUSR1) requests a
// playback action.
type RemoteActionMsg struct{ ActionName string }

// RemoteAction maps a subcommand name to the corresponding action constant.
// Unknown names return actionNone so handlePlaybackKeyAction treats them as a
// no-op rather than silently performing play/pause (which would mask typos and
// future-protocol drift between the remote client and the running instance).
func RemoteAction(name string) keys.Action {
	switch name {
	case "toggle-play":
		return keys.ActionPlayPause
	case "next":
		return keys.ActionNext
	case "previous":
		return keys.ActionPrevious
	case "volume-up":
		return keys.ActionVolumeUp
	case "volume-down":
		return keys.ActionVolumeDown
	default:
		return keys.ActionNone
	}
}

// RepeatMode controls how the queue cycles at the end of play.
type RepeatMode uint8

const (
	RepeatOff RepeatMode = iota // no repeat; stop after the last track
	RepeatAll                   // repeat the entire queue from the beginning
	RepeatOne                   // repeat the current track indefinitely
)

// String returns the session-table value for this RepeatMode.
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

// DisplayString returns a user-friendly label for the repeat mode.
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

// State represents the current playback status shown in the status bar.
type State int

const (
	StateIdle    State = iota // no track loaded; no indicator shown
	StatePlaying              // actively playing; shows ▶
	StatePaused               // paused; shows ⏸
	StateStopped              // stopped at track start; shows ■
)

// TrackChangedMsg is dispatched when the active track changes.
type TrackChangedMsg struct {
	Artist         string
	Title          string
	PlaylistSource string // "Queue" or a named playlist name; defaults to "Queue" if empty
	Hash           string // content hash of the new track; used for session persistence
	Index          int    // queue index of the new track; used for now-playing highlight
}
