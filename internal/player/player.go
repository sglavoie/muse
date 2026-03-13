package player

import "errors"

// Sentinel errors returned by Player implementations.
var (
	ErrNotConnected  = errors.New("player: not connected to mpv")
	ErrPlaybackFailed = errors.New("player: playback command failed")
)

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
