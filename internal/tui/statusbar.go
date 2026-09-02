package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// StatusBar manages the persistent status bar at the bottom of the screen.
type StatusBar struct {
	configCreated bool
	configPath    string
	watchDegraded bool
	notifications []transientNotification
	transientDur  time.Duration // configurable notification duration

	// undoExpiresAt is set by renderStatusBar when an undo slot is active.
	undoExpiresAt time.Time

	// Persistent left zone fields — set during Update when track/state changes.
	artist         string
	title          string
	album          string
	playlistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	playbackState    playback.State
	repeatMode       playback.RepeatMode
	shuffle          bool
	stopAfterCurrent bool
	visualMode       bool
	queueEmpty       bool

	// elapsed and total are pre-formatted MM:SS strings injected by
	// renderStatusBar. Both must be non-empty to show the time display.
	elapsed string
	total   string

	// elapsedSecs and totalSecs are the raw float64 values injected by
	// renderStatusBar for use in renderProgressBar.
	elapsedSecs float64
	totalSecs   float64

	// radioMode is true when Auto-DJ / radio mode is active.
	radioMode bool

	// queuePosition is the formatted "N of M" queue position string injected
	// by renderStatusBar. Empty when nothing is playing.
	queuePosition string

	// bucketLabel is an optional chunk label for the active view's focused row.
	bucketLabel string

	// yankCount, yankIsCut, and selectionCount are injected by renderStatusBar.
	yankCount      int
	yankIsCut      bool
	selectionCount int

	// longOp is the current long-running operation snapshot injected by
	// renderStatusBar.
	longOp longOpState
}
