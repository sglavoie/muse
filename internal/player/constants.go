package player

import "time"

// Timing constants for the main mpv manager.
//
// socketPollTimeout must exceed defaultPingInterval — it bounds the initial
// connection wait after mpv starts.
const (
	defaultPingInterval    = 200 * time.Millisecond
	shutdownTimeout        = 3 * time.Second
	socketPollInterval     = 50 * time.Millisecond
	socketPollTimeout      = 10 * time.Second
	staleSocketDialTimeout = 200 * time.Millisecond
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

// ipcReadTimeout lives in ipc.go as a var to allow test overrides.
