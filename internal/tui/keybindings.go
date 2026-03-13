package tui

// bindingContext identifies the context in which key bindings are resolved.
// The registry checks the view-local context first, then falls back to Global.
type bindingContext int

const (
	contextGlobal    bindingContext = iota // applies in every view
	contextLibrary                         // applies only in the Library view
	contextPlaylists                       // applies only in the Playlists view
	contextQueue                           // applies only in the Queue view
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
	actionRepeatCycle   // 'r': cycle repeat mode off→on (toggle)
	actionShuffleToggle // 'z': toggle shuffle mode
	actionUndo          // 'u': dispatch undo
	actionFilterBar     // '/': activate filter bar in active view
	actionHelp          // '?': show help overlay

	// Queue-local actions.
	actionCursorUp
	actionCursorDown
	actionSelect
	actionDequeue            // 'd': remove the selected track from the queue
	actionBulkSelectionToggle // 'x': toggle bulk-selection on the selected entry
	actionClearQueue          // 'C': clear all entries from the queue
	actionMoveTrackUp         // 'K': move selected track one position earlier
	actionMoveTrackDown       // 'J': move selected track one position later

	// Playlists-local actions.
	actionPlaylistRename
	actionPlaylistCreate // 'c': create new playlist
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
	r.register(contextGlobal, "R", actionRescan)
	r.register(contextGlobal, "r", actionRepeatCycle)
	r.register(contextGlobal, "z", actionShuffleToggle)
	r.register(contextGlobal, "u", actionUndo)
	r.register(contextGlobal, "/", actionFilterBar)
	r.register(contextGlobal, "?", actionHelp)

	// Queue-local bindings.
	r.register(contextQueue, "up", actionCursorUp)
	r.register(contextQueue, "down", actionCursorDown)
	r.register(contextQueue, "enter", actionSelect)
	r.register(contextQueue, "d", actionDequeue)
	r.register(contextQueue, "x", actionBulkSelectionToggle)
	r.register(contextQueue, "C", actionClearQueue)
	r.register(contextQueue, "K", actionMoveTrackUp)
	r.register(contextQueue, "J", actionMoveTrackDown)

	// Playlists-local bindings.
	// 'r' is bound to rename here, shadowing the global repeat-cycle binding
	// while the Playlists view is focused.
	r.register(contextPlaylists, "r", actionPlaylistRename)
	r.register(contextPlaylists, "c", actionPlaylistCreate)

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
