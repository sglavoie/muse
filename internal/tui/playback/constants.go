package playback

import "time"

// PositionSyncInterval is how often root polls the player for the current
// playback position so that crash-resume can restore to the right offset.
const PositionSyncInterval = 5 * time.Second

// SessionTickInterval is how often the periodic session ticker fires to persist
// the current elapsed position to the session table during uninterrupted playback.
const SessionTickInterval = 10 * time.Second
