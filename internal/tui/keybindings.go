package tui

// bindingContext identifies the context in which key bindings are resolved.
// The registry checks the view-local context first, then falls back to Global.
type bindingContext int

const (
	contextGlobal        bindingContext = iota // applies in every view
	contextLibrary                             // applies only in the Library view
	contextRecentlyAdded                       // applies only in the Recently Added view
	contextPlaylists                           // applies only in the Playlists view
	contextQueue                               // applies only in the Queue view
	contextArtists                             // applies only in the Artists view
	contextAlbums                              // applies only in the Albums view
	contextFavorites                           // applies only in the Favorites view
	contextGenres                              // applies only in the Genres view
	contextModals                              // applies only when a modal is active
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
	actionRepeatCycle    // 'r': cycle repeat mode off→all→one→off
	actionShuffleToggle  // 'z': toggle shuffle mode
	actionUndo           // 'u': dispatch undo
	actionFilterBar      // '/': activate filter bar in active view
	actionHelp           // '?': show help overlay
	actionSettings       // 'ctrl+,': open settings overlay
	actionDebug          // 'ctrl+g': show debug panel
	actionStats          // 'L': show listening stats overlay
	actionNextView       // tab: cycle to next view
	actionPrevView       // shift+tab: cycle to previous view
	actionPreview        // 'P': preview focused track
	actionNowPlayingInfo // 'I': show now-playing info overlay
	actionEditMetadata   // 'M': edit metadata of focused track
	actionTrackInfo      // 'i': show track info overlay (exiftool)

	actionSwitchArtists
	actionSwitchAlbums
	actionSwitchFavorites
	actionSwitchRecentlyAdded
	actionToggleFavorite
	actionGoTo // 'o': open Go To navigation popup

	// Favorites-local actions.
	actionFavoritesOpen // enter: drill into category/list or enqueue track
	actionFavoritesBack // esc: go back one level

	// Library-local actions.
	actionLibraryEnqueue        // enter: append the focused track to the play queue
	actionLibraryEnqueueReplace // shift+enter: clear queue and enqueue focused track
	actionLibraryFilterClear    // esc: clear a committed filter in the library view
	actionPlayNext              // 'N': insert the focused track right after the currently playing track

	// Queue-local actions.
	actionCursorUp
	actionCursorDown
	actionSelect
	actionDequeue             // 'd': remove the selected track from the queue
	actionBulkSelectionToggle // 'x': toggle bulk-selection on the selected entry
	actionClearSelection      // 'X': clear all bulk-selected entries
	actionClearQueue          // 'C': clear all entries from the queue
	actionMoveTrackUp         // 'K': move selected track one position earlier
	actionMoveTrackDown       // 'J': move selected track one position later
	actionSaveQueueAsPlaylist // 'S': save entire queue as a new playlist
	actionFocusNowPlaying     // 'f': jump cursor to the currently playing track

	// Playlists-local actions.
	actionPlaylistRename
	actionPlaylistCreate // 'c': create new playlist
	actionPlaylistOpen   // 'enter': open playlist to display its tracks
	actionPlaylistBack   // 'esc': return to the playlist list from track level
	actionPlaylistDelete // 'd': delete focused playlist (list level) or remove focused track (track level)

	actionPlaylistEnqueueAll // 'E': enqueue all tracks from the focused playlist

	// Smart-playlist actions (keybindings wired in Session 3).
	actionSmartPlaylistCreate // 'S': create a new smart playlist
	actionSmartPlaylistEdit   // 'e': edit rules for the focused smart playlist

	// Artists-local actions.
	actionArtistOpen // enter: drill into albums/tracks or enqueue
	actionArtistBack // esc: go back one level

	// Albums-local actions.
	actionAlbumOpen // enter: drill into tracks or enqueue
	actionAlbumBack // esc: go back one level

	// Genres-local actions.
	actionSwitchGenres
	actionGenreOpen // enter: drill into artists/tracks or enqueue
	actionGenreBack // esc: go back one level

	// Bulk-selection action — triggered by 'a' when tracks are selected.
	// actionBulkAction applies in Library (append selected tracks to queue).
	actionBulkAction // 'a': apply context-appropriate bulk action and clear selection

	// Vim-style scrolling actions — shared across Library, Queue, and Playlists.
	actionHalfPageDown // ctrl+d: move cursor half a page down
	actionHalfPageUp   // ctrl+u: move cursor half a page up
	actionFullPageDown // ctrl+f: move cursor a full page down
	actionFullPageUp   // ctrl+b: move cursor a full page up
	actionGoToTop      // g: move cursor to the first row
	actionGoToBottom   // G: move cursor to the last row

	// Sort actions.
	actionSortCycle // ctrl+s: cycle to next sort field
	actionSortAsc   // shift+up: set sort direction to ascending
	actionSortDesc  // shift+down: set sort direction to descending
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
	r.register(contextGlobal, "6", actionSwitchGenres)
	r.register(contextGlobal, "7", actionSwitchFavorites)
	r.register(contextGlobal, "8", actionSwitchRecentlyAdded)
	r.register(contextGlobal, "F", actionToggleFavorite)
	r.register(contextGlobal, "R", actionRescan)
	r.register(contextGlobal, "r", actionRepeatCycle)
	r.register(contextGlobal, "z", actionShuffleToggle)
	r.register(contextGlobal, "u", actionUndo)
	r.register(contextGlobal, "/", actionFilterBar)
	r.register(contextGlobal, "?", actionHelp)
	r.register(contextGlobal, "ctrl+,", actionSettings)
	r.register(contextGlobal, "ctrl+g", actionDebug)
	r.register(contextGlobal, "L", actionStats)
	r.register(contextGlobal, "tab", actionNextView)
	r.register(contextGlobal, "shift+tab", actionPrevView)
	r.register(contextGlobal, "P", actionPreview)
	r.register(contextGlobal, "I", actionNowPlayingInfo)
	r.register(contextGlobal, "M", actionEditMetadata)
	r.register(contextGlobal, "i", actionTrackInfo)
	r.register(contextGlobal, "ctrl+s", actionSortCycle)
	r.register(contextGlobal, "shift+up", actionSortAsc)
	r.register(contextGlobal, "shift+down", actionSortDesc)
	r.register(contextGlobal, "o", actionGoTo)

	// Library-local bindings.
	r.registerNavigation(contextLibrary)
	r.register(contextLibrary, "enter", actionLibraryEnqueue)
	r.register(contextLibrary, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextLibrary, "N", actionPlayNext)
	r.register(contextLibrary, "esc", actionLibraryFilterClear)
	r.register(contextLibrary, "x", actionBulkSelectionToggle)
	r.register(contextLibrary, "X", actionClearSelection)
	r.register(contextLibrary, "a", actionBulkAction)

	// Recently Added-local bindings.
	r.registerNavigation(contextRecentlyAdded)
	r.register(contextRecentlyAdded, "enter", actionLibraryEnqueue)
	r.register(contextRecentlyAdded, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextRecentlyAdded, "N", actionPlayNext)
	r.register(contextRecentlyAdded, "esc", actionLibraryFilterClear)
	r.register(contextRecentlyAdded, "x", actionBulkSelectionToggle)
	r.register(contextRecentlyAdded, "X", actionClearSelection)
	r.register(contextRecentlyAdded, "a", actionBulkAction)

	// Queue-local bindings.
	r.registerNavigation(contextQueue)
	r.register(contextQueue, "enter", actionSelect)
	r.register(contextQueue, "d", actionDequeue)
	r.register(contextQueue, "x", actionBulkSelectionToggle)
	r.register(contextQueue, "X", actionClearSelection)
	r.register(contextQueue, "C", actionClearQueue)
	r.register(contextQueue, "K", actionMoveTrackUp)
	r.register(contextQueue, "J", actionMoveTrackDown)
	r.register(contextQueue, "S", actionSaveQueueAsPlaylist)
	r.register(contextQueue, "f", actionFocusNowPlaying)

	// Playlists-local bindings.
	// 'r' is bound to rename here, shadowing the global repeat-cycle binding
	// while the Playlists view is focused.
	r.registerNavigation(contextPlaylists)
	r.register(contextPlaylists, "r", actionPlaylistRename)
	r.register(contextPlaylists, "c", actionPlaylistCreate)
	r.register(contextPlaylists, "enter", actionPlaylistOpen)
	r.register(contextPlaylists, "esc", actionPlaylistBack)
	r.register(contextPlaylists, "d", actionPlaylistDelete)
	r.register(contextPlaylists, "N", actionPlayNext)
	r.register(contextPlaylists, "x", actionBulkSelectionToggle)
	r.register(contextPlaylists, "X", actionClearSelection)
	r.register(contextPlaylists, "a", actionBulkAction)
	r.register(contextPlaylists, "E", actionPlaylistEnqueueAll)
	r.register(contextPlaylists, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextPlaylists, "S", actionSmartPlaylistCreate)
	r.register(contextPlaylists, "e", actionSmartPlaylistEdit)

	// Artists-local bindings.
	r.registerNavigation(contextArtists)
	r.register(contextArtists, "enter", actionArtistOpen)
	r.register(contextArtists, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextArtists, "N", actionPlayNext)
	r.register(contextArtists, "esc", actionArtistBack)
	r.register(contextArtists, "x", actionBulkSelectionToggle)
	r.register(contextArtists, "X", actionClearSelection)
	r.register(contextArtists, "a", actionBulkAction)

	// Albums-local bindings.
	r.registerNavigation(contextAlbums)
	r.register(contextAlbums, "enter", actionAlbumOpen)
	r.register(contextAlbums, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextAlbums, "N", actionPlayNext)
	r.register(contextAlbums, "esc", actionAlbumBack)
	r.register(contextAlbums, "x", actionBulkSelectionToggle)
	r.register(contextAlbums, "X", actionClearSelection)
	r.register(contextAlbums, "a", actionBulkAction)

	// Genres-local bindings.
	r.registerNavigation(contextGenres)
	r.register(contextGenres, "enter", actionGenreOpen)
	r.register(contextGenres, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextGenres, "N", actionPlayNext)
	r.register(contextGenres, "esc", actionGenreBack)
	r.register(contextGenres, "x", actionBulkSelectionToggle)
	r.register(contextGenres, "X", actionClearSelection)
	r.register(contextGenres, "a", actionBulkAction)

	// Favorites-local bindings.
	r.registerNavigation(contextFavorites)
	r.register(contextFavorites, "enter", actionFavoritesOpen)
	r.register(contextFavorites, "esc", actionFavoritesBack)
	r.register(contextFavorites, "N", actionPlayNext)
	r.register(contextFavorites, "x", actionBulkSelectionToggle)
	r.register(contextFavorites, "X", actionClearSelection)
	r.register(contextFavorites, "a", actionBulkAction)
	r.register(contextFavorites, "shift+enter", actionLibraryEnqueueReplace)

	return r
}

// registerNavigation registers the shared cursor/scroll navigation bindings
// (j/k, up/down, half/full page, go-to-top/bottom) for the given context.
func (r *Registry) registerNavigation(ctx bindingContext) {
	r.register(ctx, "j", actionCursorDown)
	r.register(ctx, "k", actionCursorUp)
	r.register(ctx, "up", actionCursorUp)
	r.register(ctx, "down", actionCursorDown)
	r.register(ctx, "ctrl+d", actionHalfPageDown)
	r.register(ctx, "ctrl+u", actionHalfPageUp)
	r.register(ctx, "ctrl+f", actionFullPageDown)
	r.register(ctx, "ctrl+b", actionFullPageUp)
	r.register(ctx, "g", actionGoToTop)
	r.register(ctx, "G", actionGoToBottom)
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
