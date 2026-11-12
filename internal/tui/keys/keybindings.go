package keys

// Keymap consistency matrix (Session 9 audit).
//
// Each row shows the action a key resolves to in each view context. "=" means
// the global binding pass-through (no view-local override). "—" means no
// binding (Resolve returns ActionNone). Capitals matter — j != J. Navigation
// keys (j k g G ctrl+d ctrl+u ctrl+f ctrl+b up down) are registered uniformly
// per context via registerNavigation and behave identically everywhere.
//
//   Key | Global         | Library    | Playlists  | Queue      | Browse(A/B/G) | Favorites  | Tags       | Mixes       | Insights
//   ----+----------------+------------+------------+------------+---------------+------------+------------+-------------+------------
//   /   | filter-bar     | =          | =          | =          | =             | =          | =          | =           | =
//   :   | palette        | =          | =          | =          | =             | =          | =          | =           | =
//   ?   | help           | =          | =          | =          | =             | =          | =          | =           | =
//   esc | —              | filter-clr | back       | filter-clr | back          | back       | back       | back        | back
//   enter | —            | enqueue    | open       | play       | open          | open       | tag-open   | open        | open
//   shift+enter | —      | replace    | replace    | —          | replace       | replace    | —          | —           | —
//   space | play/pause   | =          | =          | =          | =             | =          | =          | =           | =
//   tab, ] | next-view   | =          | =          | =          | =             | =          | =          | =           | =
//   shift+tab, [ | prev-view | =      | =          | =          | =             | =          | =          | =           | =
//   1-8 | switch-view    | =          | =          | =          | =             | =          | =          | =           | =
//   n / p | next/prev    | =          | =          | =          | =             | =          | =          | =           | =
//   N   | —              | play-next  | play-next  | —          | play-next     | play-next  | play-next  | play-next   | —
//   s   | stop           | =          | =          | =          | =             | =          | =          | sort-cycle* | =
//   S   | —              | —          | smart-new  | save-q     | —             | —          | —          | save-as-pl  | save-as-pl
//   d   | —              | —          | delete     | dequeue    | —             | —          | tag-delete | mix-dismiss | —
//   D   | —              | —          | —          | —          | —             | —          | —          | mix-restore | —
//   c   | —              | —          | create     | —          | —             | —          | tag-create | —           | —
//   e   | —              | —          | smart-edit | —          | —             | —          | —          | enqueue     | enqueue
//   E   | —              | —          | enqueue-all| —          | —             | —          | enqueue-all| enqueue-all | enqueue-all
//   m   | —              | —          | —          | —          | —             | —          | merge      | —           | —
//   r   | repeat-cycle   | =          | rename*    | =          | =             | =          | rename*    | =           | =
//   R   | reset-pos      | =          | =          | =          | =             | =          | =          | =           | =
//   z   | shuffle        | =          | =          | =          | =             | =          | =          | =           | =
//   u   | undo           | =          | =          | =          | =             | =          | =          | =           | =
//   F   | toggle-fav     | =          | =          | =          | =             | =          | = (rebind) | =           | =
//   A   | radio-toggle   | =          | =          | =          | =             | =          | =          | seed-radio* | seed-radio*
//   o   | go-to          | =          | =          | =          | =             | =          | =          | =           | =
//   t   | tag-picker     | =          | =          | =          | =             | =          | =          | =           | =
//   f   | focus-np       | =          | =          | =          | =             | =          | =          | =           | =
//   q   | queue-summary  | =          | =          | =          | =             | =          | =          | =           | =
//   ;   | modes-legend   | =          | =          | =          | =             | =          | =          | =           | =
//   P   | preview        | =          | =          | =          | =             | =          | =          | =           | =
//   M   | edit-metadata  | =          | =          | =          | =             | =          | =          | =           | =
//   I   | now-playing-info | =        | =          | =          | =             | =          | =          | =           | =
//   i   | track-info     | =          | =          | =          | =             | =          | =          | =           | =
//   L   | listening-stats| =          | =          | =          | =             | =          | =          | =           | =
//   v   | —              | visual     | visual     | visual     | visual        | visual     | visual     | visual      | visual
//   x   | —              | bulk-sel   | bulk-sel   | bulk-sel   | bulk-sel      | bulk-sel   | bulk-sel   | bulk-sel    | bulk-sel
//   X   | —              | clear-sel  | clear-sel  | clear-sel  | clear-sel     | clear-sel  | clear-sel  | clear-sel   | clear-sel
//   a   | —              | bulk-act   | bulk-act   | —          | bulk-act      | bulk-act   | bulk-act   | bulk-act    | bulk-act
//   K/J | —              | —          | move-trk   | move-trk   | —             | —          | —          | —           | —
//   C   | —              | —          | —          | clear-q    | —             | —          | —          | —           | —
//   ` / ~ | —            | mode-cycle | —          | —          | mode-cycle    | —          | —          | —           | panel-cycle
//   c   | —              | —          | create     | —          | grid/list†    | —          | tag-create | —           | —
//   h/l | —              | —          | —          | —          | grid-move†    | —          | tag-back(h)| —           | —
//   w / W | —            | —          | —          | —          | —             | —          | —          | —           | window-cycle
//   b   | —              | —          | —          | —          | —             | —          | —          | —           | jump-browse
//   !   | —              | —          | —          | —          | —             | —          | —          | —           | anti-rut
//   h, backspace | —     | —          | —          | —          | —             | —          | tag-back   | —           | —
//   ctrl+y/x/p | —       | —          | —          | yank/cut/paste | —         | —          | —          | —           | —
//   ctrl+a | autodj-tune | =          | =          | =          | =             | =          | =          | =           | =
//   ctrl+o | jump-back   | =          | =          | =          | =             | =          | =          | =           | =
//   ctrl+i | jump-fwd    | =          | =          | =          | =             | =          | =          | =           | =
//
// Entries marked "*" are intentional shadows of a global binding with a
// different action. Each must be registered via registerWithReason so the
// TestNoSilentlyShadowedGlobalBindings test passes — the domain reason is
// surfaced as the override's justification. Same-action rebinds (Tags F)
// are treated as harmless redundancy and not flagged.
//
// † Albums only. left/right also move between covers in the album grid
// (shadowing seek, marked with a reason); in the album list the root model
// resolves them, and h/l, through the global bindings instead.
//
// Mixes s is sort-cycle only once a mix is open; on the mix list the root
// model resolves it through the global binding (stop) instead.

// Context identifies the context in which key bindings are resolved.
// The registry checks the view-local context first, then falls back to Global.
type Context int

const (
	ContextGlobal    Context = iota // applies in every view
	ContextLibrary                  // applies only in the Library view
	ContextPlaylists                // applies only in the Playlists view
	ContextQueue                    // applies only in the Queue view
	ContextArtists                  // applies only in the Artists view
	ContextAlbums                   // applies only in the Albums view
	ContextFavorites                // applies only in the Favorites view
	ContextGenres                   // applies only in the Genres view
	ContextTags                     // applies only in the Tags view
	ContextModals                   // applies only when a modal is active
	ContextMixes                    // applies only in the Mixes view
	ContextBrowse                   // applies only in the Browse view (fallback; real sub-context resolved dynamically)
	ContextInsights                 // applies only in the Insights view
)

// Action represents a named TUI action that a key binding triggers.
type Action int

const (
	ActionNone Action = iota

	// Global actions — available in every view unless a view-local binding
	// shadows the same key.
	ActionQuit
	ActionPlayPause
	ActionStop
	ActionResetPosition
	ActionNext
	ActionPrevious
	ActionSeekBack
	ActionSeekForward
	ActionSeekBackLong
	ActionSeekForwardLong
	ActionVolumeUp
	ActionVolumeDown
	ActionSwitchLibrary
	ActionSwitchPlaylists
	ActionSwitchQueue
	ActionRescan
	ActionRepeatCycle      // 'r': cycle repeat mode off→all→one→off
	ActionShuffleToggle    // 'z': toggle shuffle mode
	ActionUndo             // 'u': dispatch undo
	ActionFilterBar        // '/': activate filter bar in active view
	ActionHelp             // '?': show help overlay
	ActionSettings         // 'ctrl+,': open settings overlay
	ActionDebug            // 'ctrl+g': show debug panel
	ActionNextView         // tab: cycle to next view
	ActionPrevView         // shift+tab: cycle to previous view
	ActionPreview          // 'P': preview focused track
	ActionNowPlayingInfo   // 'I': show now-playing info overlay
	ActionEditMetadata     // 'M': edit metadata of focused track
	ActionTrackInfo        // 'i': show track info overlay (exiftool)
	ActionStopAfterCurrent // ctrl+n: toggle "stop after current track"
	ActionQueueSummary     // 'q': show queue summary overlay
	ActionModesLegend      // ';': show the playback-mode legend overlay
	ActionCommandPalette   // ':': open command palette
	ActionTagPicker        // 't': open tag picker overlay
	ActionTagFilter        // ctrl+t: open/clear cross-view tag filter
	ActionRadioToggle      // 'A': toggle radio / auto-DJ mode
	ActionFocusNowPlaying  // 'f': jump cursor to the currently playing track

	ActionSwitchBrowse
	ActionSwitchArtists // legacy — no longer bound to a key; kept for existing dispatch cases
	ActionSwitchAlbums  // legacy — no longer bound to a key; kept for existing dispatch cases
	ActionSwitchFavorites
	ActionToggleFavorite
	ActionLibraryModeNext // '`': cycle Library sub-mode forward (All → Recent → History → All)
	ActionLibraryModePrev // '~': cycle Library sub-mode backward
	ActionBrowseModeNext  // '`': cycle Browse sub-mode forward (Artists → Albums → Genres → Artists)
	ActionBrowseModePrev  // '~': cycle Browse sub-mode backward
	ActionGoTo            // 'o': open Go To navigation popup

	// Favorites-local actions.
	ActionFavoritesOpen // enter: drill into category/list or enqueue track
	ActionFavoritesBack // esc: go back one level

	// Library-local actions.
	ActionLibraryEnqueue        // enter: append the focused or selected tracks to the play queue
	ActionLibraryEnqueueReplace // shift+enter: replace queue with selected or focused tracks
	ActionLibraryFilterClear    // esc: clear a committed filter in the library view
	ActionPlayNext              // 'N': insert selected or focused tracks after the current track

	// Queue-local actions.
	ActionCursorUp
	ActionCursorDown
	ActionSelect
	ActionDequeue             // 'd': remove the selected track from the queue
	ActionBulkSelectionToggle // 'x': toggle bulk-selection on the selected entry
	ActionClearSelection      // 'X': clear all bulk-selected entries
	ActionVisualMode          // 'v': toggle visual line selection mode
	ActionClearQueue          // 'C': clear all entries from the queue
	ActionMoveTrackUp         // 'K': move selected track one position earlier
	ActionMoveTrackDown       // 'J': move selected track one position later
	ActionSaveQueueAsPlaylist // 'S': save entire queue as a new playlist
	ActionYank                // ctrl+y: copy focused/selected tracks to yank buffer
	ActionPaste               // ctrl+p: paste yanked tracks below cursor
	ActionCut                 // ctrl+x: cut focused/selected tracks (move on paste)

	// Playlists-local actions.
	ActionPlaylistRename
	ActionPlaylistCreate // 'c': create new playlist
	ActionPlaylistOpen   // 'enter': open playlist to display its tracks
	ActionPlaylistBack   // 'esc': return to the playlist list from track level
	ActionPlaylistDelete // 'd': delete focused playlist (list level) or remove focused track (track level)

	ActionPlaylistEnqueueAll // 'E': enqueue all tracks from the focused playlist

	// Smart-playlist actions (keybindings wired in Session 3).
	ActionSmartPlaylistCreate // 'S': create a new smart playlist
	ActionSmartPlaylistEdit   // 'e': edit rules for the focused smart playlist

	// Artists-local actions.
	ActionArtistOpen // enter: drill into albums/tracks or enqueue
	ActionArtistBack // esc: go back one level

	// Albums-local actions.
	ActionAlbumOpen // enter: drill into tracks or enqueue
	ActionAlbumBack // esc: go back one level

	// Genres-local actions.
	ActionSwitchGenres // legacy — no longer bound to a key; kept for existing dispatch cases
	ActionGenreOpen    // enter: drill into artists/tracks or enqueue
	ActionGenreBack    // esc: go back one level

	// Bulk-selection action — triggered by 'a' when tracks are selected.
	// ActionBulkAction applies in Library (append selected tracks to queue).
	ActionBulkAction // 'a': apply context-appropriate bulk action and clear selection

	// Vim-style scrolling actions — shared across Library, Queue, and Playlists.
	ActionHalfPageDown // ctrl+d: move cursor half a page down
	ActionHalfPageUp   // ctrl+u: move cursor half a page up
	ActionFullPageDown // ctrl+f: move cursor a full page down
	ActionFullPageUp   // ctrl+b: move cursor a full page up
	ActionGoToTop      // g: move cursor to the first row
	ActionGoToBottom   // G: move cursor to the last row

	// Sort actions.
	ActionSortCycle // ctrl+s: cycle to next sort field
	ActionSortAsc   // shift+up: set sort direction to ascending
	ActionSortDesc  // shift+down: set sort direction to descending

	// Tags view actions.
	ActionSwitchTags    // '6': switch to Tags view
	ActionTagOpen       // enter: drill into tag tracks (or enqueue at TrackLevel)
	ActionTagBack       // backspace/h: go back to tag list
	ActionTagCreate     // 'c': create a new tag (inline input)
	ActionTagRename     // 'r': rename the focused tag (inline input)
	ActionTagDelete     // 'd': soft-delete the focused tag (5s undo)
	ActionTagEnqueueAll // 'E': enqueue all tracks with the focused tag
	ActionTagMerge      // 'm': merge focused tag into another (opens merge picker)

	// Mixes-local actions.
	ActionSwitchMixes       // '7': switch to Mixes view
	ActionMixOpen           // enter: open mix or enqueue selected/focused tracks
	ActionMixBack           // esc: go back from track list to mix list
	ActionMixEnqueueAll     // 'E': enqueue all tracks from the focused mix
	ActionSaveMixAsPlaylist // 'S': save current mix as a static playlist
	ActionMixRadioSeed      // 'A': start radio seeded from the focused mix's aggregate metadata
	ActionMixDismiss        // 'd': dismiss focused track from recommendations
	ActionMixUndismiss      // 'D': restore the last successful dismissal

	// Insights-local actions.
	ActionSwitchInsights       // '8': switch to Insights view
	ActionInsightsPanelNext    // '`': cycle to the next panel
	ActionInsightsPanelPrev    // '~': cycle to the previous panel
	ActionInsightsWindowNext   // 'w': advance the panel's window or bucket size
	ActionInsightsWindowPrev   // 'W': step the panel's window or bucket size back
	ActionInsightsOpen         // enter: drill into the focused row's tracks
	ActionInsightsBack         // esc: return from the drill-down to the panel
	ActionInsightsEnqueueAll   // 'E': enqueue every track behind the focused row
	ActionInsightsAntiRut      // '!': enqueue unheard tracks from the focused shelf
	ActionInsightsRadioSeed    // 'A': start radio seeded from the focused row
	ActionInsightsJumpBrowse   // 'b': jump to the focused row's artist in Browse
	ActionInsightsSavePlaylist // 'S': save the drilled-in tracks as a playlist
	ActionInsightsMergeGenre   // 'm': fold a Data Health genre collision into one spelling
	ActionYearInReview         // 'Y': open the yearly listening recap
	ActionAutoDJTune           // 'ctrl+a': open the Auto-DJ Tune panel

	// Album cover grid actions.
	ActionCursorLeft        // h, left: move to the previous tile in the album cover grid
	ActionCursorRight       // l, right: move to the next tile in the album cover grid
	ActionAlbumLayoutToggle // 'c': switch Browse Albums between the cover grid and the list

	// Jump list actions (Vim's ctrl+o / ctrl+i).
	ActionJumpBack    // ctrl+o: return to the previous place in the jump list
	ActionJumpForward // ctrl+i: move forward again in the jump list
)
