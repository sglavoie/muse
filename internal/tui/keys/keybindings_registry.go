package keys

import (
	"sort"
	"strings"
	"unicode"
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
// bulk-action workflow; Tracks describes an open collection, and Default is
// the resting-state set.
type HintMode int

const (
	HintModeDefault        HintMode = iota // resting-state hints (no selection, no visual mode)
	HintModeSelection                      // one or more rows are bulk-selected
	HintModeVisual                         // visual line-selection mode is active
	HintModeTrackSelection                 // selected tracks, Enter enqueues the selection
	HintModeTracks                         // a collection is open at its track list
	HintModeFilter                         // a view filter is being edited
	HintModeEmpty                          // no row is available for actions
	HintModeEmptyTracks                    // an open collection has no tracks
	HintModeNoMatches                      // filters hide every row
	HintModeCollections                    // only navigation applies at this collection level
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
	if mode == HintModeTrackSelection {
		return r.trackSelectionHints(ctx)
	}

	// Filter input is handled before view bindings, identically in every view.
	if mode == HintModeCollections {
		return []primaryHint{{Key: "j/k", Label: "nav"}, {Key: "enter", Label: "open"}, {Key: ":", Label: "commands"}, {Key: "?", Label: "help"}}
	}
	if mode == HintModeEmpty || mode == HintModeEmptyTracks || mode == HintModeNoMatches {
		return emptyHints(ctx, mode)
	}
	if mode == HintModeFilter {
		return []primaryHint{{Key: "enter", Label: "apply"}, {Key: "esc", Label: "clear"}, {Key: "↑/↓", Label: "history"}}
	}
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

func emptyHints(ctx Context, mode HintMode) []primaryHint {
	hints := []primaryHint{{Key: "1", Label: "library"}}
	switch {
	case mode == HintModeNoMatches:
		hints = []primaryHint{{Key: "esc", Label: "clear/back"}, {Key: "ctrl+t", Label: "tag filter"}}
	case mode == HintModeEmptyTracks:
		hints = append([]primaryHint{{Key: "esc", Label: "back"}}, hints...)
	case emptyNeedsLibrary(ctx):
		hints = []primaryHint{{Key: "ctrl+,", Label: "folders"}, {Key: "ctrl+r", Label: "rescan"}}
	case ctx == ContextPlaylists || ctx == ContextTags:
		hints = []primaryHint{{Key: "c", Label: "create"}}
	}
	return append(hints, primaryHint{Key: ":", Label: "commands"}, primaryHint{Key: "?", Label: "help"})
}

func emptyNeedsLibrary(ctx Context) bool {
	switch ctx {
	case ContextLibrary, ContextBrowse, ContextArtists, ContextAlbums, ContextGenres, ContextMixes:
		return true
	}
	return false
}

// Track selections enqueue; collection selections still open the focused collection.
func (r Registry) trackSelectionHints(ctx Context) []primaryHint {
	hints := append([]primaryHint(nil), r.primaryHintsForMode(ctx, HintModeSelection)...)
	for i := range hints {
		if hints[i].Key == "enter" {
			hints[i].Label = "enqueue selected"
		}
	}
	return hints
}
