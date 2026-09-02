package playlists

// FallbackVisibleRows is the assumed number of visible rows when
// the terminal height is unknown (zero). Used by EnsureCursorVisible to
// keep the cursor within the viewport before the first WindowSizeMsg.
const FallbackVisibleRows = 50
