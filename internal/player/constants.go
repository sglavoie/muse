package player

import (
	"errors"
	"time"
)

// errConnectTimeout is returned by connectSocket when the mpv IPC socket does
// not accept a connection before the poll timeout elapses. The restart
// supervisor treats it as a failed attempt (same path as a launch failure).
var errConnectTimeout = errors.New("player: mpv socket connection timed out")

// Timing constants for the main mpv manager.
//
// socketPollTimeout must exceed defaultPingInterval — it bounds the initial
// connection wait after mpv starts.
const (
	defaultPingInterval = 200 * time.Millisecond
	shutdownTimeout     = 3 * time.Second
	socketPollInterval  = 50 * time.Millisecond
	socketPollTimeout   = 10 * time.Second
)

// Timing constants for the preview player.
const (
	previewPollInterval    = 50 * time.Millisecond
	previewPollTimeout     = 5 * time.Second
	previewShutdownTimeout = 3 * time.Second
)

// soundPlayTimeout is the maximum time allowed for a single sound playback.
const soundPlayTimeout = 5 * time.Second

// Restart backoff constants for watchAndRestart.
//
// restartMaxDelay caps the exponential growth so the loop never waits more
// than 30 s between attempts.
const (
	restartInitialDelay = 500 * time.Millisecond
	restartMaxDelay     = 30 * time.Second
	restartMaxAttempts  = 10
)

// mpv IPC command names.
const (
	cmdAdd             = "add"
	cmdCycle           = "cycle"
	cmdGetProperty     = "get_property"
	cmdLoadfile        = "loadfile"
	cmdObserveProperty = "observe_property"
	cmdPlaylistRemove  = "playlist-remove"
	cmdSeek            = "seek"
	cmdSetProperty     = "set_property"
)

// mpv property names used with get_property / set_property / observe_property.
const (
	propAF               = "af"
	propDuration         = "duration"
	propGaplessAudio     = "gapless-audio"
	propLoopFile         = "loop-file"
	propLoopPlaylist     = "loop-playlist"
	propPath             = "path"
	propPause            = "pause"
	propPrefetchPlaylist = "prefetch-playlist"
	propTimePos          = "time-pos"
	propVolume           = "volume"
)

// ipcReadTimeout lives in ipc.go as a var to allow test overrides.
