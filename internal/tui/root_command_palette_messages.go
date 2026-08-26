package tui

// commandPaletteActionMsg carries an action to replay through handleKeyAction
// after the palette has been dismissed. A non-zero view switches to that view
// first, so a view-local action reaches its own handler; 0 runs the action in
// place.
type commandPaletteActionMsg struct {
	action action
	view   viewID // 0 = run in place
}

// navigateLibraryModeMsg requests deterministic navigation to one of the
// Library view's modes. It is separate from the mode-cycle actions because a
// palette command must land on the requested mode regardless of current state.
type navigateLibraryModeMsg struct{ mode LibraryMode }

// navigateBrowseModeMsg requests deterministic navigation to one of the
// Browse view's modes.
type navigateBrowseModeMsg struct{ mode BrowseMode }

// configToggleField identifies which audio setting to toggle.
type configToggleField int

const (
	toggleNormalize configToggleField = iota
	toggleEQ
	toggleStereoWide
)

// configToggleMsg requests that an audio setting be toggled in the config.
type configToggleMsg struct{ field configToggleField }

// randomAlbumMsg switches to Albums view and positions the cursor on a
// randomly chosen album.
type randomAlbumMsg struct{}

// randomArtistMsg switches to Artists view and positions the cursor on a
// randomly chosen artist.
type randomArtistMsg struct{}

// queueShuffleMsg requests an in-place Fisher-Yates shuffle of the queue.
type queueShuffleMsg struct{}

// queueDedupMsg requests removal of duplicate queue entries (keep first
// occurrence of each track hash).
type queueDedupMsg struct{}

// playlistShuffleMsg requests an in-place Fisher-Yates shuffle of the
// focused playlist's tracks.
type playlistShuffleMsg struct{}

// jumpToNowPlayingMsg switches to Library view with the cursor on the
// currently-playing track.
type jumpToNowPlayingMsg struct{}

// toggleFooterHintsMsg flips the runtime state of the context-aware footer
// hint strip and persists the new value to the config.
type toggleFooterHintsMsg struct{}

// toggleTrackNotifyMsg flips the runtime state of the on-track-change macOS
// banner and persists the new value to the config.
type toggleTrackNotifyMsg struct{}

// toggleQueueFollowMsg flips the runtime state of Queue auto-scroll
// (follow now-playing) and persists the new value to the config.
type toggleQueueFollowMsg struct{}
