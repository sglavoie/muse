package tui

// newRegistry returns a Registry pre-populated with the application's default
// key bindings.
func newRegistry() Registry {
	r := Registry{
		bindings:      make(map[registryKey]action),
		primaries:     make(map[primaryHintKey][]primaryHint),
		domainReasons: make(map[registryKey]string),
	}

	registerDefaultBindings(&r)
	registerDefaultPrimaries(&r)
	registerSelectionModePrimaries(&r)
	registerVisualModePrimaries(&r)

	return r
}

func registerDefaultBindings(r *Registry) {
	registerGlobalBindings(r)
	registerLibraryBindings(r)
	registerQueueBindings(r)
	registerPlaylistBindings(r)
	registerBrowseBindings(r)
	registerTagsBindings(r)
	registerMixesBindings(r)
	registerFavoritesBindings(r)
}

func registerGlobalBindings(r *Registry) {
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
	r.register(contextGlobal, "4", actionSwitchBrowse)
	r.register(contextGlobal, "5", actionSwitchFavorites)
	r.register(contextGlobal, "6", actionSwitchTags)
	r.register(contextGlobal, "7", actionSwitchMixes)
	r.register(contextGlobal, "F", actionToggleFavorite)
	r.register(contextGlobal, "ctrl+r", actionRescan)
	r.register(contextGlobal, "R", actionResetPosition)
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
	r.register(contextGlobal, "alt+right", actionNextView)
	r.register(contextGlobal, "alt+left", actionPrevView)
	// Some terminals (notably macOS Terminal.app / iTerm2 with Option = Meta)
	// translate Option+Arrow into readline word-nav sequences (alt+f / alt+b).
	// Bind those as aliases so the chord works regardless of terminal config.
	// Text inputs (bubbles textinput/textarea) already bind alt+b/alt+f for
	// word navigation and consume the key before it reaches this global
	// registry, so this does not interfere with editing.
	r.register(contextGlobal, "alt+f", actionNextView)
	r.register(contextGlobal, "alt+b", actionPrevView)
	r.register(contextGlobal, "P", actionPreview)
	r.register(contextGlobal, "I", actionNowPlayingInfo)
	r.register(contextGlobal, "M", actionEditMetadata)
	r.register(contextGlobal, "i", actionTrackInfo)
	r.register(contextGlobal, "ctrl+s", actionSortCycle)
	r.register(contextGlobal, "shift+up", actionSortAsc)
	r.register(contextGlobal, "shift+down", actionSortDesc)
	r.register(contextGlobal, "o", actionGoTo)
	r.register(contextGlobal, "ctrl+n", actionStopAfterCurrent)
	r.register(contextGlobal, "q", actionQueueSummary)
	r.register(contextGlobal, ":", actionCommandPalette)
	r.register(contextGlobal, "t", actionTagPicker)
	r.register(contextGlobal, "ctrl+t", actionTagFilter)
	r.register(contextGlobal, "A", actionRadioToggle)
	r.register(contextGlobal, "f", actionFocusNowPlaying)
}

func registerLibraryBindings(r *Registry) {
	r.registerNavigation(contextLibrary)
	r.register(contextLibrary, "enter", actionLibraryEnqueue)
	r.register(contextLibrary, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextLibrary, "N", actionPlayNext)
	r.register(contextLibrary, "esc", actionLibraryFilterClear)
	r.register(contextLibrary, "x", actionBulkSelectionToggle)
	r.register(contextLibrary, "X", actionClearSelection)
	r.register(contextLibrary, "v", actionVisualMode)
	r.register(contextLibrary, "a", actionBulkAction)
	r.register(contextLibrary, "`", actionLibraryModeNext)
	r.register(contextLibrary, "~", actionLibraryModePrev)
}

func registerQueueBindings(r *Registry) {
	r.registerNavigation(contextQueue)
	r.register(contextQueue, "enter", actionSelect)
	r.register(contextQueue, "d", actionDequeue)
	r.register(contextQueue, "x", actionBulkSelectionToggle)
	r.register(contextQueue, "X", actionClearSelection)
	r.register(contextQueue, "v", actionVisualMode)
	r.register(contextQueue, "C", actionClearQueue)
	r.register(contextQueue, "K", actionMoveTrackUp)
	r.register(contextQueue, "J", actionMoveTrackDown)
	r.register(contextQueue, "S", actionSaveQueueAsPlaylist)
	r.register(contextQueue, "ctrl+y", actionYank)
	r.register(contextQueue, "ctrl+x", actionCut)
	r.register(contextQueue, "ctrl+p", actionPaste)
	r.register(contextQueue, "esc", actionLibraryFilterClear)
}

func registerPlaylistBindings(r *Registry) {
	r.registerNavigation(contextPlaylists)
	r.registerWithReason(contextPlaylists, "r", actionPlaylistRename,
		"rename focused playlist (or refresh smart playlist at track level)")
	r.register(contextPlaylists, "c", actionPlaylistCreate)
	r.register(contextPlaylists, "enter", actionPlaylistOpen)
	r.register(contextPlaylists, "esc", actionPlaylistBack)
	r.register(contextPlaylists, "d", actionPlaylistDelete)
	r.register(contextPlaylists, "N", actionPlayNext)
	r.register(contextPlaylists, "x", actionBulkSelectionToggle)
	r.register(contextPlaylists, "X", actionClearSelection)
	r.register(contextPlaylists, "v", actionVisualMode)
	r.register(contextPlaylists, "a", actionBulkAction)
	r.register(contextPlaylists, "E", actionPlaylistEnqueueAll)
	r.register(contextPlaylists, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextPlaylists, "S", actionSmartPlaylistCreate)
	r.register(contextPlaylists, "e", actionSmartPlaylistEdit)
	r.register(contextPlaylists, "K", actionMoveTrackUp)
	r.register(contextPlaylists, "J", actionMoveTrackDown)
}

func registerBrowseBindings(r *Registry) {
	registerArtistsBindings(r)
	registerAlbumsBindings(r)
	registerGenresBindings(r)
}

func registerArtistsBindings(r *Registry) {
	r.registerNavigation(contextArtists)
	r.register(contextArtists, "enter", actionArtistOpen)
	r.register(contextArtists, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextArtists, "N", actionPlayNext)
	r.register(contextArtists, "esc", actionArtistBack)
	r.register(contextArtists, "x", actionBulkSelectionToggle)
	r.register(contextArtists, "X", actionClearSelection)
	r.register(contextArtists, "v", actionVisualMode)
	r.register(contextArtists, "a", actionBulkAction)
	r.register(contextArtists, "`", actionBrowseModeNext)
	r.register(contextArtists, "~", actionBrowseModePrev)
}

func registerAlbumsBindings(r *Registry) {
	r.registerNavigation(contextAlbums)
	r.register(contextAlbums, "enter", actionAlbumOpen)
	r.register(contextAlbums, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextAlbums, "N", actionPlayNext)
	r.register(contextAlbums, "esc", actionAlbumBack)
	r.register(contextAlbums, "x", actionBulkSelectionToggle)
	r.register(contextAlbums, "X", actionClearSelection)
	r.register(contextAlbums, "v", actionVisualMode)
	r.register(contextAlbums, "a", actionBulkAction)
	r.register(contextAlbums, "`", actionBrowseModeNext)
	r.register(contextAlbums, "~", actionBrowseModePrev)
}

func registerGenresBindings(r *Registry) {
	r.registerNavigation(contextGenres)
	r.register(contextGenres, "enter", actionGenreOpen)
	r.register(contextGenres, "shift+enter", actionLibraryEnqueueReplace)
	r.register(contextGenres, "N", actionPlayNext)
	r.register(contextGenres, "esc", actionGenreBack)
	r.register(contextGenres, "x", actionBulkSelectionToggle)
	r.register(contextGenres, "X", actionClearSelection)
	r.register(contextGenres, "v", actionVisualMode)
	r.register(contextGenres, "a", actionBulkAction)
	r.register(contextGenres, "`", actionBrowseModeNext)
	r.register(contextGenres, "~", actionBrowseModePrev)
}

func registerTagsBindings(r *Registry) {
	r.registerNavigation(contextTags)
	r.register(contextTags, "enter", actionTagOpen)
	r.register(contextTags, "backspace", actionTagBack)
	r.register(contextTags, "h", actionTagBack)
	r.register(contextTags, "c", actionTagCreate)
	r.registerWithReason(contextTags, "r", actionTagRename,
		"rename focused tag inline (mirrors playlists 'r')")
	r.register(contextTags, "d", actionTagDelete)
	r.register(contextTags, "E", actionTagEnqueueAll)
	r.register(contextTags, "m", actionTagMerge)
	r.register(contextTags, "N", actionPlayNext)
	r.register(contextTags, "x", actionBulkSelectionToggle)
	r.register(contextTags, "X", actionClearSelection)
	r.register(contextTags, "v", actionVisualMode)
	r.register(contextTags, "a", actionBulkAction)
	r.register(contextTags, "esc", actionTagBack)
	r.register(contextTags, "F", actionToggleFavorite)
}

func registerMixesBindings(r *Registry) {
	r.registerNavigation(contextMixes)
	r.register(contextMixes, "enter", actionMixOpen)
	r.register(contextMixes, "esc", actionMixBack)
	r.register(contextMixes, "e", actionLibraryEnqueue)
	r.register(contextMixes, "E", actionMixEnqueueAll)
	r.register(contextMixes, "N", actionPlayNext)
	r.register(contextMixes, "x", actionMixDismiss)
	r.register(contextMixes, "X", actionMixUndismiss)
	r.register(contextMixes, "v", actionVisualMode)
	r.register(contextMixes, "a", actionBulkAction)
	r.registerWithReason(contextMixes, "s", actionSortCycle,
		"one-key sort cycle at mix track level (ctrl+s remains available)")
	r.register(contextMixes, "S", actionSaveMixAsPlaylist)
	r.registerWithReason(contextMixes, "A", actionMixRadioSeed,
		"seed radio from focused mix's aggregate metadata")
}

func registerFavoritesBindings(r *Registry) {
	r.registerNavigation(contextFavorites)
	r.register(contextFavorites, "enter", actionFavoritesOpen)
	r.register(contextFavorites, "esc", actionFavoritesBack)
	r.register(contextFavorites, "N", actionPlayNext)
	r.register(contextFavorites, "x", actionBulkSelectionToggle)
	r.register(contextFavorites, "X", actionClearSelection)
	r.register(contextFavorites, "v", actionVisualMode)
	r.register(contextFavorites, "a", actionBulkAction)
	r.register(contextFavorites, "shift+enter", actionLibraryEnqueueReplace)
}

func registerDefaultPrimaries(r *Registry) {
	// Primary hints surfaced in the footer hint strip. Keys are referenced
	// by string (not action enum) because the same physical key may map to
	// different actions across contexts. Use addPrimary for keys whose
	// binding lives in contextGlobal so we don't double-register.
	r.addPrimary(contextLibrary, "j/k", "nav")
	r.addPrimary(contextLibrary, "enter", "enqueue")
	r.addPrimary(contextLibrary, "N", "play-next")
	r.addPrimary(contextLibrary, "/", "filter")
	r.addPrimary(contextLibrary, "v", "select")
	r.addPrimary(contextLibrary, "?", "help")

	r.addPrimary(contextQueue, "j/k", "nav")
	r.addPrimary(contextQueue, "enter", "play")
	r.addPrimary(contextQueue, "d", "remove")
	r.addPrimary(contextQueue, "S", "save")
	r.addPrimary(contextQueue, "/", "filter")
	r.addPrimary(contextQueue, "?", "help")

	r.addPrimary(contextPlaylists, "j/k", "nav")
	r.addPrimary(contextPlaylists, "enter", "open")
	r.addPrimary(contextPlaylists, "r", "rename")
	r.addPrimary(contextPlaylists, "c", "new")
	r.addPrimary(contextPlaylists, "/", "filter")
	r.addPrimary(contextPlaylists, "?", "help")

	for _, ctx := range []bindingContext{contextBrowse, contextArtists, contextAlbums, contextGenres} {
		r.addPrimary(ctx, "j/k", "nav")
		r.addPrimary(ctx, "enter", "open")
		r.addPrimary(ctx, "esc", "back")
		r.addPrimary(ctx, "`/~", "cycle mode")
		r.addPrimary(ctx, "/", "filter")
		r.addPrimary(ctx, "?", "help")
	}

	r.addPrimary(contextFavorites, "j/k", "nav")
	r.addPrimary(contextFavorites, "enter", "open")
	r.addPrimary(contextFavorites, "F", "toggle fav")
	r.addPrimary(contextFavorites, "/", "filter")
	r.addPrimary(contextFavorites, "?", "help")

	r.addPrimary(contextTags, "j/k", "nav")
	r.addPrimary(contextTags, "enter", "filter-by")
	r.addPrimary(contextTags, "c", "create")
	r.addPrimary(contextTags, "r", "rename")
	r.addPrimary(contextTags, "/", "filter")
	r.addPrimary(contextTags, "?", "help")

	r.addPrimary(contextMixes, "j/k", "nav")
	r.addPrimary(contextMixes, "enter", "open")
	r.addPrimary(contextMixes, "E", "enqueue all")
	r.addPrimary(contextMixes, "A", "radio")
	r.addPrimary(contextMixes, "/", "filter")
	r.addPrimary(contextMixes, "?", "help")
}

func registerSelectionModePrimaries(r *Registry) {
	// Selection-mode hint sets (surfaced when 1+ rows are bulk-selected).
	// These are not new bindings — every key here is already registered for
	// the context. The selection set highlights the actions that operate on
	// the current selection rather than a single focused row.
	r.addPrimaryMode(contextLibrary, HintModeSelection, "enter", "enqueue")
	r.addPrimaryMode(contextLibrary, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(contextLibrary, HintModeSelection, "t", "tag")
	r.addPrimaryMode(contextLibrary, HintModeSelection, "X", "clear")
	r.addPrimaryMode(contextLibrary, HintModeSelection, "?", "help")

	r.addPrimaryMode(contextQueue, HintModeSelection, "enter", "play")
	r.addPrimaryMode(contextQueue, HintModeSelection, "d", "remove")
	r.addPrimaryMode(contextQueue, HintModeSelection, "t", "tag")
	r.addPrimaryMode(contextQueue, HintModeSelection, "S", "save")
	r.addPrimaryMode(contextQueue, HintModeSelection, "X", "clear")
	r.addPrimaryMode(contextQueue, HintModeSelection, "?", "help")

	r.addPrimaryMode(contextPlaylists, HintModeSelection, "enter", "open")
	r.addPrimaryMode(contextPlaylists, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(contextPlaylists, HintModeSelection, "t", "tag")
	r.addPrimaryMode(contextPlaylists, HintModeSelection, "X", "clear")
	r.addPrimaryMode(contextPlaylists, HintModeSelection, "?", "help")

	for _, ctx := range []bindingContext{contextBrowse, contextArtists, contextAlbums, contextGenres} {
		r.addPrimaryMode(ctx, HintModeSelection, "enter", "open")
		r.addPrimaryMode(ctx, HintModeSelection, "a", "bulk action")
		r.addPrimaryMode(ctx, HintModeSelection, "t", "tag")
		r.addPrimaryMode(ctx, HintModeSelection, "X", "clear")
		r.addPrimaryMode(ctx, HintModeSelection, "?", "help")
	}

	r.addPrimaryMode(contextFavorites, HintModeSelection, "enter", "open")
	r.addPrimaryMode(contextFavorites, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(contextFavorites, HintModeSelection, "F", "toggle fav")
	r.addPrimaryMode(contextFavorites, HintModeSelection, "X", "clear")
	r.addPrimaryMode(contextFavorites, HintModeSelection, "?", "help")

	r.addPrimaryMode(contextTags, HintModeSelection, "enter", "filter-by")
	r.addPrimaryMode(contextTags, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(contextTags, HintModeSelection, "t", "tag")
	r.addPrimaryMode(contextTags, HintModeSelection, "X", "clear")
	r.addPrimaryMode(contextTags, HintModeSelection, "?", "help")
}

func registerVisualModePrimaries(r *Registry) {
	// Visual-mode hint sets (surfaced while visual line-selection is active).
	// Visual mode is launched by 'v' and exited with 'esc'; while active,
	// j/k extend the range and the action keys behave like selection-mode
	// shortcuts once the user exits visual.
	for _, ctx := range []bindingContext{
		contextLibrary, contextQueue, contextPlaylists,
		contextBrowse, contextArtists, contextAlbums, contextGenres,
		contextFavorites, contextTags,
	} {
		r.addPrimaryMode(ctx, HintModeVisual, "j/k", "extend")
		r.addPrimaryMode(ctx, HintModeVisual, "v", "exit")
		r.addPrimaryMode(ctx, HintModeVisual, "esc", "cancel")
		r.addPrimaryMode(ctx, HintModeVisual, "X", "clear")
		r.addPrimaryMode(ctx, HintModeVisual, "?", "help")
	}
}
