package player

import "errors"

// Sentinel errors returned by Player implementations.
var (
	ErrNotConnected   = errors.New("player: not connected to mpv")
	ErrPlaybackFailed = errors.New("player: playback command failed")
	ErrMissingFile    = errors.New("player: audio file is missing from disk")
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
}
