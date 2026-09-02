package keys

// NewRegistry returns a Registry pre-populated with the application's default
// key bindings.
func NewRegistry() Registry {
	r := Registry{
		Bindings:      make(map[RegistryKey]Action),
		primaries:     make(map[primaryHintKey][]primaryHint),
		domainReasons: make(map[RegistryKey]string),
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
	registerInsightsBindings(r)
}

func registerGlobalBindings(r *Registry) {
	r.register(ContextGlobal, "ctrl+c", ActionQuit)
	r.register(ContextGlobal, "space", ActionPlayPause)
	r.register(ContextGlobal, "s", ActionStop)
	r.register(ContextGlobal, "n", ActionNext)
	r.register(ContextGlobal, "p", ActionPrevious)
	r.register(ContextGlobal, "left", ActionSeekBack)
	r.register(ContextGlobal, "right", ActionSeekForward)
	r.register(ContextGlobal, "shift+left", ActionSeekBackLong)
	r.register(ContextGlobal, "shift+right", ActionSeekForwardLong)
	r.register(ContextGlobal, "=", ActionVolumeUp)
	r.register(ContextGlobal, "-", ActionVolumeDown)
	r.register(ContextGlobal, "1", ActionSwitchLibrary)
	r.register(ContextGlobal, "2", ActionSwitchPlaylists)
	r.register(ContextGlobal, "3", ActionSwitchQueue)
	r.register(ContextGlobal, "4", ActionSwitchBrowse)
	r.register(ContextGlobal, "5", ActionSwitchFavorites)
	r.register(ContextGlobal, "6", ActionSwitchTags)
	r.register(ContextGlobal, "7", ActionSwitchMixes)
	r.register(ContextGlobal, "8", ActionSwitchInsights)
	r.register(ContextGlobal, "F", ActionToggleFavorite)
	r.register(ContextGlobal, "ctrl+r", ActionRescan)
	r.register(ContextGlobal, "R", ActionResetPosition)
	r.register(ContextGlobal, "r", ActionRepeatCycle)
	r.register(ContextGlobal, "z", ActionShuffleToggle)
	r.register(ContextGlobal, "u", ActionUndo)
	r.register(ContextGlobal, "/", ActionFilterBar)
	r.register(ContextGlobal, "?", ActionHelp)
	r.register(ContextGlobal, "ctrl+,", ActionSettings)
	r.register(ContextGlobal, "ctrl+g", ActionDebug)
	r.register(ContextGlobal, "L", ActionSwitchInsights)
	r.register(ContextGlobal, "tab", ActionNextView)
	r.register(ContextGlobal, "shift+tab", ActionPrevView)
	r.register(ContextGlobal, "]", ActionNextView)
	r.register(ContextGlobal, "[", ActionPrevView)
	r.register(ContextGlobal, "P", ActionPreview)
	r.register(ContextGlobal, "I", ActionNowPlayingInfo)
	r.register(ContextGlobal, "M", ActionEditMetadata)
	r.register(ContextGlobal, "i", ActionTrackInfo)
	r.register(ContextGlobal, "ctrl+s", ActionSortCycle)
	r.register(ContextGlobal, "shift+up", ActionSortAsc)
	r.register(ContextGlobal, "shift+down", ActionSortDesc)
	r.register(ContextGlobal, "o", ActionGoTo)
	r.register(ContextGlobal, "ctrl+n", ActionStopAfterCurrent)
	r.register(ContextGlobal, "q", ActionQueueSummary)
	r.register(ContextGlobal, ";", ActionModesLegend)
	r.register(ContextGlobal, ":", ActionCommandPalette)
	r.register(ContextGlobal, "t", ActionTagPicker)
	r.register(ContextGlobal, "ctrl+t", ActionTagFilter)
	r.register(ContextGlobal, "A", ActionRadioToggle)
	r.register(ContextGlobal, "f", ActionFocusNowPlaying)
}

func registerLibraryBindings(r *Registry) {
	r.registerNavigation(ContextLibrary)
	r.register(ContextLibrary, "enter", ActionLibraryEnqueue)
	r.register(ContextLibrary, "shift+enter", ActionLibraryEnqueueReplace)
	r.register(ContextLibrary, "N", ActionPlayNext)
	r.register(ContextLibrary, "esc", ActionLibraryFilterClear)
	r.register(ContextLibrary, "x", ActionBulkSelectionToggle)
	r.register(ContextLibrary, "X", ActionClearSelection)
	r.register(ContextLibrary, "v", ActionVisualMode)
	r.register(ContextLibrary, "a", ActionBulkAction)
	r.register(ContextLibrary, "`", ActionLibraryModeNext)
	r.register(ContextLibrary, "~", ActionLibraryModePrev)
}

func registerQueueBindings(r *Registry) {
	r.registerNavigation(ContextQueue)
	r.register(ContextQueue, "enter", ActionSelect)
	r.register(ContextQueue, "d", ActionDequeue)
	r.register(ContextQueue, "x", ActionBulkSelectionToggle)
	r.register(ContextQueue, "X", ActionClearSelection)
	r.register(ContextQueue, "v", ActionVisualMode)
	r.register(ContextQueue, "C", ActionClearQueue)
	r.register(ContextQueue, "K", ActionMoveTrackUp)
	r.register(ContextQueue, "J", ActionMoveTrackDown)
	r.register(ContextQueue, "S", ActionSaveQueueAsPlaylist)
	r.register(ContextQueue, "ctrl+y", ActionYank)
	r.register(ContextQueue, "ctrl+x", ActionCut)
	r.register(ContextQueue, "ctrl+p", ActionPaste)
	r.register(ContextQueue, "esc", ActionLibraryFilterClear)
}

func registerPlaylistBindings(r *Registry) {
	r.registerNavigation(ContextPlaylists)
	r.registerWithReason(ContextPlaylists, "r", ActionPlaylistRename,
		"rename focused playlist (or refresh smart playlist at track level)")
	r.register(ContextPlaylists, "c", ActionPlaylistCreate)
	r.register(ContextPlaylists, "enter", ActionPlaylistOpen)
	r.register(ContextPlaylists, "esc", ActionPlaylistBack)
	r.register(ContextPlaylists, "d", ActionPlaylistDelete)
	r.register(ContextPlaylists, "N", ActionPlayNext)
	r.register(ContextPlaylists, "x", ActionBulkSelectionToggle)
	r.register(ContextPlaylists, "X", ActionClearSelection)
	r.register(ContextPlaylists, "v", ActionVisualMode)
	r.register(ContextPlaylists, "a", ActionBulkAction)
	r.register(ContextPlaylists, "E", ActionPlaylistEnqueueAll)
	r.register(ContextPlaylists, "shift+enter", ActionLibraryEnqueueReplace)
	r.register(ContextPlaylists, "S", ActionSmartPlaylistCreate)
	r.register(ContextPlaylists, "e", ActionSmartPlaylistEdit)
	r.register(ContextPlaylists, "K", ActionMoveTrackUp)
	r.register(ContextPlaylists, "J", ActionMoveTrackDown)
}

func registerBrowseBindings(r *Registry) {
	registerArtistsBindings(r)
	registerAlbumsBindings(r)
	registerGenresBindings(r)
}

func registerArtistsBindings(r *Registry) {
	r.registerNavigation(ContextArtists)
	r.register(ContextArtists, "enter", ActionArtistOpen)
	r.register(ContextArtists, "shift+enter", ActionLibraryEnqueueReplace)
	r.register(ContextArtists, "N", ActionPlayNext)
	r.register(ContextArtists, "esc", ActionArtistBack)
	r.register(ContextArtists, "x", ActionBulkSelectionToggle)
	r.register(ContextArtists, "X", ActionClearSelection)
	r.register(ContextArtists, "v", ActionVisualMode)
	r.register(ContextArtists, "a", ActionBulkAction)
	r.register(ContextArtists, "`", ActionBrowseModeNext)
	r.register(ContextArtists, "~", ActionBrowseModePrev)
}

func registerAlbumsBindings(r *Registry) {
	r.registerNavigation(ContextAlbums)
	r.register(ContextAlbums, "enter", ActionAlbumOpen)
	r.register(ContextAlbums, "shift+enter", ActionLibraryEnqueueReplace)
	r.register(ContextAlbums, "N", ActionPlayNext)
	r.register(ContextAlbums, "esc", ActionAlbumBack)
	r.register(ContextAlbums, "x", ActionBulkSelectionToggle)
	r.register(ContextAlbums, "X", ActionClearSelection)
	r.register(ContextAlbums, "v", ActionVisualMode)
	r.register(ContextAlbums, "a", ActionBulkAction)
	r.register(ContextAlbums, "`", ActionBrowseModeNext)
	r.register(ContextAlbums, "~", ActionBrowseModePrev)
}

func registerGenresBindings(r *Registry) {
	r.registerNavigation(ContextGenres)
	r.register(ContextGenres, "enter", ActionGenreOpen)
	r.register(ContextGenres, "shift+enter", ActionLibraryEnqueueReplace)
	r.register(ContextGenres, "N", ActionPlayNext)
	r.register(ContextGenres, "esc", ActionGenreBack)
	r.register(ContextGenres, "x", ActionBulkSelectionToggle)
	r.register(ContextGenres, "X", ActionClearSelection)
	r.register(ContextGenres, "v", ActionVisualMode)
	r.register(ContextGenres, "a", ActionBulkAction)
	r.register(ContextGenres, "`", ActionBrowseModeNext)
	r.register(ContextGenres, "~", ActionBrowseModePrev)
}

func registerTagsBindings(r *Registry) {
	r.registerNavigation(ContextTags)
	r.register(ContextTags, "enter", ActionTagOpen)
	r.register(ContextTags, "backspace", ActionTagBack)
	r.register(ContextTags, "h", ActionTagBack)
	r.register(ContextTags, "c", ActionTagCreate)
	r.registerWithReason(ContextTags, "r", ActionTagRename,
		"rename focused tag inline (mirrors playlists 'r')")
	r.register(ContextTags, "d", ActionTagDelete)
	r.register(ContextTags, "E", ActionTagEnqueueAll)
	r.register(ContextTags, "m", ActionTagMerge)
	r.register(ContextTags, "N", ActionPlayNext)
	r.register(ContextTags, "x", ActionBulkSelectionToggle)
	r.register(ContextTags, "X", ActionClearSelection)
	r.register(ContextTags, "v", ActionVisualMode)
	r.register(ContextTags, "a", ActionBulkAction)
	r.register(ContextTags, "esc", ActionTagBack)
	r.register(ContextTags, "F", ActionToggleFavorite)
}

func registerMixesBindings(r *Registry) {
	r.registerNavigation(ContextMixes)
	r.register(ContextMixes, "enter", ActionMixOpen)
	r.register(ContextMixes, "esc", ActionMixBack)
	r.register(ContextMixes, "e", ActionLibraryEnqueue)
	r.register(ContextMixes, "E", ActionMixEnqueueAll)
	r.register(ContextMixes, "N", ActionPlayNext)
	r.register(ContextMixes, "x", ActionMixDismiss)
	r.register(ContextMixes, "X", ActionMixUndismiss)
	r.register(ContextMixes, "v", ActionVisualMode)
	r.register(ContextMixes, "a", ActionBulkAction)
	r.registerWithReason(ContextMixes, "s", ActionSortCycle,
		"one-key sort cycle at mix track level (ctrl+s remains available)")
	r.register(ContextMixes, "S", ActionSaveMixAsPlaylist)
	r.registerWithReason(ContextMixes, "A", ActionMixRadioSeed,
		"seed radio from focused mix's aggregate metadata")
}

func registerInsightsBindings(r *Registry) {
	r.registerNavigation(ContextInsights)
	r.register(ContextInsights, "enter", ActionInsightsOpen)
	r.register(ContextInsights, "esc", ActionInsightsBack)
	r.register(ContextInsights, "`", ActionInsightsPanelNext)
	r.register(ContextInsights, "~", ActionInsightsPanelPrev)
	r.register(ContextInsights, "w", ActionInsightsWindowNext)
	r.register(ContextInsights, "W", ActionInsightsWindowPrev)
	r.register(ContextInsights, "e", ActionLibraryEnqueue)
	r.register(ContextInsights, "E", ActionInsightsEnqueueAll)
	r.register(ContextInsights, "!", ActionInsightsAntiRut)
	r.register(ContextInsights, "b", ActionInsightsJumpBrowse)
	r.register(ContextInsights, "S", ActionInsightsSavePlaylist)
	r.register(ContextInsights, "m", ActionInsightsMergeGenre)
	r.register(ContextInsights, "x", ActionBulkSelectionToggle)
	r.register(ContextInsights, "X", ActionClearSelection)
	r.register(ContextInsights, "v", ActionVisualMode)
	r.register(ContextInsights, "a", ActionBulkAction)
	r.registerWithReason(ContextInsights, "A", ActionInsightsRadioSeed,
		"seed radio from the focused insight row rather than toggling auto-DJ")
}

func registerFavoritesBindings(r *Registry) {
	r.registerNavigation(ContextFavorites)
	r.register(ContextFavorites, "enter", ActionFavoritesOpen)
	r.register(ContextFavorites, "esc", ActionFavoritesBack)
	r.register(ContextFavorites, "N", ActionPlayNext)
	r.register(ContextFavorites, "x", ActionBulkSelectionToggle)
	r.register(ContextFavorites, "X", ActionClearSelection)
	r.register(ContextFavorites, "v", ActionVisualMode)
	r.register(ContextFavorites, "a", ActionBulkAction)
	r.register(ContextFavorites, "shift+enter", ActionLibraryEnqueueReplace)
}

func registerDefaultPrimaries(r *Registry) {
	// Primary hints surfaced in the footer hint strip. Keys are referenced
	// by string (not action enum) because the same physical key may map to
	// different actions across contexts. Use addPrimary for keys whose
	// binding lives in contextGlobal so we don't double-register.
	r.addPrimary(ContextLibrary, "j/k", "nav")
	r.addPrimary(ContextLibrary, "enter", "enqueue")
	r.addPrimary(ContextLibrary, "N", "play-next")
	r.addPrimary(ContextLibrary, "/", "filter")
	r.addPrimary(ContextLibrary, "v", "select")
	r.addPrimary(ContextLibrary, "?", "help")

	r.addPrimary(ContextQueue, "j/k", "nav")
	r.addPrimary(ContextQueue, "enter", "play")
	r.addPrimary(ContextQueue, "d", "remove")
	r.addPrimary(ContextQueue, "S", "save")
	r.addPrimary(ContextQueue, "/", "filter")
	r.addPrimary(ContextQueue, "?", "help")

	r.addPrimary(ContextPlaylists, "j/k", "nav")
	r.addPrimary(ContextPlaylists, "enter", "open")
	r.addPrimary(ContextPlaylists, "r", "rename")
	r.addPrimary(ContextPlaylists, "c", "new")
	r.addPrimary(ContextPlaylists, "/", "filter")
	r.addPrimary(ContextPlaylists, "?", "help")

	for _, ctx := range []Context{ContextBrowse, ContextArtists, ContextAlbums, ContextGenres} {
		r.addPrimary(ctx, "j/k", "nav")
		r.addPrimary(ctx, "enter", "open")
		r.addPrimary(ctx, "esc", "back")
		r.addPrimary(ctx, "`/~", "cycle mode")
		r.addPrimary(ctx, "/", "filter")
		r.addPrimary(ctx, "?", "help")
	}

	r.addPrimary(ContextFavorites, "j/k", "nav")
	r.addPrimary(ContextFavorites, "enter", "open")
	r.addPrimary(ContextFavorites, "F", "choose fav")
	r.addPrimary(ContextFavorites, "/", "filter")
	r.addPrimary(ContextFavorites, "?", "help")

	r.addPrimary(ContextTags, "j/k", "nav")
	r.addPrimary(ContextTags, "enter", "filter-by")
	r.addPrimary(ContextTags, "c", "create")
	r.addPrimary(ContextTags, "r", "rename")
	r.addPrimary(ContextTags, "/", "filter")
	r.addPrimary(ContextTags, "?", "help")

	r.addPrimary(ContextMixes, "j/k", "nav")
	r.addPrimary(ContextMixes, "enter", "open")
	r.addPrimary(ContextMixes, "E", "enqueue all")
	r.addPrimary(ContextMixes, "A", "radio")
	r.addPrimary(ContextMixes, "/", "filter")
	r.addPrimary(ContextMixes, "?", "help")

	r.addPrimary(ContextInsights, "j/k", "nav")
	r.addPrimary(ContextInsights, "`/~", "panel")
	r.addPrimary(ContextInsights, "w/W", "window")
	r.addPrimary(ContextInsights, "enter", "open")
	r.addPrimary(ContextInsights, "E", "enqueue all")
	r.addPrimary(ContextInsights, "?", "help")
}

func registerSelectionModePrimaries(r *Registry) {
	// Selection-mode hint sets (surfaced when 1+ rows are bulk-selected).
	// These are not new bindings — every key here is already registered for
	// the context. The selection set highlights the actions that operate on
	// the current selection rather than a single focused row.
	r.addPrimaryMode(ContextLibrary, HintModeSelection, "enter", "enqueue")
	r.addPrimaryMode(ContextLibrary, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(ContextLibrary, HintModeSelection, "t", "tag")
	r.addPrimaryMode(ContextLibrary, HintModeSelection, "X", "clear")
	r.addPrimaryMode(ContextLibrary, HintModeSelection, "?", "help")

	r.addPrimaryMode(ContextQueue, HintModeSelection, "enter", "play")
	r.addPrimaryMode(ContextQueue, HintModeSelection, "d", "remove")
	r.addPrimaryMode(ContextQueue, HintModeSelection, "t", "tag")
	r.addPrimaryMode(ContextQueue, HintModeSelection, "S", "save")
	r.addPrimaryMode(ContextQueue, HintModeSelection, "X", "clear")
	r.addPrimaryMode(ContextQueue, HintModeSelection, "?", "help")

	r.addPrimaryMode(ContextPlaylists, HintModeSelection, "enter", "open")
	r.addPrimaryMode(ContextPlaylists, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(ContextPlaylists, HintModeSelection, "t", "tag")
	r.addPrimaryMode(ContextPlaylists, HintModeSelection, "X", "clear")
	r.addPrimaryMode(ContextPlaylists, HintModeSelection, "?", "help")

	for _, ctx := range []Context{ContextBrowse, ContextArtists, ContextAlbums, ContextGenres} {
		r.addPrimaryMode(ctx, HintModeSelection, "enter", "open")
		r.addPrimaryMode(ctx, HintModeSelection, "a", "bulk action")
		r.addPrimaryMode(ctx, HintModeSelection, "t", "tag")
		r.addPrimaryMode(ctx, HintModeSelection, "X", "clear")
		r.addPrimaryMode(ctx, HintModeSelection, "?", "help")
	}

	r.addPrimaryMode(ContextFavorites, HintModeSelection, "enter", "open")
	r.addPrimaryMode(ContextFavorites, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(ContextFavorites, HintModeSelection, "F", "choose fav")
	r.addPrimaryMode(ContextFavorites, HintModeSelection, "X", "clear")
	r.addPrimaryMode(ContextFavorites, HintModeSelection, "?", "help")

	r.addPrimaryMode(ContextTags, HintModeSelection, "enter", "filter-by")
	r.addPrimaryMode(ContextTags, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(ContextTags, HintModeSelection, "t", "tag")
	r.addPrimaryMode(ContextTags, HintModeSelection, "X", "clear")
	r.addPrimaryMode(ContextTags, HintModeSelection, "?", "help")

	r.addPrimaryMode(ContextInsights, HintModeSelection, "enter", "enqueue")
	r.addPrimaryMode(ContextInsights, HintModeSelection, "a", "bulk action")
	r.addPrimaryMode(ContextInsights, HintModeSelection, "t", "tag")
	r.addPrimaryMode(ContextInsights, HintModeSelection, "X", "clear")
	r.addPrimaryMode(ContextInsights, HintModeSelection, "?", "help")
}

func registerVisualModePrimaries(r *Registry) {
	// Visual-mode hint sets (surfaced while visual line-selection is active).
	// Visual mode is launched by 'v' and exited with 'esc'; while active,
	// j/k extend the range and the action keys behave like selection-mode
	// shortcuts once the user exits visual.
	for _, ctx := range []Context{
		ContextLibrary, ContextQueue, ContextPlaylists,
		ContextBrowse, ContextArtists, ContextAlbums, ContextGenres,
		ContextFavorites, ContextTags,
	} {
		r.addPrimaryMode(ctx, HintModeVisual, "j/k", "extend")
		r.addPrimaryMode(ctx, HintModeVisual, "v", "exit")
		r.addPrimaryMode(ctx, HintModeVisual, "esc", "cancel")
		r.addPrimaryMode(ctx, HintModeVisual, "X", "clear")
		r.addPrimaryMode(ctx, HintModeVisual, "?", "help")
	}

	// Insights only enters visual mode inside a drilled-in track list, where esc
	// leaves the drill outright rather than merely cancelling the range.
	r.addPrimaryMode(ContextInsights, HintModeVisual, "j/k", "extend")
	r.addPrimaryMode(ContextInsights, HintModeVisual, "v", "exit")
	r.addPrimaryMode(ContextInsights, HintModeVisual, "esc", "back")
	r.addPrimaryMode(ContextInsights, HintModeVisual, "X", "clear")
	r.addPrimaryMode(ContextInsights, HintModeVisual, "?", "help")
}
