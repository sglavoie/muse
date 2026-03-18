package tui

// bindingContext identifies the context in which key bindings are resolved.
// The registry checks the view-local context first, then falls back to Global.
type bindingContext int

const (
	contextGlobal    bindingContext = iota // applies in every view
	contextLibrary                         // applies only in the Library view
	contextPlaylists                       // applies only in the Playlists view
	contextQueue                           // applies only in the Queue view
	contextArtists                         // applies only in the Artists view
	contextAlbums                          // applies only in the Albums view
	contextModals                          // applies only when a modal is active
)

// action represents a named TUI action that a key binding triggers.
type action int

const (
	actionNone action = iota

	// Global actions — available in every view unless a view-local binding
	// shadows the same key.
	actionQuit
	actionPlayPause
	actionStop
	actionNext
	actionPrevious
	actionSeekBack
	actionSeekForward
	actionSeekBackLong
	actionSeekForwardLong
	actionVolumeUp
	actionVolumeDown
	actionSwitchLibrary
	actionSwitchPlaylists
	actionSwitchQueue
	actionRescan
	actionRepeatCycle   // 'r': cycle repeat mode off→all→one→off
	actionShuffleToggle // 'z': toggle shuffle mode
	actionUndo          // 'u': dispatch undo
	actionFilterBar     // '/': activate filter bar in active view
	actionHelp          // '?': show help overlay
	actionNextView      // tab: cycle to next view
	actionPreview       // 'P': preview focused track

	actionSwitchArtists
	actionSwitchAlbums

	// Library-local actions.
	actionLibraryEnqueue     // enter: append the focused track to the play queue
	actionLibraryFilterClear // esc: clear a committed filter in the library view

	// Queue-local actions.
	actionCursorUp
	actionCursorDown
	actionSelect
	actionDequeue             // 'd': remove the selected track from the queue
	actionBulkSelectionToggle // 'x': toggle bulk-selection on the selected entry
	actionClearQueue          // 'C': clear all entries from the queue
	actionMoveTrackUp         // 'K': move selected track one position earlier
	actionMoveTrackDown       // 'J': move selected track one position later
	actionSaveQueueAsPlaylist // 'S': save entire queue as a new playlist

	// Playlists-local actions.
	actionPlaylistRename
	actionPlaylistCreate // 'c': create new playlist
	actionPlaylistOpen   // 'enter': open playlist to display its tracks
	actionPlaylistBack   // 'esc': return to the playlist list from track level
	actionPlaylistDelete // 'd': delete focused playlist (list level) or remove focused track (track level)

	actionPlaylistEnqueueAll // 'E': enqueue all tracks from the focused playlist

	// Artists-local actions.
	actionArtistOpen // enter: drill into albums/tracks or enqueue
	actionArtistBack // esc: go back one level

	// Albums-local actions.
	actionAlbumOpen // enter: drill into tracks or enqueue
	actionAlbumBack // esc: go back one level

	// Bulk-selection action — triggered by 'a' when tracks are selected.
	// actionBulkAction applies in Library (append selected tracks to queue).
	// actionBulkRemovePlaylistTracks applies in Playlists (remove selected tracks from a playlist).
	actionBulkAction               // 'a': apply context-appropriate bulk action and clear selection
	actionBulkRemovePlaylistTracks // 'a': remove selected track(s) from a playlist (Playlists context)

	// Vim-style scrolling actions — shared across Library, Queue, and Playlists.
	actionHalfPageDown // ctrl+d: move cursor half a page down
	actionHalfPageUp   // ctrl+u: move cursor half a page up
	actionFullPageDown // ctrl+f: move cursor a full page down
	actionFullPageUp   // ctrl+b: move cursor a full page up
	actionGoToTop      // g: move cursor to the first row
	actionGoToBottom   // G: move cursor to the last row
)

// registryKey is the composite map key for registry lookups.
type registryKey struct {
	ctx bindingContext
	key string
}

// Registry maps (context, key) → action. The zero value is not usable;
// use newRegistry to obtain a pre-populated instance.
type Registry struct {
	bindings map[registryKey]action
}

// newRegistry returns a Registry pre-populated with the application's default
// key bindings.
func newRegistry() Registry {
	r := Registry{bindings: make(map[registryKey]action)}

	// Global bindings.
	r.register(contextGlobal, "ctrl+c", actionQuit)
	r.register(contextGlobal, "space", actionPlayPause)
	r.register(contextGlobal, "s", actionStop)
	r.register(contextGlobal, "n", actionNext)
	r.register(contextGlobal, "p", actionPrevious)
	r.register(contextGlobal, "left", actionSeekBack)
	r.register(contextGlobal, "right", actionSeekForward)
	r.register(contextGlobal, "shift+left", actionSeekBackLong)
	r.register(contextGlobal, "shift+right", actionSeekForwardLong)
	r.register(contextGlobal, "=", actionVolumeUp)
	r.register(contextGlobal, "-", actionVolumeDown)
	r.register(contextGlobal, "1", actionSwitchLibrary)
	r.register(contextGlobal, "2", actionSwitchPlaylists)
	r.register(contextGlobal, "3", actionSwitchQueue)
	r.register(contextGlobal, "4", actionSwitchArtists)
	r.register(contextGlobal, "5", actionSwitchAlbums)
	r.register(contextGlobal, "R", actionRescan)
	r.register(contextGlobal, "r", actionRepeatCycle)
	r.register(contextGlobal, "z", actionShuffleToggle)
	r.register(contextGlobal, "u", actionUndo)
	r.register(contextGlobal, "/", actionFilterBar)
	r.register(contextGlobal, "?", actionHelp)
	r.register(contextGlobal, "tab", actionNextView)
	r.register(contextGlobal, "P", actionPreview)

	// Library-local bindings.
	r.register(contextLibrary, "j", actionCursorDown)
	r.register(contextLibrary, "k", actionCursorUp)
	r.register(contextLibrary, "up", actionCursorUp)
	r.register(contextLibrary, "down", actionCursorDown)
	r.register(contextLibrary, "enter", actionLibraryEnqueue)
	r.register(contextLibrary, "esc", actionLibraryFilterClear)
	r.register(contextLibrary, "x", actionBulkSelectionToggle)
	r.register(contextLibrary, "a", actionBulkAction)
	r.register(contextLibrary, "ctrl+d", actionHalfPageDown)
	r.register(contextLibrary, "ctrl+u", actionHalfPageUp)
	r.register(contextLibrary, "ctrl+f", actionFullPageDown)
	r.register(contextLibrary, "ctrl+b", actionFullPageUp)
	r.register(contextLibrary, "g", actionGoToTop)
	r.register(contextLibrary, "G", actionGoToBottom)

	// Queue-local bindings.
	r.register(contextQueue, "j", actionCursorDown)
	r.register(contextQueue, "k", actionCursorUp)
	r.register(contextQueue, "up", actionCursorUp)
	r.register(contextQueue, "down", actionCursorDown)
	r.register(contextQueue, "enter", actionSelect)
	r.register(contextQueue, "d", actionDequeue)
	r.register(contextQueue, "x", actionBulkSelectionToggle)
	r.register(contextQueue, "C", actionClearQueue)
	r.register(contextQueue, "K", actionMoveTrackUp)
	r.register(contextQueue, "J", actionMoveTrackDown)
	r.register(contextQueue, "ctrl+d", actionHalfPageDown)
	r.register(contextQueue, "ctrl+u", actionHalfPageUp)
	r.register(contextQueue, "ctrl+f", actionFullPageDown)
	r.register(contextQueue, "ctrl+b", actionFullPageUp)
	r.register(contextQueue, "g", actionGoToTop)
	r.register(contextQueue, "G", actionGoToBottom)
	r.register(contextQueue, "S", actionSaveQueueAsPlaylist)

	// Playlists-local bindings.
	// 'r' is bound to rename here, shadowing the global repeat-cycle binding
	// while the Playlists view is focused.
	r.register(contextPlaylists, "j", actionCursorDown)
	r.register(contextPlaylists, "k", actionCursorUp)
	r.register(contextPlaylists, "up", actionCursorUp)
	r.register(contextPlaylists, "down", actionCursorDown)
	r.register(contextPlaylists, "r", actionPlaylistRename)
	r.register(contextPlaylists, "c", actionPlaylistCreate)
	r.register(contextPlaylists, "enter", actionPlaylistOpen)
	r.register(contextPlaylists, "esc", actionPlaylistBack)
	r.register(contextPlaylists, "d", actionPlaylistDelete)
	r.register(contextPlaylists, "x", actionBulkSelectionToggle)
	r.register(contextPlaylists, "a", actionBulkRemovePlaylistTracks)
	r.register(contextPlaylists, "E", actionPlaylistEnqueueAll)
	r.register(contextPlaylists, "ctrl+d", actionHalfPageDown)
	r.register(contextPlaylists, "ctrl+u", actionHalfPageUp)
	r.register(contextPlaylists, "ctrl+f", actionFullPageDown)
	r.register(contextPlaylists, "ctrl+b", actionFullPageUp)
	r.register(contextPlaylists, "g", actionGoToTop)
	r.register(contextPlaylists, "G", actionGoToBottom)

	// Artists-local bindings.
	r.register(contextArtists, "j", actionCursorDown)
	r.register(contextArtists, "k", actionCursorUp)
	r.register(contextArtists, "up", actionCursorUp)
	r.register(contextArtists, "down", actionCursorDown)
	r.register(contextArtists, "enter", actionArtistOpen)
	r.register(contextArtists, "esc", actionArtistBack)
	r.register(contextArtists, "ctrl+d", actionHalfPageDown)
	r.register(contextArtists, "ctrl+u", actionHalfPageUp)
	r.register(contextArtists, "ctrl+f", actionFullPageDown)
	r.register(contextArtists, "ctrl+b", actionFullPageUp)
	r.register(contextArtists, "g", actionGoToTop)
	r.register(contextArtists, "G", actionGoToBottom)

	// Albums-local bindings.
	r.register(contextAlbums, "j", actionCursorDown)
	r.register(contextAlbums, "k", actionCursorUp)
	r.register(contextAlbums, "up", actionCursorUp)
	r.register(contextAlbums, "down", actionCursorDown)
	r.register(contextAlbums, "enter", actionAlbumOpen)
	r.register(contextAlbums, "esc", actionAlbumBack)
	r.register(contextAlbums, "ctrl+d", actionHalfPageDown)
	r.register(contextAlbums, "ctrl+u", actionHalfPageUp)
	r.register(contextAlbums, "ctrl+f", actionFullPageDown)
	r.register(contextAlbums, "ctrl+b", actionFullPageUp)
	r.register(contextAlbums, "g", actionGoToTop)
	r.register(contextAlbums, "G", actionGoToBottom)

	return r
}

// register adds a (context, key) → action entry to the registry.
func (r *Registry) register(ctx bindingContext, key string, a action) {
	r.bindings[registryKey{ctx: ctx, key: key}] = a
}

// lookup returns the action bound to (ctx, key), or actionNone if not found.
func (r Registry) lookup(ctx bindingContext, key string) action {
	return r.bindings[registryKey{ctx: ctx, key: key}]
}

// resolve returns the effective action for key in the given view context.
// The view-local context is checked first; if no binding is found there the
// global context is consulted. This gives view-local bindings precedence over
// global ones for the same key.
func (r Registry) resolve(viewCtx bindingContext, key string) action {
	if a := r.lookup(viewCtx, key); a != actionNone {
		return a
	}
	return r.lookup(contextGlobal, key)
}
