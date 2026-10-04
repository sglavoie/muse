package statusbar

import (
	"time"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// StatusBar manages the persistent status bar at the bottom of the screen.
type StatusBar struct {
	ConfigCreated bool
	ConfigPath    string
	WatchDegraded bool
	Notifications []TransientNotification
	history       []Message
	TransientDur  time.Duration // configurable notification duration

	// UndoExpiresAt identifies the active undo prompt; zero hides past prompts.
	UndoExpiresAt time.Time

	// Persistent left zone fields — set during Update when track/state changes.
	Artist         string
	Title          string
	Album          string
	PlaylistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	PlaybackState    playback.State
	RepeatMode       playback.RepeatMode
	Shuffle          bool
	StopAfterCurrent bool
	VisualMode       bool
	QueueEmpty       bool

	// Elapsed and total are pre-formatted MM:SS strings injected by
	// renderStatusBar. Both must be non-empty to show the time display.
	Elapsed string
	Total   string

	// ElapsedSecs and totalSecs are the raw float64 values injected by
	// renderStatusBar for use in renderProgressBar.
	ElapsedSecs float64
	TotalSecs   float64

	// RadioMode is true when Auto-DJ / radio mode is active.
	RadioMode bool

	// QueuePosition is the formatted "N of M" queue position string injected
	// by renderStatusBar. Empty when nothing is playing.
	QueuePosition string

	// BucketLabel is an optional chunk label for the active view's focused row.
	BucketLabel string

	// YankCount, yankIsCut, and selectionCount are injected by renderStatusBar.
	YankCount      int
	YankIsCut      bool
	SelectionCount int

	// LongOp is the current long-running operation snapshot injected by
	// renderStatusBar.
	LongOp LongOpState
}
