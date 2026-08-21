package tui

import "slices"

// Naming rules for palette entries, enforced by
// TestPaletteCatalogNamesFollowNamingRules:
//
//  1. Imperative verb first — "Show listening stats", not "Listening stats".
//  2. No trailing punctuation, except a trailing "…" marking an entry that
//     opens a picker or modal instead of acting immediately.
//  3. At most maxPaletteNameWidth columns wide.
//  4. Category drawn from paletteCategories.
//
// maxPaletteNameWidth is a readability cap, not the layout ceiling. A palette
// row costs len(Name) + 6 + (widest shortcut + 2) + (widest category + 3)
// columns (command_palette.go:230-259), and innerWidth is clamped to
// termWidth-4, so with the widest bound key ("shift+enter", 11 columns) and a
// 5-column category a name only starts truncating past 89 columns at the
// 120x50 command-palette-search scenario, or 69 at the 100x30 palette step of
// layout-regressions-rich. 32 keeps the whole box at 61 columns — about half
// the 120-column reference terminal — with two columns of headroom over the
// longest name in the palette today.
//
// Dynamic names such as "Enqueue tag: " + tag are data-dependent and cannot be
// capped here; only static catalog names are checked.
const maxPaletteNameWidth = 32

// paletteCategories is the closed category vocabulary, mapping each category to
// what it covers. Categories are at most 5 columns because the category column
// is sized from the widest one (command_palette.go:236-245).
var paletteCategories = map[string]string{
	"app":   "app-wide settings, stats and undo",
	"dev":   "developer and diagnostic tools",
	"file":  "files, directories and the clipboard",
	"lib":   "library data: scanning, metadata, refresh",
	"list":  "playlists and smart playlists",
	"mix":   "generated mixes and recommendations",
	"mode":  "toggles that change a persistent mode",
	"nav":   "jumping to a view, mode or row",
	"play":  "playback transport, volume and info",
	"queue": "the play queue and its contents",
	"sort":  "sort field and direction",
	"stats": "listening insights and library coverage",
	"tag":   "tags and tag filtering",
	"trk":   "the focused track",
	"view":  "the active view's own presentation",
}

// paletteEntry describes a command-palette entry backed by a keybinding
// action. Commands are built from these at palette-open time, so the shortcut
// shown always comes from the live registry and a rebind can never leave a
// stale hint behind.
//
// ctx and view are independent: viewBrowse carries three dynamically resolved
// sub-contexts (keybindings.go:85), while contextArtists/contextAlbums/
// contextGenres are reached only through the legacy sub-view IDs that are
// deliberately absent from viewOrder (view_ids.go:15-18).
type paletteEntry struct {
	act       action
	id        string // stable identifier for usage tracking
	name      string
	category  string         // one of paletteCategories
	ctx       bindingContext // owning context, used for the shortcut reverse-lookup
	view      viewID         // view to switch to before running; 0 runs in place
	preferKey string         // when set, overrides the keysFor ordering for display
	// enabled runs against the pre-switch model: the row is rendered while the
	// user is still in whatever view the palette was opened from. An entry with
	// a non-zero view may therefore only read state that survives the switch
	// (len(m.queue.entries) > 0, m.tagReader != nil), never cursor or focus
	// state, which switchToView and the destination view's own state discard.
	enabled func(Model) bool
	pin     PalettePin
}

// paletteCatalog is the set of registry-backed palette commands. Every action
// bound in the registry must appear here or in paletteExcluded with a written
// reason; TestPaletteCatalogCoversRegistryActions enforces that.
var paletteCatalog = []paletteEntry{
	// Pinned anchors (Serial Position Effect): Help at top, Quit at bottom.
	{act: actionHelp, id: "help.show", name: "Show help", ctx: contextGlobal, pin: pinTop},
	{act: actionQuit, id: "app.quit", name: "Quit", ctx: contextGlobal, pin: pinBottom},

	{act: actionDebug, id: "debug.show", name: "Show debug panel", category: "dev", ctx: contextGlobal},
	{act: actionRadioToggle, id: "radio.toggle", name: "Toggle radio mode", category: "mode", ctx: contextGlobal},

	{act: actionRepeatCycle, id: "playback.repeat.cycle", name: "Cycle repeat mode", category: "mode", ctx: contextGlobal},
	{act: actionShuffleToggle, id: "playback.shuffle.toggle", name: "Toggle shuffle", category: "mode", ctx: contextGlobal},

	{act: actionPlayPause, id: "playback.toggle", name: "Play or pause", category: "play", ctx: contextGlobal},
	{act: actionStop, id: "playback.stop", name: "Stop playback", category: "play", ctx: contextGlobal},
	{act: actionNext, id: "playback.next", name: "Play next track", category: "play", ctx: contextGlobal},
	{act: actionPrevious, id: "playback.previous", name: "Play previous track", category: "play", ctx: contextGlobal},
	{act: actionStopAfterCurrent, id: "playback.stop_after_current", name: "Toggle stop after current", category: "play", ctx: contextGlobal},
	{act: actionVolumeUp, id: "playback.volume.up", name: "Raise volume", category: "play", ctx: contextGlobal},
	{act: actionVolumeDown, id: "playback.volume.down", name: "Lower volume", category: "play", ctx: contextGlobal},
	{act: actionSeekBack, id: "playback.seek.back", name: "Seek backward", category: "play", ctx: contextGlobal, enabled: playbackHasTrack},
	{act: actionSeekForward, id: "playback.seek.forward", name: "Seek forward", category: "play", ctx: contextGlobal, enabled: playbackHasTrack},
	{act: actionSeekBackLong, id: "playback.seek.back.long", name: "Seek backward (long)", category: "play", ctx: contextGlobal, enabled: playbackHasTrack},
	{act: actionSeekForwardLong, id: "playback.seek.forward.long", name: "Seek forward (long)", category: "play", ctx: contextGlobal, enabled: playbackHasTrack},
	// Named "Seek to start", not "Restart current track": the latter fuzzy-matches
	// the query "recent" at a word start and outranks "Go to Library: Recently
	// Added" for it.
	{act: actionResetPosition, id: "playback.seek.start", name: "Seek to start of track", category: "play", ctx: contextGlobal, enabled: playbackHasTrack},

	{act: actionSwitchPlaylists, id: "nav.playlists", name: "Go to Playlists", category: "nav", ctx: contextGlobal},
	{act: actionSwitchQueue, id: "nav.queue", name: "Go to Queue", category: "nav", ctx: contextGlobal},
	{act: actionSwitchFavorites, id: "nav.favorites", name: "Go to Favorites", category: "nav", ctx: contextGlobal},
	{act: actionSwitchTags, id: "nav.tags", name: "Go to Tags", category: "nav", ctx: contextGlobal},
	{act: actionSwitchMixes, id: "nav.mixes", name: "Go to Mixes", category: "nav", ctx: contextGlobal},
	{act: actionSwitchInsights, id: "nav.insights", name: "Go to Insights", category: "nav", ctx: contextGlobal},

	// tab and shift+tab are also bound to ] and [; preferKey advertises the
	// discoverable key instead of the bracket keysFor ranks first.
	{act: actionNextView, id: "nav.view.next", name: "Go to next view", category: "nav", ctx: contextGlobal, preferKey: "tab"},
	{act: actionPrevView, id: "nav.view.previous", name: "Go to previous view", category: "nav", ctx: contextGlobal, preferKey: "shift+tab"},
	{act: actionFocusNowPlaying, id: "queue.jump.nowplaying", name: "Jump to now-playing in queue", category: "nav", ctx: contextGlobal, enabled: playbackHasTrack},
	{act: actionGoTo, id: "nav.goto", name: "Go to artist, album or genre…", category: "nav", ctx: contextGlobal},

	{act: actionSortCycle, id: "sort.cycle", name: "Cycle sort field", category: "sort", ctx: contextGlobal},
	{act: actionSortAsc, id: "sort.ascending", name: "Sort ascending", category: "sort", ctx: contextGlobal},
	{act: actionSortDesc, id: "sort.descending", name: "Sort descending", category: "sort", ctx: contextGlobal},

	{act: actionSettings, id: "settings.open", name: "Open settings", category: "app", ctx: contextGlobal},
	{act: actionUndo, id: "app.undo", name: "Undo last change", category: "app", ctx: contextGlobal},
	{act: actionRescan, id: "library.rescan", name: "Rescan library", category: "lib", ctx: contextGlobal},
	{act: actionFilterBar, id: "view.filter", name: "Filter current view", category: "view", ctx: contextGlobal},

	{act: actionNowPlayingInfo, id: "playback.nowplaying.info", name: "Show now-playing info", category: "play", ctx: contextGlobal},
	{act: actionQueueSummary, id: "queue.summary", name: "Show queue summary", category: "queue", ctx: contextGlobal},
	{act: actionTagPicker, id: "tag.picker.open", name: "Manage tags for track…", category: "tag", ctx: contextGlobal},

	{act: actionPreview, id: "track.preview", name: "Preview focused track", category: "trk", ctx: contextGlobal, enabled: hasFocusedTrack},
	{act: actionTrackInfo, id: "track.info", name: "Show track info", category: "trk", ctx: contextGlobal, enabled: hasFocusedTrack},
	{act: actionEditMetadata, id: "track.metadata.edit", name: "Edit track metadata", category: "trk", ctx: contextGlobal, enabled: hasFocusedTrack},
	{act: actionToggleFavorite, id: "track.favorite.toggle", name: "Toggle favorite", category: "trk", ctx: contextGlobal, enabled: hasFocusedTrack},

	// Collection-targeting view-local actions: they operate on a whole
	// collection, not on the cursor row, so switching to the owning view first
	// cannot mutate a row the user never saw. Their gates read only state that
	// survives switchToView.
	//
	// The three create handlers also require their view's collection level,
	// which switchToView preserves rather than resets; handleCommandPaletteAction
	// drills the destination back out for them (atCollectionLevelFor).
	{act: actionClearQueue, id: "queue.clear", name: "Clear queue", category: "queue", ctx: contextQueue, view: viewQueue, enabled: queueHasEntries},
	{act: actionSaveQueueAsPlaylist, id: "queue.save_as_playlist", name: "Save queue as playlist", category: "queue", ctx: contextQueue, view: viewQueue, enabled: queueHasEntries},
	{act: actionPaste, id: "queue.paste", name: "Paste yanked tracks", category: "queue", ctx: contextQueue, view: viewQueue, enabled: func(m Model) bool { return len(m.yankBuffer) > 0 }},
	{act: actionPlaylistCreate, id: "playlist.create", name: "Create playlist", category: "list", ctx: contextPlaylists, view: viewPlaylists},
	{act: actionSmartPlaylistCreate, id: "playlist.smart.create", name: "Create smart playlist", category: "list", ctx: contextPlaylists, view: viewPlaylists},
	{act: actionTagCreate, id: "tag.create", name: "Create tag", category: "tag", ctx: contextTags, view: viewTags},

	// Cursor-row-targeting view-local actions: view 0 plus an active-view gate.
	// Switching first would run a destructive edit against whatever row the
	// destination view's cursor happens to sit on, so these stay listed and
	// searchable but render dimmed and are skipped by enter elsewhere.
	{act: actionDequeue, id: "queue.dequeue", name: "Remove track from queue", category: "queue", ctx: contextQueue, enabled: activeViewIs(viewQueue)},
	{act: actionMoveTrackUp, id: "queue.move.up", name: "Move track up", category: "queue", ctx: contextQueue, enabled: activeViewIs(viewQueue, viewPlaylists)},
	{act: actionMoveTrackDown, id: "queue.move.down", name: "Move track down", category: "queue", ctx: contextQueue, enabled: activeViewIs(viewQueue, viewPlaylists)},
	{act: actionYank, id: "queue.yank", name: "Copy tracks to yank buffer", category: "queue", ctx: contextQueue, enabled: activeViewIs(viewQueue)},
	{act: actionCut, id: "queue.cut", name: "Cut tracks to yank buffer", category: "queue", ctx: contextQueue, enabled: activeViewIs(viewQueue)},
	{act: actionLibraryEnqueue, id: "queue.enqueue.focused", name: "Enqueue focused or selected", category: "queue", ctx: contextLibrary, enabled: activeViewIs(viewLibrary, viewMixes)},
	{act: actionLibraryEnqueueReplace, id: "queue.enqueue.replace", name: "Replace queue with track", category: "queue", ctx: contextLibrary, enabled: activeViewIs(viewLibrary, viewBrowse, viewPlaylists, viewFavorites, viewTags)},
	{act: actionPlayNext, id: "queue.play_next", name: "Play focused track next", category: "queue", ctx: contextLibrary, enabled: activeViewIs(viewLibrary, viewBrowse, viewPlaylists, viewFavorites, viewTags)},
	{act: actionLibraryFilterClear, id: "view.filter.clear", name: "Clear view filter", category: "view", ctx: contextLibrary, enabled: activeViewIs(viewLibrary, viewQueue)},

	{act: actionPlaylistRename, id: "playlist.rename", name: "Rename playlist", category: "list", ctx: contextPlaylists, enabled: activeViewIs(viewPlaylists)},
	{act: actionPlaylistDelete, id: "playlist.delete", name: "Delete playlist", category: "list", ctx: contextPlaylists, enabled: atPlaylistListLevel},
	{act: actionPlaylistEnqueueAll, id: "playlist.enqueue_all", name: "Enqueue playlist tracks", category: "list", ctx: contextPlaylists, enabled: activeViewIs(viewPlaylists)},
	{act: actionSmartPlaylistEdit, id: "playlist.smart.edit", name: "Edit smart playlist rules", category: "list", ctx: contextPlaylists, enabled: activeViewIs(viewPlaylists)},

	{act: actionTagRename, id: "tag.rename", name: "Rename tag", category: "tag", ctx: contextTags, enabled: activeViewIs(viewTags)},
	{act: actionTagDelete, id: "tag.delete", name: "Delete tag", category: "tag", ctx: contextTags, enabled: activeViewIs(viewTags)},
	{act: actionTagEnqueueAll, id: "tag.enqueue_all", name: "Enqueue tag tracks", category: "tag", ctx: contextTags, enabled: activeViewIs(viewTags)},
	{act: actionTagMerge, id: "tag.merge", name: "Merge tag into another…", category: "tag", ctx: contextTags, enabled: activeViewIs(viewTags)},

	{act: actionMixEnqueueAll, id: "mix.enqueue_all", name: "Enqueue mix tracks", category: "mix", ctx: contextMixes, enabled: activeViewIs(viewMixes)},
	{act: actionSaveMixAsPlaylist, id: "mix.save_as_playlist", name: "Save mix as playlist", category: "mix", ctx: contextMixes, enabled: activeViewIs(viewMixes)},
	{act: actionMixRadioSeed, id: "mix.radio.seed", name: "Start radio from mix", category: "mix", ctx: contextMixes, enabled: activeViewIs(viewMixes)},
	{act: actionMixDismiss, id: "mix.dismiss", name: "Dismiss track from mix", category: "mix", ctx: contextMixes, enabled: activeViewIs(viewMixes)},
	{act: actionMixUndismiss, id: "mix.undismiss", name: "Undismiss track", category: "mix", ctx: contextMixes, enabled: activeViewIs(viewMixes)},

	{act: actionInsightsPanelNext, id: "insights.panel.next", name: "Next insights panel", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsPanelPrev, id: "insights.panel.previous", name: "Previous insights panel", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsWindowNext, id: "insights.window.next", name: "Next window or bucket size", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsWindowPrev, id: "insights.window.previous", name: "Previous window or bucket size", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsEnqueueAll, id: "insights.enqueue_all", name: "Enqueue tracks behind row", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsAntiRut, id: "insights.anti_rut", name: "Enqueue a rut-breaking set", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsRadioSeed, id: "insights.radio.seed", name: "Start radio from insight row", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsSavePlaylist, id: "insights.save_as_playlist", name: "Save insight tracks as playlist", category: "stats", ctx: contextInsights, enabled: activeViewIs(viewInsights)},
	{act: actionInsightsMergeGenre, id: "insights.merge_genre", name: "Merge genre spellings", category: "stats", ctx: contextInsights, enabled: insightsHasGenreCollision},
}

// playbackHasTrack reports whether a track is loaded, gating catalog entries
// that act on the current playback position and would otherwise be dispatched
// into a stopped player.
func playbackHasTrack(m Model) bool { return m.playback.currentHash != "" }

// hasFocusedTrack reports whether the cursor sits on a track row, gating
// catalog entries that act on the focused track.
func hasFocusedTrack(m Model) bool { _, _, _, ok := m.focusedTrackForPalette(); return ok }

// queueHasEntries reports whether the queue holds anything, gating the
// collection-wide queue entries whose handlers notify or no-op when it is empty.
func queueHasEntries(m Model) bool { return len(m.queue.entries) > 0 }

// atPlaylistListLevel gates the "Delete playlist" entry. handlePlaylistDelete
// is dual-purpose: at playlist track level it removes the focused or selected
// tracks, which the row's name does not advertise. handleCommandPaletteAction
// refuses the action under the same condition, so the dimmed row is backed by
// a real guard rather than by rendering alone.
func atPlaylistListLevel(m Model) bool {
	return m.activeView == viewPlaylists && m.playlists.atListLevel()
}

// activeViewIs builds the gate for a cursor-row entry: true only while one of
// views owns the screen. Listing the views the handler actually acts in — not
// just the one whose context supplies the shortcut — keeps the dimmed row
// honest for actions bound in several contexts, such as actionPlayNext.
// insightsHasGenreCollision gates the genre merge entry: the action only ever
// applies to a focused Data Health collision row, and offering it anywhere else
// would put a palette row in front of the user that can only notify a refusal.
func insightsHasGenreCollision(m Model) bool {
	row := m.insights.focusedHealthRow()
	return row != nil && row.kind == healthGenreCollision
}

func activeViewIs(views ...viewID) func(Model) bool {
	return func(m Model) bool { return slices.Contains(views, m.activeView) }
}

// paletteExcluded records actions that will never get a palette entry, with
// the reason each one is unusable or redundant there.
var paletteExcluded = map[action]string{
	actionNone: "sentinel value, not a dispatchable action",

	// Cursor and scroll motion: the palette closes before the action runs, so
	// moving the cursor by one row or one page has no observable effect.
	actionCursorUp:     "cursor motion is meaningless once the palette closes",
	actionCursorDown:   "cursor motion is meaningless once the palette closes",
	actionHalfPageUp:   "scroll motion is meaningless once the palette closes",
	actionHalfPageDown: "scroll motion is meaningless once the palette closes",
	actionFullPageUp:   "scroll motion is meaningless once the palette closes",
	actionFullPageDown: "scroll motion is meaningless once the palette closes",
	actionGoToTop:      "scroll motion is meaningless once the palette closes",
	actionGoToBottom:   "scroll motion is meaningless once the palette closes",

	// Drill in and out: the result depends entirely on the cursor row and the
	// drill level the palette covered up.
	actionSelect:        "acts on the cursor row, which the palette hides",
	actionArtistOpen:    "acts on the cursor row, which the palette hides",
	actionArtistBack:    "depends on the drill level, which the palette hides",
	actionAlbumOpen:     "acts on the cursor row, which the palette hides",
	actionAlbumBack:     "depends on the drill level, which the palette hides",
	actionGenreOpen:     "acts on the cursor row, which the palette hides",
	actionGenreBack:     "depends on the drill level, which the palette hides",
	actionFavoritesOpen: "acts on the cursor row, which the palette hides",
	actionFavoritesBack: "depends on the drill level, which the palette hides",
	actionPlaylistOpen:  "acts on the cursor row, which the palette hides",
	actionPlaylistBack:  "depends on the drill level, which the palette hides",
	actionTagOpen:       "acts on the cursor row, which the palette hides",
	actionTagBack:       "depends on the drill level, which the palette hides",
	actionMixOpen:       "acts on the cursor row, which the palette hides",
	actionMixBack:       "depends on the drill level, which the palette hides",

	actionInsightsOpen:       "acts on the cursor row, which the palette hides",
	actionInsightsBack:       "depends on the drill level, which the palette hides",
	actionInsightsJumpBrowse: "acts on the cursor row, which the palette hides",

	// Selection primitives: only meaningful part-way through a bulk-action
	// flow, which the palette interrupts.
	actionBulkSelectionToggle: "only meaningful mid bulk-selection flow",
	actionClearSelection:      "only meaningful mid bulk-selection flow",
	actionVisualMode:          "only meaningful mid bulk-selection flow",
	actionBulkAction:          "only meaningful mid bulk-selection flow",

	actionCommandPalette: "opens the palette itself",

	// Mode cycling and view switches whose destination depends on current
	// state: the deterministic nav.* entries land on a named destination
	// instead.
	actionLibraryModeNext: "superseded by the deterministic nav.library.* entries",
	actionLibraryModePrev: "superseded by the deterministic nav.library.* entries",
	actionBrowseModeNext:  "superseded by the deterministic nav.browse.* entries",
	actionBrowseModePrev:  "superseded by the deterministic nav.browse.* entries",
	actionSwitchLibrary:   "superseded by the deterministic nav.library.* entries",
	actionSwitchBrowse:    "superseded by the deterministic nav.browse.* entries",

	// Legacy sub-view switches, kept only for existing dispatch cases.
	actionSwitchArtists: "legacy sub-view switch, no longer bound to any key",
	actionSwitchAlbums:  "legacy sub-view switch, no longer bound to any key",
	actionSwitchGenres:  "legacy sub-view switch, no longer bound to any key",

	actionTagFilter: "covered by the hand-written \"Filter by tag\" palette entry",
}
