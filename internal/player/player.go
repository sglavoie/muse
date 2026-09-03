package player

import "errors"

// Sentinel errors returned by Player implementations.
var (
	ErrNotConnected   = errors.New("player: not connected to mpv")
	ErrPlaybackFailed = errors.New("player: playback command failed")
	ErrMissingFile    = errors.New("player: audio file is missing from disk")
	// ErrIPCTransport wraps low-level socket I/O and JSON encoding errors so
	// callers can distinguish transport failures from mpv-reported errors
	// (ErrPlaybackFailed) or connectivity state (ErrNotConnected).
	ErrIPCTransport = errors.New("player: IPC transport error")
)

// MissingMarker persists the missing state for a track and its queue entries.
// db.Writer satisfies this interface.
type MissingMarker interface {
	MarkTrackMissing(hash string) error
	MarkQueueEntryMissing(id int64) error
}

// QueueTrack holds the metadata the Manager needs to load a queue entry into
// mpv and to mark it as missing if the file cannot be found on disk.
type QueueTrack struct {
	Path    string // absolute path to the audio file
	Hash    string // BLAKE3 content hash (primary key of the tracks table)
	QueueID int64  // primary key of the queue table row
	Artist  string // artist display name; used by trackChangedFn on auto-advance
	Title   string // track title; used by trackChangedFn on auto-advance
	Source  string // playlist name this track was enqueued from; "Queue" for individual enqueues
}

// QueueTraversal is an immutable snapshot of the player-owned playback
// timeline: the order the Manager will actually visit queue rows in, where
// playback currently sits in that order, and how much of the cycle is left.
//
// It is a snapshot of Go-owned traversal state, not of the durable queue: the
// visible queue order is unchanged by shuffle, and Order is what maps the two
// together. Every live queue row appears in Order exactly once — rows the
// current cycle no longer schedules (skipped missing entries, entries excluded
// from the cycle) are appended at the end so they stay visible, but they are not
// counted in Remaining.
type QueueTraversal struct {
	// Order holds live queue indices in traversal order. With shuffle on that
	// is played history, the current entry, the forward history a Previous left
	// behind, the unseen pool, and finally the unscheduled rows. With shuffle
	// off it is plain ascending queue order.
	Order []int

	// CurrentPosition is the position within Order of the entry playing now, or
	// -1 when nothing is current (or the current row has left the queue).
	CurrentPosition int

	// Remaining counts the scheduled entries after CurrentPosition. Unscheduled
	// rows parked at the end of Order are excluded. With no current entry every
	// scheduled row counts as remaining.
	Remaining int

	// Shuffled reports whether Order came from shuffle traversal.
	Shuffled bool
}

// Player defines the playback control interface backed by mpv over a Unix
// domain socket IPC connection.
type Player interface {
	// Play resumes or starts playback.
	Play() error

	// Pause pauses playback.
	Pause() error

	// Stop pauses playback and seeks to the beginning of the current track.
	Stop() error

	// SeekRelative seeks forward (positive) or backward (negative) by the
	// given number of seconds.
	SeekRelative(seconds float64) error

	// SetVolumeRelative adjusts volume by the given percentage delta
	// (e.g. +5 or -5).
	SetVolumeRelative(delta float64) error

	// Next advances to the next track in the playlist.
	Next() error

	// Previous returns to the previous track in the playlist.
	Previous() error

	// PlayFromIndex starts playback from the queue entry at the given index.
	PlayFromIndex(index int) error
}
