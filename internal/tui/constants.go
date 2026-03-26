package tui

import "time"

// undoWindowDuration is the time after a destructive queue action during which
// pressing 'u' will reverse the action. Only a single undo slot is maintained;
// a new destructive action within the window replaces the prior slot.
const undoWindowDuration = 5 * time.Second

// positionSyncInterval is how often root polls the player for the current
// playback position so that crash-resume can restore to the right offset.
const positionSyncInterval = 5 * time.Second

// sessionTickInterval is how often the periodic session ticker fires to persist
// the current elapsed position to the session table during uninterrupted playback.
const sessionTickInterval = 10 * time.Second

// confirmationWindow is the duration of the two-press confirmation gate for
// destructive library actions. A second press within this window executes the
// action; after expiry the gate resets.
const confirmationWindow = 2 * time.Second

// queueConfirmationWindow is the duration of the two-press confirmation
// gate for destructive queue actions (e.g. clear queue). A second press
// within this window executes the action; after expiry the gate resets.
const queueConfirmationWindow = 2 * time.Second

// scanBatchSize is the maximum number of tracks collected per batch before
// returning a ScanBatchMsg to the update loop.
const scanBatchSize = 100

// transientDuration is how long a transient notification stays visible before
// the 1-second tick clears it.
const transientDuration = 5 * time.Second

// playlistFallbackVisibleRows is the assumed number of visible rows when
// the terminal height is unknown (zero). Used by ensureCursorVisible to
// keep the cursor within the viewport before the first WindowSizeMsg.
const playlistFallbackVisibleRows = 50
