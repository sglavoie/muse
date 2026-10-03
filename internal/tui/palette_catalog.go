package tui

import (
	"slices"

	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/palette"
)

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
	act       keys.Action
	id        string // stable identifier for usage tracking
	name      string
	category  string       // one of paletteCategories
	ctx       keys.Context // owning context, used for the shortcut reverse-lookup
	view      viewID       // view to switch to before running; 0 runs in place
	preferKey string       // when set, overrides the keysFor ordering for display
	// enabled runs against the pre-switch model: the row is rendered while the
	// user is still in whatever view the palette was opened from. An entry with
	// a non-zero view may therefore only read state that survives the switch
	// (len(m.queue.entries) > 0, m.tagReader != nil), never cursor or focus
	// state, which switchToView and the destination view's own state discard.
	disabledReason string
	enabled        func(Model) bool
	pin            palette.PalettePin
}

// paletteCatalog is the set of registry-backed palette commands. Every action
// bound in the registry must appear here or in paletteExcluded with a written
// reason; TestPaletteCatalogCoversRegistryActions enforces that.
var paletteCatalog = []paletteEntry{
	// Pinned anchors (Serial Position Effect): Help at top, Quit at bottom.
	{act: keys.ActionHelp, id: "help.show", name: "Show help", ctx: keys.ContextGlobal, pin: palette.PinTop},
	{act: keys.ActionQuit, id: "app.quit", name: "Quit", ctx: keys.ContextGlobal, pin: palette.PinBottom},

	{act: keys.ActionDebug, id: "debug.show", name: "Show debug panel", category: "dev", ctx: keys.ContextGlobal},
	{act: keys.ActionRadioToggle, id: "radio.toggle", name: "Toggle radio mode", category: "mode", ctx: keys.ContextGlobal},
	{act: keys.ActionAutoDJTune, id: "autodj.tune", name: "Tune Auto-DJ…", category: "mode", ctx: keys.ContextGlobal},

	{act: keys.ActionRepeatCycle, id: "playback.repeat.cycle", name: "Cycle repeat mode", category: "mode", ctx: keys.ContextGlobal},
	{act: keys.ActionShuffleToggle, id: "playback.shuffle.toggle", name: "Toggle shuffle", category: "mode", ctx: keys.ContextGlobal},

	{act: keys.ActionPlayPause, id: "playback.toggle", name: "Play or pause", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionStop, id: "playback.stop", name: "Stop playback", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionNext, id: "playback.next", name: "Play next track", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionPrevious, id: "playback.previous", name: "Play previous track", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionStopAfterCurrent, id: "playback.stop_after_current", name: "Toggle stop after current", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionVolumeUp, id: "playback.volume.up", name: "Raise volume", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionVolumeDown, id: "playback.volume.down", name: "Lower volume", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionSeekBack, id: "playback.seek.back", name: "Seek backward", category: "play", ctx: keys.ContextGlobal, disabledReason: "Load a track first", enabled: playbackHasTrack},
	{act: keys.ActionSeekForward, id: "playback.seek.forward", name: "Seek forward", category: "play", ctx: keys.ContextGlobal, disabledReason: "Load a track first", enabled: playbackHasTrack},
	{act: keys.ActionSeekBackLong, id: "playback.seek.back.long", name: "Seek backward (long)", category: "play", ctx: keys.ContextGlobal, disabledReason: "Load a track first", enabled: playbackHasTrack},
	{act: keys.ActionSeekForwardLong, id: "playback.seek.forward.long", name: "Seek forward (long)", category: "play", ctx: keys.ContextGlobal, disabledReason: "Load a track first", enabled: playbackHasTrack},
	// Named "Seek to start", not "Restart current track": the latter fuzzy-matches
	// the query "recent" at a word start and outranks "Go to Library: Recently
	// Added" for it.
	{act: keys.ActionResetPosition, id: "playback.seek.start", name: "Seek to start of track", category: "play", ctx: keys.ContextGlobal, disabledReason: "Load a track first", enabled: playbackHasTrack},

	{act: keys.ActionSwitchPlaylists, id: "nav.playlists", name: "Go to Playlists", category: "nav", ctx: keys.ContextGlobal},
	{act: keys.ActionSwitchQueue, id: "nav.queue", name: "Go to Queue", category: "nav", ctx: keys.ContextGlobal},
	{act: keys.ActionSwitchFavorites, id: "nav.favorites", name: "Go to Favorites", category: "nav", ctx: keys.ContextGlobal},
	{act: keys.ActionSwitchTags, id: "nav.tags", name: "Go to Tags", category: "nav", ctx: keys.ContextGlobal},
	{act: keys.ActionSwitchMixes, id: "nav.mixes", name: "Go to Mixes", category: "nav", ctx: keys.ContextGlobal},
	{act: keys.ActionSwitchInsights, id: "nav.insights", name: "Go to Insights", category: "nav", ctx: keys.ContextGlobal},

	// tab and shift+tab are also bound to ] and [; preferKey advertises the
	// discoverable key instead of the bracket keysFor ranks first.
	{act: keys.ActionNextView, id: "nav.view.next", name: "Go to next view", category: "nav", ctx: keys.ContextGlobal, preferKey: "tab"},
	{act: keys.ActionPrevView, id: "nav.view.previous", name: "Go to previous view", category: "nav", ctx: keys.ContextGlobal, preferKey: "shift+tab"},
	{act: keys.ActionFocusNowPlaying, id: "queue.jump.nowplaying", name: "Jump to now-playing in queue", category: "nav", ctx: keys.ContextGlobal, disabledReason: "Load a track first", enabled: playbackHasTrack},
	{act: keys.ActionGoTo, id: "nav.goto", name: "Go to artist, album or genre…", category: "nav", ctx: keys.ContextGlobal},

	{act: keys.ActionSortCycle, id: "sort.cycle", name: "Cycle sort field", category: "sort", ctx: keys.ContextGlobal},
	{act: keys.ActionSortAsc, id: "sort.ascending", name: "Sort ascending", category: "sort", ctx: keys.ContextGlobal},
	{act: keys.ActionSortDesc, id: "sort.descending", name: "Sort descending", category: "sort", ctx: keys.ContextGlobal},

	{act: keys.ActionSettings, id: "settings.open", name: "Open settings", category: "app", ctx: keys.ContextGlobal},
	{act: keys.ActionUndo, id: "app.undo", name: "Undo last change", category: "app", ctx: keys.ContextGlobal},
	{act: keys.ActionRescan, id: "library.rescan", name: "Rescan library", category: "lib", ctx: keys.ContextGlobal},
	{act: keys.ActionFilterBar, id: "view.filter", name: "Filter current view", category: "view", ctx: keys.ContextGlobal},

	{act: keys.ActionNowPlayingInfo, id: "playback.nowplaying.info", name: "Show now-playing info", category: "play", ctx: keys.ContextGlobal},
	// "play" ("playback transport, volume and info"), not "mode": the legend only
	// explains the mode toggles, it never changes one.
	{act: keys.ActionModesLegend, id: "help.modes_legend", name: "Show playback mode legend", category: "play", ctx: keys.ContextGlobal},
	{act: keys.ActionQueueSummary, id: "queue.summary", name: "Show queue summary", category: "queue", ctx: keys.ContextGlobal},
	{act: keys.ActionTagPicker, id: "tag.picker.open", name: "Manage tags for track…", category: "tag", ctx: keys.ContextGlobal},

	{act: keys.ActionPreview, id: "track.preview", name: "Preview focused track", category: "trk", ctx: keys.ContextGlobal, disabledReason: "Focus a track first", enabled: hasFocusedTrack},
	{act: keys.ActionTrackInfo, id: "track.info", name: "Show track info", category: "trk", ctx: keys.ContextGlobal, disabledReason: "Focus a track first", enabled: hasFocusedTrack},
	{act: keys.ActionEditMetadata, id: "track.metadata.edit", name: "Edit track metadata", category: "trk", ctx: keys.ContextGlobal, disabledReason: "Focus a track first", enabled: hasFocusedTrack},
	{act: keys.ActionToggleFavorite, id: "track.favorite.toggle", name: "Toggle favorite", category: "trk", ctx: keys.ContextGlobal, disabledReason: "Focus a track first", enabled: hasFocusedTrack},

	// Collection-targeting view-local actions: they operate on a whole
	// collection, not on the cursor row, so switching to the owning view first
	// cannot mutate a row the user never saw. Their gates read only state that
	// survives switchToView.
	//
	// The three create handlers also require their view's collection level,
	// which switchToView preserves rather than resets; handleCommandPaletteAction
	// drills the destination back out for them (atCollectionLevelFor).
	{act: keys.ActionClearQueue, id: "queue.clear", name: "Clear queue", category: "queue", ctx: keys.ContextQueue, view: viewQueue, disabledReason: "Queue is empty", enabled: queueHasEntries},
	{act: keys.ActionSaveQueueAsPlaylist, id: "queue.save_as_playlist", name: "Save queue as playlist", category: "queue", ctx: keys.ContextQueue, view: viewQueue, disabledReason: "Queue is empty", enabled: queueHasEntries},
	{act: keys.ActionPaste, id: "queue.paste", name: "Paste yanked tracks", category: "queue", ctx: keys.ContextQueue, view: viewQueue, disabledReason: "Copy or cut tracks first", enabled: func(m Model) bool { return len(m.yankBuffer) > 0 }},
	{act: keys.ActionPlaylistCreate, id: "playlist.create", name: "Create playlist", category: "list", ctx: keys.ContextPlaylists, view: viewPlaylists},
	{act: keys.ActionSmartPlaylistCreate, id: "playlist.smart.create", name: "Create smart playlist", category: "list", ctx: keys.ContextPlaylists, view: viewPlaylists},
	{act: keys.ActionTagCreate, id: "tag.create", name: "Create tag", category: "tag", ctx: keys.ContextTags, view: viewTags},

	// Cursor-row-targeting view-local actions: view 0 plus an active-view gate.
	// Switching first would run a destructive edit against whatever row the
	// destination view's cursor happens to sit on, so these stay listed and
	// searchable but render dimmed and are skipped by enter elsewhere.
	{act: keys.ActionDequeue, id: "queue.dequeue", name: "Remove track from queue", category: "queue", ctx: keys.ContextQueue, disabledReason: "Open Queue first", enabled: activeViewIs(viewQueue)},
	{act: keys.ActionMoveTrackUp, id: "queue.move.up", name: "Move track up", category: "queue", ctx: keys.ContextQueue, disabledReason: "Open Queue or Playlists first", enabled: activeViewIs(viewQueue, viewPlaylists)},
	{act: keys.ActionMoveTrackDown, id: "queue.move.down", name: "Move track down", category: "queue", ctx: keys.ContextQueue, disabledReason: "Open Queue or Playlists first", enabled: activeViewIs(viewQueue, viewPlaylists)},
	{act: keys.ActionYank, id: "queue.yank", name: "Copy tracks to yank buffer", category: "queue", ctx: keys.ContextQueue, disabledReason: "Open Queue first", enabled: activeViewIs(viewQueue)},
	{act: keys.ActionCut, id: "queue.cut", name: "Cut tracks to yank buffer", category: "queue", ctx: keys.ContextQueue, disabledReason: "Open Queue first", enabled: activeViewIs(viewQueue)},
	{act: keys.ActionLibraryEnqueue, id: "queue.enqueue.focused", name: "Enqueue focused or selected", category: "queue", ctx: keys.ContextLibrary, disabledReason: "Open Library or Mixes first", enabled: activeViewIs(viewLibrary, viewMixes)},
	{act: keys.ActionLibraryEnqueueReplace, id: "queue.enqueue.replace", name: "Replace queue with track", category: "queue", ctx: keys.ContextLibrary, disabledReason: "Open Library, Browse, Playlists, Favorites or Tags", enabled: activeViewIs(viewLibrary, viewBrowse, viewPlaylists, viewFavorites, viewTags)},
	{act: keys.ActionPlayNext, id: "queue.play_next", name: "Play focused track next", category: "queue", ctx: keys.ContextLibrary, disabledReason: "Open Library, Browse, Playlists, Favorites or Tags", enabled: activeViewIs(viewLibrary, viewBrowse, viewPlaylists, viewFavorites, viewTags)},
	{act: keys.ActionLibraryFilterClear, id: "view.filter.clear", name: "Clear view filter", category: "view", ctx: keys.ContextLibrary, disabledReason: "No text filter to clear", enabled: Model.hasCommittedFilter},

	{act: keys.ActionPlaylistRename, id: "playlist.rename", name: "Rename playlist", category: "list", ctx: keys.ContextPlaylists, disabledReason: "Open Playlists first", enabled: activeViewIs(viewPlaylists)},
	{act: keys.ActionPlaylistDelete, id: "playlist.delete", name: "Delete playlist", category: "list", ctx: keys.ContextPlaylists, disabledReason: "Open the playlist list first", enabled: atPlaylistListLevel},
	{act: keys.ActionPlaylistEnqueueAll, id: "playlist.enqueue_all", name: "Enqueue playlist tracks", category: "list", ctx: keys.ContextPlaylists, disabledReason: "Open Playlists first", enabled: activeViewIs(viewPlaylists)},
	{act: keys.ActionSmartPlaylistEdit, id: "playlist.smart.edit", name: "Edit smart playlist rules", category: "list", ctx: keys.ContextPlaylists, disabledReason: "Open Playlists first", enabled: activeViewIs(viewPlaylists)},

	{act: keys.ActionTagRename, id: "tag.rename", name: "Rename tag", category: "tag", ctx: keys.ContextTags, disabledReason: "Open Tags first", enabled: activeViewIs(viewTags)},
	{act: keys.ActionTagDelete, id: "tag.delete", name: "Delete tag", category: "tag", ctx: keys.ContextTags, disabledReason: "Open Tags first", enabled: activeViewIs(viewTags)},
	{act: keys.ActionTagEnqueueAll, id: "tag.enqueue_all", name: "Enqueue tag tracks", category: "tag", ctx: keys.ContextTags, disabledReason: "Open Tags first", enabled: activeViewIs(viewTags)},
	{act: keys.ActionTagMerge, id: "tag.merge", name: "Merge tag into another…", category: "tag", ctx: keys.ContextTags, disabledReason: "Open Tags first", enabled: activeViewIs(viewTags)},

	{act: keys.ActionMixEnqueueAll, id: "mix.enqueue_all", name: "Enqueue mix tracks", category: "mix", ctx: keys.ContextMixes, disabledReason: "Open Mixes first", enabled: activeViewIs(viewMixes)},
	{act: keys.ActionSaveMixAsPlaylist, id: "mix.save_as_playlist", name: "Save mix as playlist", category: "mix", ctx: keys.ContextMixes, disabledReason: "Open Mixes first", enabled: activeViewIs(viewMixes)},
	{act: keys.ActionMixRadioSeed, id: "mix.radio.seed", name: "Start radio from mix", category: "mix", ctx: keys.ContextMixes, disabledReason: "Open Mixes first", enabled: activeViewIs(viewMixes)},
	{act: keys.ActionMixDismiss, id: "mix.dismiss", name: "Dismiss track from mix", category: "mix", ctx: keys.ContextMixes, disabledReason: "Open Mixes first", enabled: activeViewIs(viewMixes)},
	{act: keys.ActionMixUndismiss, id: "mix.undismiss", name: "Undismiss track", category: "mix", ctx: keys.ContextMixes, disabledReason: "Open Mixes first", enabled: activeViewIs(viewMixes)},

	{act: keys.ActionInsightsPanelNext, id: "insights.panel.next", name: "Next insights panel", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsPanelPrev, id: "insights.panel.previous", name: "Previous insights panel", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsWindowNext, id: "insights.window.next", name: "Next window or bucket size", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsWindowPrev, id: "insights.window.previous", name: "Previous window or bucket size", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsEnqueueAll, id: "insights.enqueue_all", name: "Enqueue tracks behind row", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsAntiRut, id: "insights.anti_rut", name: "Enqueue a rut-breaking set", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsRadioSeed, id: "insights.radio.seed", name: "Start radio from insight row", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsSavePlaylist, id: "insights.save_as_playlist", name: "Save insight tracks as playlist", category: "stats", ctx: keys.ContextInsights, disabledReason: "Open Insights first", enabled: activeViewIs(viewInsights)},
	{act: keys.ActionInsightsMergeGenre, id: "insights.merge_genre", name: "Merge genre spellings", category: "stats", ctx: keys.ContextInsights, disabledReason: "Select a genre collision in Data Health", enabled: insightsHasGenreCollision},
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
func queueHasEntries(m Model) bool { return len(m.queue.Entries) > 0 }

// atPlaylistListLevel gates the "Delete playlist" entry. handlePlaylistDelete
// is dual-purpose: at playlist track level it removes the focused or selected
// tracks, which the row's name does not advertise. handleCommandPaletteAction
// refuses the action under the same condition, so the dimmed row is backed by
// a real guard rather than by rendering alone.
func atPlaylistListLevel(m Model) bool {
	return m.ActiveView == viewPlaylists && m.playlists.AtListLevel()
}

// activeViewIs builds the gate for a cursor-row entry: true only while one of
// views owns the screen. Listing the views the handler actually acts in — not
// just the one whose context supplies the shortcut — keeps the dimmed row
// honest for actions bound in several contexts, such as actionPlayNext.
// insightsHasGenreCollision gates the genre merge entry: the action only ever
// applies to a focused Data Health collision row, and offering it anywhere else
// would put a palette row in front of the user that can only notify a refusal.
func insightsHasGenreCollision(m Model) bool {
	row := m.insights.FocusedHealthRow()
	return row != nil && row.Kind == instui.HealthGenreCollision
}

func activeViewIs(views ...viewID) func(Model) bool {
	return func(m Model) bool { return slices.Contains(views, m.ActiveView) }
}

// paletteExcluded records actions that will never get a palette entry, with
// the reason each one is unusable or redundant there.
var paletteExcluded = map[keys.Action]string{
	keys.ActionNone: "sentinel value, not a dispatchable action",

	// Cursor and scroll motion: the palette closes before the action runs, so
	// moving the cursor by one row or one page has no observable effect.
	keys.ActionCursorUp:     "cursor motion is meaningless once the palette closes",
	keys.ActionCursorDown:   "cursor motion is meaningless once the palette closes",
	keys.ActionHalfPageUp:   "scroll motion is meaningless once the palette closes",
	keys.ActionHalfPageDown: "scroll motion is meaningless once the palette closes",
	keys.ActionFullPageUp:   "scroll motion is meaningless once the palette closes",
	keys.ActionFullPageDown: "scroll motion is meaningless once the palette closes",
	keys.ActionGoToTop:      "scroll motion is meaningless once the palette closes",
	keys.ActionGoToBottom:   "scroll motion is meaningless once the palette closes",

	// Drill in and out: the result depends entirely on the cursor row and the
	// drill level the palette covered up.
	keys.ActionSelect:        "acts on the cursor row, which the palette hides",
	keys.ActionArtistOpen:    "acts on the cursor row, which the palette hides",
	keys.ActionArtistBack:    "depends on the drill level, which the palette hides",
	keys.ActionAlbumOpen:     "acts on the cursor row, which the palette hides",
	keys.ActionAlbumBack:     "depends on the drill level, which the palette hides",
	keys.ActionGenreOpen:     "acts on the cursor row, which the palette hides",
	keys.ActionGenreBack:     "depends on the drill level, which the palette hides",
	keys.ActionFavoritesOpen: "acts on the cursor row, which the palette hides",
	keys.ActionFavoritesBack: "depends on the drill level, which the palette hides",
	keys.ActionPlaylistOpen:  "acts on the cursor row, which the palette hides",
	keys.ActionPlaylistBack:  "depends on the drill level, which the palette hides",
	keys.ActionTagOpen:       "acts on the cursor row, which the palette hides",
	keys.ActionTagBack:       "depends on the drill level, which the palette hides",
	keys.ActionMixOpen:       "acts on the cursor row, which the palette hides",
	keys.ActionMixBack:       "depends on the drill level, which the palette hides",

	keys.ActionInsightsOpen:       "acts on the cursor row, which the palette hides",
	keys.ActionInsightsBack:       "depends on the drill level, which the palette hides",
	keys.ActionInsightsJumpBrowse: "acts on the cursor row, which the palette hides",

	// Selection primitives: only meaningful part-way through a bulk-action
	// flow, which the palette interrupts.
	keys.ActionBulkSelectionToggle: "only meaningful mid bulk-selection flow",
	keys.ActionClearSelection:      "only meaningful mid bulk-selection flow",
	keys.ActionVisualMode:          "only meaningful mid bulk-selection flow",
	keys.ActionBulkAction:          "only meaningful mid bulk-selection flow",

	keys.ActionCommandPalette: "opens the palette itself",

	// Mode cycling and view switches whose destination depends on current
	// state: the deterministic nav.* entries land on a named destination
	// instead.
	keys.ActionLibraryModeNext: "superseded by the deterministic nav.library.* entries",
	keys.ActionLibraryModePrev: "superseded by the deterministic nav.library.* entries",
	keys.ActionBrowseModeNext:  "superseded by the deterministic nav.browse.* entries",
	keys.ActionBrowseModePrev:  "superseded by the deterministic nav.browse.* entries",
	keys.ActionSwitchLibrary:   "superseded by the deterministic nav.library.* entries",
	keys.ActionSwitchBrowse:    "superseded by the deterministic nav.browse.* entries",

	// Legacy sub-view switches, kept only for existing dispatch cases.
	keys.ActionSwitchArtists: "legacy sub-view switch, no longer bound to any key",
	keys.ActionSwitchAlbums:  "legacy sub-view switch, no longer bound to any key",
	keys.ActionSwitchGenres:  "legacy sub-view switch, no longer bound to any key",

	keys.ActionTagFilter: "covered by the hand-written \"Filter by tag\" palette entry",
}
