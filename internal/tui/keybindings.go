package tui

// Keymap consistency matrix (Session 9 audit).
//
// Each row shows the action a key resolves to in each view context. "=" means
// the global binding pass-through (no view-local override). "—" means no
// binding (resolve returns actionNone). Capitals matter — j != J. Navigation
// keys (j k g G ctrl+d ctrl+u ctrl+f ctrl+b up down) are registered uniformly
// per context via registerNavigation and behave identically everywhere.
//
//   Key | Global         | Library    | Playlists  | Queue      | Browse(A/B/G) | Favorites  | Tags       | Mixes
//   ----+----------------+------------+------------+------------+---------------+------------+------------+------------
//   /   | filter-bar     | =          | =          | =          | =             | =          | =          | =
//   :   | palette        | =          | =          | =          | =             | =          | =          | =
//   ?   | help           | =          | =          | =          | =             | =          | =          | =
//   esc | —              | filter-clr | back       | filter-clr | back          | back       | back       | back
//   enter | —            | enqueue    | open       | play       | open          | open       | tag-open   | open
//   shift+enter | —      | replace    | replace    | —          | replace       | replace    | —          | —
//   space | play/pause   | =          | =          | =          | =             | =          | =          | =
//   tab | next-view      | =          | =          | =          | =             | =          | =          | =
//   1-7 | switch-view    | =          | =          | =          | =             | =          | =          | =
//   n / p | next/prev    | =          | =          | =          | =             | =          | =          | =
//   N   | —              | play-next  | play-next  | —          | play-next     | play-next  | play-next  | play-next
//   s   | stop           | =          | =          | =          | =             | =          | =          | sort-cycle*
//   S   | —              | —          | smart-new  | save-q     | —             | —          | —          | save-as-pl
//   d   | —              | —          | delete     | dequeue    | —             | —          | tag-delete | —
//   c   | —              | —          | create     | —          | —             | —          | tag-create | —
//   e   | —              | —          | smart-edit | —          | —             | —          | —          | enqueue
//   E   | —              | —          | enqueue-all| —          | —             | —          | enqueue-all| enqueue-all
//   m   | —              | —          | —          | —          | —             | —          | merge      | —
//   r   | repeat-cycle   | =          | rename*    | =          | =             | =          | rename*    | =
//   R   | reset-pos      | =          | =          | =          | =             | =          | =          | =
//   z   | shuffle        | =          | =          | =          | =             | =          | =          | =
//   u   | undo           | =          | =          | =          | =             | =          | =          | =
//   F   | toggle-fav     | =          | =          | =          | =             | =          | = (rebind) | =
//   A   | radio-toggle   | =          | =          | =          | =             | =          | =          | seed-radio*
//   o   | go-to          | =          | =          | =          | =             | =          | =          | =
//   t   | tag-picker     | =          | =          | =          | =             | =          | =          | =
//   f   | focus-np       | =          | =          | =          | =             | =          | =          | =
//   q   | queue-summary  | =          | =          | =          | =             | =          | =          | =
//   P   | preview        | =          | =          | =          | =             | =          | =          | =
//   M   | edit-metadata  | =          | =          | =          | =             | =          | =          | =
//   I   | now-playing-info | =        | =          | =          | =             | =          | =          | =
//   i   | track-info     | =          | =          | =          | =             | =          | =          | =
//   L   | listening-stats| =          | =          | =          | =             | =          | =          | =
//   v   | —              | visual     | visual     | visual     | visual        | visual     | visual     | visual
//   x   | —              | bulk-sel   | bulk-sel   | bulk-sel   | bulk-sel      | bulk-sel   | bulk-sel   | mix-dismiss*
//   X   | —              | clear-sel  | clear-sel  | clear-sel  | clear-sel     | clear-sel  | clear-sel  | mix-undism*
//   a   | —              | bulk-act   | bulk-act   | —          | bulk-act      | bulk-act   | bulk-act   | bulk-act
//   K/J | —              | —          | move-trk   | move-trk   | —             | —          | —          | —
//   C   | —              | —          | —          | clear-q    | —             | —          | —          | —
//   ` / ~ | —            | mode-cycle | —          | —          | mode-cycle    | —          | —          | —
//   h, backspace | —     | —          | —          | —          | —             | —          | tag-back   | —
//   ctrl+y/x/p | —       | —          | —          | yank/cut/paste | —         | —          | —          | —
//
// Entries marked "*" are intentional shadows of a global binding with a
// different action. Each must be registered via registerWithReason so the
// TestNoSilentlyShadowedGlobalBindings test passes — the domain reason is
// surfaced as the override's justification. Same-action rebinds (Tags F)
// are treated as harmless redundancy and not flagged.

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
	contextFavorites                       // applies only in the Favorites view
	contextGenres                          // applies only in the Genres view
	contextTags                            // applies only in the Tags view
	contextModals                          // applies only when a modal is active
	contextMixes                           // applies only in the Mixes view
	contextBrowse                          // applies only in the Browse view (fallback; real sub-context resolved dynamically)
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
	actionResetPosition
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
	actionRepeatCycle      // 'r': cycle repeat mode off→all→one→off
	actionShuffleToggle    // 'z': toggle shuffle mode
	actionUndo             // 'u': dispatch undo
	actionFilterBar        // '/': activate filter bar in active view
	actionHelp             // '?': show help overlay
	actionSettings         // 'ctrl+,': open settings overlay
	actionDebug            // 'ctrl+g': show debug panel
	actionStats            // 'L': show listening stats overlay
	actionNextView         // tab: cycle to next view
	actionPrevView         // shift+tab: cycle to previous view
	actionPreview          // 'P': preview focused track
	actionNowPlayingInfo   // 'I': show now-playing info overlay
	actionEditMetadata     // 'M': edit metadata of focused track
	actionTrackInfo        // 'i': show track info overlay (exiftool)
	actionStopAfterCurrent // ctrl+n: toggle "stop after current track"
	actionQueueSummary     // 'q': show queue summary overlay
	actionCommandPalette   // ':': open command palette
	actionTagPicker        // 't': open tag picker overlay
	actionTagFilter        // ctrl+t: open/clear cross-view tag filter
	actionRadioToggle      // 'A': toggle radio / auto-DJ mode
	actionFocusNowPlaying  // 'f': jump cursor to the currently playing track

	actionSwitchBrowse
	actionSwitchArtists // legacy — no longer bound to a key; kept for existing dispatch cases
	actionSwitchAlbums  // legacy — no longer bound to a key; kept for existing dispatch cases
	actionSwitchFavorites
	actionToggleFavorite
	actionLibraryModeNext // '`': cycle Library sub-mode forward (All → Recent → History → All)
	actionLibraryModePrev // '~': cycle Library sub-mode backward
	actionBrowseModeNext  // '`': cycle Browse sub-mode forward (Artists → Albums → Genres → Artists)
	actionBrowseModePrev  // '~': cycle Browse sub-mode backward
	actionGoTo            // 'o': open Go To navigation popup

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
	actionVisualMode          // 'v': toggle visual line selection mode
	actionClearQueue          // 'C': clear all entries from the queue
	actionMoveTrackUp         // 'K': move selected track one position earlier
	actionMoveTrackDown       // 'J': move selected track one position later
	actionSaveQueueAsPlaylist // 'S': save entire queue as a new playlist
	actionYank                // ctrl+y: copy focused/selected tracks to yank buffer
	actionPaste               // ctrl+p: paste yanked tracks below cursor
	actionCut                 // ctrl+x: cut focused/selected tracks (move on paste)

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
	actionSwitchGenres // legacy — no longer bound to a key; kept for existing dispatch cases
	actionGenreOpen    // enter: drill into artists/tracks or enqueue
	actionGenreBack    // esc: go back one level

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

	// Tags view actions.
	actionSwitchTags    // '6': switch to Tags view
	actionTagOpen       // enter: drill into tag tracks (or enqueue at TrackLevel)
	actionTagBack       // backspace/h: go back to tag list
	actionTagCreate     // 'c': create a new tag (inline input)
	actionTagRename     // 'r': rename the focused tag (inline input)
	actionTagDelete     // 'd': soft-delete the focused tag (5s undo)
	actionTagEnqueueAll // 'E': enqueue all tracks with the focused tag
	actionTagMerge      // 'm': merge focused tag into another (opens merge picker)

	// Mixes-local actions.
	actionSwitchMixes       // '7': switch to Mixes view
	actionMixOpen           // enter: drill into mix track list
	actionMixBack           // esc: go back from track list to mix list
	actionMixEnqueueAll     // 'E': enqueue all tracks from the focused mix
	actionSaveMixAsPlaylist // 'S': save current mix as a static playlist
	actionMixRadioSeed      // 'A': start radio seeded from the focused mix's aggregate metadata
	actionMixDismiss        // 'x': dismiss focused track from recommendations
	actionMixUndismiss      // 'X': undismiss focused track
)

// registryKey is the composite map key for registry lookups.
type registryKey struct {
	ctx bindingContext
	key string
}

// primaryHint is a single (key, label) pair shown in the footer hint strip.
// Labels are short verb-phrases (e.g. "nav", "enqueue") tuned for the
// 4–6 most relevant bindings per context.
type primaryHint struct {
	Key   string
	Label string
}

// HintMode selects which primary-hint set the footer strip should show.
// Selection and Visual modes surface action sets relevant to an in-progress
// bulk-action workflow; Default is the resting-state set.
type HintMode int

const (
	HintModeDefault   HintMode = iota // resting-state hints (no selection, no visual mode)
	HintModeSelection                 // one or more rows are bulk-selected
	HintModeVisual                    // visual line-selection mode is active
)

// primaryHintKey is the composite map key for per-context, per-mode hint sets.
type primaryHintKey struct {
	ctx  bindingContext
	mode HintMode
}

// Registry maps (context, key) → action. The zero value is not usable;
// use newRegistry to obtain a pre-populated instance.
//
// domainReasons records the justification for a view-local override that
// shadows a global binding with a different action. The audit test
// (TestNoSilentlyShadowedGlobalBindings) requires every such override to
// have a non-empty reason here, so any silently shadowed key fails CI.
type Registry struct {
	bindings      map[registryKey]action
	primaries     map[primaryHintKey][]primaryHint
	domainReasons map[registryKey]string
}

// Primaries returns the ordered list of default-mode primary keybinding
// hints for ctx. Equivalent to PrimariesForMode(ctx, HintModeDefault).
func (r Registry) Primaries(ctx bindingContext) []primaryHint {
	return r.primaries[primaryHintKey{ctx: ctx, mode: HintModeDefault}]
}

// PrimariesForMode returns the primary hints for (ctx, mode). When mode
// is non-default and no hints are registered for it, falls back to the
// default-mode set so the footer never goes empty just because a mode-
// specific set wasn't customized for this view.
func (r Registry) PrimariesForMode(ctx bindingContext, mode HintMode) []primaryHint {
	if hints, ok := r.primaries[primaryHintKey{ctx: ctx, mode: mode}]; ok && len(hints) > 0 {
		return hints
	}
	return r.primaries[primaryHintKey{ctx: ctx, mode: HintModeDefault}]
}

// newRegistry returns a Registry pre-populated with the application's default
// key bindings.
func newRegistry() Registry {
	r := Registry{
		bindings:      make(map[registryKey]action),
		primaries:     make(map[primaryHintKey][]primaryHint),
		domainReasons: make(map[registryKey]string),
	}

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

	// Library-local bindings.
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

	// Queue-local bindings.
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

	// Playlists-local bindings.
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

	// Artists-local bindings.
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

	// Albums-local bindings.
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

	// Genres-local bindings.
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

	// Tags-local bindings.
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

	// Mixes-local bindings.
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

	// Favorites-local bindings.
	r.registerNavigation(contextFavorites)
	r.register(contextFavorites, "enter", actionFavoritesOpen)
	r.register(contextFavorites, "esc", actionFavoritesBack)
	r.register(contextFavorites, "N", actionPlayNext)
	r.register(contextFavorites, "x", actionBulkSelectionToggle)
	r.register(contextFavorites, "X", actionClearSelection)
	r.register(contextFavorites, "v", actionVisualMode)
	r.register(contextFavorites, "a", actionBulkAction)
	r.register(contextFavorites, "shift+enter", actionLibraryEnqueueReplace)

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

// registerWithReason registers a view-local binding that intentionally
// shadows a global binding with a different action. The reason is recorded
// in domainReasons; TestNoSilentlyShadowedGlobalBindings asserts that any
// shadowed key has a non-empty reason.
func (r *Registry) registerWithReason(ctx bindingContext, key string, a action, reason string) {
	r.register(ctx, key, a)
	r.domainReasons[registryKey{ctx: ctx, key: key}] = reason
}

// registerPrimary binds the action and also records (key, label) as a primary
// hint for the footer hint strip in the given context (default mode). Order of
// calls is the order of display.
func (r *Registry) registerPrimary(ctx bindingContext, key string, a action, label string) {
	r.register(ctx, key, a)
	r.addPrimary(ctx, key, label)
}

// addPrimary records (key, label) as a default-mode primary hint without
// (re)binding the key. Use this when the binding already exists in another
// context (e.g. a global binding that we want to surface in a view-local
// hint strip).
func (r *Registry) addPrimary(ctx bindingContext, key, label string) {
	r.addPrimaryMode(ctx, HintModeDefault, key, label)
}

// addPrimaryMode records (key, label) as a primary hint for the given
// (context, mode) pair without (re)binding the key. Used to surface
// selection-aware and visual-mode hint sets in the footer strip.
func (r *Registry) addPrimaryMode(ctx bindingContext, mode HintMode, key, label string) {
	k := primaryHintKey{ctx: ctx, mode: mode}
	r.primaries[k] = append(r.primaries[k], primaryHint{Key: key, Label: label})
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
