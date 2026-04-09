package tui

// commandPaletteActionMsg carries an action to replay through handleKeyAction
// after the palette has been dismissed.
type commandPaletteActionMsg struct {
	action action
}

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
