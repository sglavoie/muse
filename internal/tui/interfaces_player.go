package tui

import (
	"context"

	"github.com/sglavoie/muse/internal/player"
)

// playPauser is the subset of the player interface used by the root model for
// global play/pause toggling and queue-index playback. Keeping it narrow makes
// test injection simple.
type playPauser interface {
	Play() error
	Pause() error
	PlayFromIndex(int) error
}

// commandPlayer is the optional extended interface for full playback control.
// Root checks for it via type assertion so that test stubs only need to
// implement playPauser.
type commandPlayer interface {
	Stop() error
	SeekAbsolute(seconds float64) error
	SeekRelative(seconds float64) error
	SetVolumeRelative(delta float64) error
	Next() error
	Previous() error
}

// playerQueueAppender is an optional interface for appending a single entry to
// the player's in-memory queue without replacing the whole queue. Root checks
// for it via type assertion after a track is enqueued from the library view.
type playerQueueAppender interface {
	AppendToQueue(player.QueueTrack)
}

// playerQueueSetter is an optional interface for replacing the player's entire
// in-memory queue. Root checks for it via type assertion on ClearQueueMsg (to
// clear the player queue) and UndoMsg for undoKindClear (to restore it), keeping
// the player and TUI queue lists in sync after destructive clear operations.
type playerQueueSetter interface {
	SetQueue([]player.QueueTrack)
}

// playerQueueRemover is an optional interface for removing a single entry from
// the player's in-memory queue by index. Root checks for it via type assertion
// in the DequeueTrackMsg handler to keep the player queue in sync with the TUI
// queue after a track is dequeued.
type playerQueueRemover interface {
	RemoveFromQueue(index int) error
}

// playerIndexSetter is an optional interface for updating the player's current
// queue index after the TUI reorders or deduplicates the queue. Without this,
// the player's auto-advance uses a stale index causing skipped or repeated
// tracks after shuffle, dedup, move, or paste operations.
type playerIndexSetter interface {
	SetCurrentIndex(index int)
}

// positionSyncer is implemented by player.Manager. Root uses it to
// periodically persist the current playback position for crash-resume.
type positionSyncer interface {
	SyncPosition() error
}

// repeatShuffler is an optional extension checked via type assertion. Root
// calls SetRepeat and SetShuffle after cycling the model modes so that mpv is
// instructed to change its repeat/shuffle mode and the player state stays in
// sync with what the status bar displays. SetRepeat receives the string form
// of RepeatMode ("none", "all", or "one") to avoid a circular import with the
// player package.
type repeatShuffler interface {
	SetRepeat(mode string) error
	SetShuffle(bool) error
}

// queueCycleRestarter is an optional interface exposing RestartQueueCycle for
// repeat-all queue exhaustion. When shuffle is on it builds a fresh permutation
// (avoiding an immediate replay of the just-played track); when off it restarts
// sequentially from index zero. Root checks for it via type assertion so that
// lightweight test players implementing only playPauser fall back to
// PlayFromIndex(0).
type queueCycleRestarter interface {
	RestartQueueCycle() error
}

// queueTraversalReader is an optional interface exposing the player-owned
// playback timeline. Root checks for it via type assertion wherever a decision
// depends on how much of the queue is still ahead: the visible queue index says
// nothing about that under shuffle, where traversal order is Go-owned and
// unrelated to the durable row order. Lightweight test players implementing only
// playPauser fall back to visible-index arithmetic.
type queueTraversalReader interface {
	QueueTraversal() player.QueueTraversal
}

// currentTrackReplayer is an optional interface exposing ReplayCurrent, which
// restarts the entry already playing without moving the playback timeline. Root
// checks for it via type assertion when the user selects the row that is already
// current: PlayFromIndex would answer that keypress by reanchoring the shuffle
// cycle, discarding the played history and re-permuting everything still
// upcoming, which is a visible reshuffle of the Queue panel the user never asked
// for. Lightweight test players implementing only playPauser fall back to
// PlayFromIndex, whose behaviour on a non-shuffling player is identical.
type currentTrackReplayer interface {
	ReplayCurrent() error
}

// queueExtensionResumer is an optional interface exposing
// ResumeAfterQueueExtension, which continues the traversal already in progress
// after the queue grew and reports the queue index that actually started. Root
// checks for it via type assertion on every natural-exhaustion resume so that a
// completed shuffle cycle is not reanchored — PlayFromIndex, the fallback for
// players lacking the extension, would start a fresh permutation and replay rows
// the cycle already visited.
type queueExtensionResumer interface {
	ResumeAfterQueueExtension() (int, error)
}

// stopAfterToggler enables or disables "stop after current track" mode in
// the player. Root checks for it via type assertion so that test stubs only
// need to implement playPauser.
type stopAfterToggler interface {
	SetStopAfterCurrent(bool)
	StopAfterCurrent() bool
}

// elapsedReader is implemented by player.Manager to provide the current
// elapsed playback position and total track duration. Root queries it on
// each 1-second tick to keep the status bar time display current.
type elapsedReader interface {
	ElapsedPosition() (elapsed, total float64, err error)
}

// queueRefreshCommitter is the optional player interface that runs a refresh
// through the guarded four-stage sequence: settle, capture, the caller's durable
// commit, publication, and mpv reconciliation. Root checks for it via type
// assertion on the player field, so the lightweight test players that implement
// only playPauser leave Apply on the refill path rather than the refresh path.
//
// It blocks — on a settle wait, a database transaction, and mpv IPC — so every
// call is made from a tea.Cmd goroutine behind the play-request gate, never on
// the Bubble Tea update path.
type queueRefreshCommitter interface {
	CommitQueueRefresh(ctx context.Context, rc player.RefreshCommit) (player.RefreshCommitResult, error)
}

// audioNormalizer enables or disables real-time EBU R128 volume normalization
// via the player's audio filter chain. Root checks for it via type assertion
// on settings save and is not required by test stubs.
type audioNormalizer interface {
	SetNormalize(enabled bool) error
}

// gaplessToggler enables or disables gapless playback via the player.
// Root checks for it via type assertion on settings save.
type gaplessToggler interface {
	SetGapless(enabled bool) error
}

// equalizerSetter enables or disables the 10-band parametric equalizer via the
// player. Root checks for it via type assertion on settings save and live
// preview.
type equalizerSetter interface {
	SetEqualizer(enabled bool, bands [10]int) error
}

// stereoWideSetter enables or disables stereo widening via the player.
// Root checks for it via type assertion on settings save and live preview.
type stereoWideSetter interface {
	SetStereoWide(enabled bool, amount int) error
}

// debugInfoProvider is implemented by player.Manager to expose diagnostic
// properties for the debug panel. Root checks for it via type assertion so that
// test stubs do not need to implement it.
type debugInfoProvider interface {
	DebugInfo() (map[string]string, error)
}
