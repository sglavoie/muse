package keys

import (
	"sort"
	"strings"
	"unicode"
)

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
//   d   | —              | —          | delete     | dequeue    | —             | —          | tag-delete | —           | —
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
//   v   | —              | visual     | visual     | visual     | visual        | visual     | visual     | —           | visual
//   x   | —              | bulk-sel   | bulk-sel   | bulk-sel   | bulk-sel      | bulk-sel   | bulk-sel   | mix-dismiss*| bulk-sel
//   X   | —              | clear-sel  | clear-sel  | clear-sel  | clear-sel     | clear-sel  | clear-sel  | mix-undism* | clear-sel
//   a   | —              | bulk-act   | bulk-act   | —          | bulk-act      | bulk-act   | bulk-act   | —           | bulk-act
//   K/J | —              | —          | move-trk   | move-trk   | —             | —          | —          | —           | —
//   C   | —              | —          | —          | clear-q    | —             | —          | —          | —           | —
//   ` / ~ | —            | mode-cycle | —          | —          | mode-cycle    | —          | —          | —           | panel-cycle
//   w / W | —            | —          | —          | —          | —             | —          | —          | —           | window-cycle
//   b   | —              | —          | —          | —          | —             | —          | —          | —           | jump-browse
//   !   | —              | —          | —          | —          | —             | —          | —          | —           | anti-rut
//   h, backspace | —     | —          | —          | —          | —             | —          | tag-back   | —           | —
//   ctrl+y/x/p | —       | —          | —          | yank/cut/paste | —         | —          | —          | —           | —
//   ctrl+a | autodj-tune | =          | =          | =          | =             | =          | =          | =           | =
//
// Entries marked "*" are intentional shadows of a global binding with a
// different action. Each must be registered via registerWithReason so the
// TestNoSilentlyShadowedGlobalBindings test passes — the domain reason is
// surfaced as the override's justification. Same-action rebinds (Tags F)
// are treated as harmless redundancy and not flagged.
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
	ActionLibraryEnqueueReplace // shift+enter: clear queue and enqueue focused track
	ActionLibraryFilterClear    // esc: clear a committed filter in the library view
	ActionPlayNext              // 'N': insert the focused track right after the currently playing track

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
	ActionMixOpen           // enter: drill into mix track list
	ActionMixBack           // esc: go back from track list to mix list
	ActionMixEnqueueAll     // 'E': enqueue all tracks from the focused mix
	ActionSaveMixAsPlaylist // 'S': save current mix as a static playlist
	ActionMixRadioSeed      // 'A': start radio seeded from the focused mix's aggregate metadata
	ActionMixDismiss        // 'x': dismiss focused track from recommendations
	ActionMixUndismiss      // 'X': undismiss focused track

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
	ActionAutoDJTune           // 'ctrl+a': open the Auto-DJ Tune panel
)

// RegistryKey is the composite map key for registry lookups.
type RegistryKey struct {
	Ctx Context
	Key string
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
	ctx  Context
	mode HintMode
}

// Registry maps (context, key) → action. The zero value is not usable;
// use NewRegistry to obtain a pre-populated instance.
//
// domainReasons records the justification for a view-local override that
// shadows a global binding with a different action. The audit test
// (TestNoSilentlyShadowedGlobalBindings) requires every such override to
// have a non-empty reason here, so any silently shadowed key fails CI.
type Registry struct {
	Bindings      map[RegistryKey]Action
	primaries     map[primaryHintKey][]primaryHint
	domainReasons map[RegistryKey]string
}

// primaryHints returns the ordered list of default-mode primary keybinding
// hints for ctx. Equivalent to PrimariesForMode(ctx, HintModeDefault).
func (r Registry) primaryHints(ctx Context) []primaryHint {
	return r.primaries[primaryHintKey{ctx: ctx, mode: HintModeDefault}]
}

// primaryHintsForMode returns the primary hints for (ctx, mode). When mode
// is non-default and no hints are registered for it, falls back to the
// default-mode set so the footer never goes empty just because a mode-
// specific set wasn't customized for this view.
func (r Registry) primaryHintsForMode(ctx Context, mode HintMode) []primaryHint {
	if hints, ok := r.primaries[primaryHintKey{ctx: ctx, mode: mode}]; ok && len(hints) > 0 {
		return hints
	}
	return r.primaries[primaryHintKey{ctx: ctx, mode: HintModeDefault}]
}

// registerNavigation registers the shared cursor/scroll navigation bindings
// (j/k, up/down, half/full page, go-to-top/bottom) for the given context.
func (r *Registry) registerNavigation(ctx Context) {
	r.register(ctx, "j", ActionCursorDown)
	r.register(ctx, "k", ActionCursorUp)
	r.register(ctx, "up", ActionCursorUp)
	r.register(ctx, "down", ActionCursorDown)
	r.register(ctx, "ctrl+d", ActionHalfPageDown)
	r.register(ctx, "ctrl+u", ActionHalfPageUp)
	r.register(ctx, "ctrl+f", ActionFullPageDown)
	r.register(ctx, "ctrl+b", ActionFullPageUp)
	r.register(ctx, "g", ActionGoToTop)
	r.register(ctx, "G", ActionGoToBottom)
}

// register adds a (context, key) → action entry to the registry.
func (r *Registry) register(ctx Context, key string, a Action) {
	r.Bindings[RegistryKey{Ctx: ctx, Key: key}] = a
}

// registerWithReason registers a view-local binding that intentionally
// shadows a global binding with a different action. The reason is recorded
// in domainReasons; TestNoSilentlyShadowedGlobalBindings asserts that any
// shadowed key has a non-empty reason.
func (r *Registry) registerWithReason(ctx Context, key string, a Action, reason string) {
	r.register(ctx, key, a)
	r.domainReasons[RegistryKey{Ctx: ctx, Key: key}] = reason
}

// addPrimary records (key, label) as a default-mode primary hint without
// (re)binding the key. Use this when the binding already exists in another
// context (e.g. a global binding that we want to surface in a view-local
// hint strip).
func (r *Registry) addPrimary(ctx Context, key, label string) {
	r.addPrimaryMode(ctx, HintModeDefault, key, label)
}

// addPrimaryMode records (key, label) as a primary hint for the given
// (context, mode) pair without (re)binding the key. Used to surface
// selection-aware and visual-mode hint sets in the footer strip.
func (r *Registry) addPrimaryMode(ctx Context, mode HintMode, key, label string) {
	k := primaryHintKey{ctx: ctx, mode: mode}
	r.primaries[k] = append(r.primaries[k], primaryHint{Key: key, Label: label})
}

// Lookup returns the action bound to (ctx, key), or actionNone if not found.
func (r Registry) Lookup(ctx Context, key string) Action {
	return r.Bindings[RegistryKey{Ctx: ctx, Key: key}]
}

// keyRank classifies a key string for display ordering: 0 for a single
// alphanumeric rune, 1 for any other single rune, 2 for a named key
// (tab, esc, enter, backspace, up, ...), 3 for a modifier combo (ctrl+…,
// shift+…). Lower ranks read as the more idiomatic shortcut to advertise.
func keyRank(key string) int {
	runes := []rune(key)
	if len(runes) == 1 {
		if unicode.IsLetter(runes[0]) || unicode.IsDigit(runes[0]) {
			return 0
		}
		return 1
	}
	if strings.Contains(key, "+") {
		return 3
	}
	return 2
}

// KeysFor returns every key bound to a in ctx, in stable display order.
// Unlike resolve there is no contextGlobal fallback: callers that want one
// ask for contextGlobal explicitly. Results are sorted by
// (rank, length, lexical) so that map iteration order never reaches
// rendered output.
func (r Registry) KeysFor(ctx Context, a Action) []string {
	keys := make([]string, 0, 4)
	for rk, bound := range r.Bindings {
		if rk.Ctx == ctx && bound == a {
			keys = append(keys, rk.Key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := keyRank(keys[i]), keyRank(keys[j])
		if ri != rj {
			return ri < rj
		}
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}

// Resolve returns the effective action for key in the given view context.
// The view-local context is checked first; if no binding is found there the
// global context is consulted. This gives view-local bindings precedence over
// global ones for the same key.
func (r Registry) Resolve(viewCtx Context, key string) Action {
	if a := r.Lookup(viewCtx, key); a != ActionNone {
		return a
	}
	return r.Lookup(ContextGlobal, key)
}
