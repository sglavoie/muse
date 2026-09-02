package keys

import (
	"slices"
	"testing"
)

// TestPlaylistsBindingsResolve verifies that every key registered under
// contextPlaylists resolves to the expected action via registry.resolve.
func TestPlaylistsBindingsResolve(t *testing.T) {
	reg := NewRegistry()

	cases := []struct {
		key  string
		want Action
	}{
		{"j", ActionCursorDown},
		{"k", ActionCursorUp},
		{"up", ActionCursorUp},
		{"down", ActionCursorDown},
		{"enter", ActionPlaylistOpen},
		{"esc", ActionPlaylistBack},
		{"r", ActionPlaylistRename},
		{"c", ActionPlaylistCreate},
		{"d", ActionPlaylistDelete},
		{"x", ActionBulkSelectionToggle},
		{"a", ActionBulkAction},
	}

	for _, tc := range cases {
		got := reg.Resolve(ContextPlaylists, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextPlaylists, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestQueueBindingsResolve verifies that every key registered under
// contextQueue resolves to the expected action via registry.resolve.
func TestQueueBindingsResolve(t *testing.T) {
	reg := NewRegistry()

	cases := []struct {
		key  string
		want Action
	}{
		{"j", ActionCursorDown},
		{"k", ActionCursorUp},
		{"up", ActionCursorUp},
		{"down", ActionCursorDown},
		{"enter", ActionSelect},
		{"d", ActionDequeue},
		{"x", ActionBulkSelectionToggle},
		{"C", ActionClearQueue},
		{"K", ActionMoveTrackUp},
		{"J", ActionMoveTrackDown},
	}

	for _, tc := range cases {
		got := reg.Resolve(ContextQueue, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextQueue, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestArtistsBindingsResolve verifies that every key registered under
// contextArtists resolves to the expected action via registry.resolve.
func TestArtistsBindingsResolve(t *testing.T) {
	reg := NewRegistry()

	cases := []struct {
		key  string
		want Action
	}{
		{"j", ActionCursorDown},
		{"k", ActionCursorUp},
		{"up", ActionCursorUp},
		{"down", ActionCursorDown},
		{"enter", ActionArtistOpen},
		{"esc", ActionArtistBack},
		{"x", ActionBulkSelectionToggle},
		{"X", ActionClearSelection},
		{"a", ActionBulkAction},
		{"ctrl+d", ActionHalfPageDown},
		{"ctrl+u", ActionHalfPageUp},
		{"ctrl+f", ActionFullPageDown},
		{"ctrl+b", ActionFullPageUp},
		{"shift+up", ActionSortAsc},
		{"shift+down", ActionSortDesc},
		{"g", ActionGoToTop},
		{"G", ActionGoToBottom},
	}

	for _, tc := range cases {
		got := reg.Resolve(ContextArtists, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextArtists, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestAlbumsBindingsResolve verifies that every key registered under
// contextAlbums resolves to the expected action via registry.resolve.
func TestAlbumsBindingsResolve(t *testing.T) {
	reg := NewRegistry()

	cases := []struct {
		key  string
		want Action
	}{
		{"j", ActionCursorDown},
		{"k", ActionCursorUp},
		{"up", ActionCursorUp},
		{"down", ActionCursorDown},
		{"enter", ActionAlbumOpen},
		{"esc", ActionAlbumBack},
		{"x", ActionBulkSelectionToggle},
		{"X", ActionClearSelection},
		{"a", ActionBulkAction},
		{"ctrl+d", ActionHalfPageDown},
		{"ctrl+u", ActionHalfPageUp},
		{"ctrl+f", ActionFullPageDown},
		{"ctrl+b", ActionFullPageUp},
		{"shift+up", ActionSortAsc},
		{"shift+down", ActionSortDesc},
		{"g", ActionGoToTop},
		{"G", ActionGoToBottom},
	}

	for _, tc := range cases {
		got := reg.Resolve(ContextAlbums, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextAlbums, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestSwitchBrowseGlobalBinding verifies that '4' resolves to
// actionSwitchBrowse in the global context.
func TestSwitchBrowseGlobalBinding(t *testing.T) {
	reg := NewRegistry()
	got := reg.Resolve(ContextGlobal, "4")
	if got != ActionSwitchBrowse {
		t.Errorf("resolve(contextGlobal, \"4\") = %v, want actionSwitchBrowse", got)
	}
}

// TestSwitchFavoritesGlobalBinding verifies that '5' resolves to
// actionSwitchFavorites in the global context.
func TestSwitchFavoritesGlobalBinding(t *testing.T) {
	reg := NewRegistry()
	got := reg.Resolve(ContextGlobal, "5")
	if got != ActionSwitchFavorites {
		t.Errorf("resolve(contextGlobal, \"5\") = %v, want actionSwitchFavorites", got)
	}
}

// TestSwitchTagsGlobalBinding verifies that '6' resolves to
// actionSwitchTags in the global context.
func TestSwitchTagsGlobalBinding(t *testing.T) {
	reg := NewRegistry()
	got := reg.Resolve(ContextGlobal, "6")
	if got != ActionSwitchTags {
		t.Errorf("resolve(contextGlobal, \"6\") = %v, want actionSwitchTags", got)
	}
}

// TestLibraryEscResolvesToFilterClear verifies that 'esc' in the library
// context resolves to actionLibraryFilterClear.
func TestLibraryEscResolvesToFilterClear(t *testing.T) {
	reg := NewRegistry()
	got := reg.Resolve(ContextLibrary, "esc")
	if got != ActionLibraryFilterClear {
		t.Errorf("resolve(contextLibrary, \"esc\") = %v, want actionLibraryFilterClear", got)
	}
}

// TestQueueEscResolvesToFilterClear verifies that 'esc' in the queue
// context resolves to actionLibraryFilterClear.
func TestQueueEscResolvesToFilterClear(t *testing.T) {
	reg := NewRegistry()
	got := reg.Resolve(ContextQueue, "esc")
	if got != ActionLibraryFilterClear {
		t.Errorf("resolve(contextQueue, \"esc\") = %v, want actionLibraryFilterClear", got)
	}
}

// TestDebugGlobalBinding verifies that 'ctrl+g' resolves to actionDebug in
// the global context.
func TestDebugGlobalBinding(t *testing.T) {
	reg := NewRegistry()
	got := reg.Resolve(ContextGlobal, "ctrl+g")
	if got != ActionDebug {
		t.Errorf("resolve(contextGlobal, \"ctrl+g\") = %v, want actionDebug", got)
	}
}

// TestNoSilentlyShadowedGlobalBindings walks the registry and asserts that
// every view-local entry which shadows a global binding with a different
// action has a non-empty domain reason. Same-action rebinds are tolerated
// (they cause no semantic divergence). This guards the keymap consistency
// audit performed in Session 9 against silent regressions.
func TestNoSilentlyShadowedGlobalBindings(t *testing.T) {
	reg := NewRegistry()

	for rk, viewAction := range reg.Bindings {
		if rk.Ctx == ContextGlobal {
			continue
		}
		globalAction, hasGlobal := reg.Bindings[RegistryKey{Ctx: ContextGlobal, Key: rk.Key}]
		if !hasGlobal {
			continue
		}
		if viewAction == globalAction {
			continue
		}
		if reg.domainReasons[rk] == "" {
			t.Errorf("silent shadow: context=%v key=%q view-action=%v shadows global-action=%v without a domain reason — use registerWithReason or remove the override",
				rk.Ctx, rk.Key, viewAction, globalAction)
		}
	}
}

// TestKeysForReturnsStableOrder verifies that keysFor returns every key bound
// to an action in its context, ordered by (rank, length, lexical). Each case
// runs repeatedly because Go randomises map iteration order per range, so a
// missing sort would only fail intermittently.
func TestKeysForReturnsStableOrder(t *testing.T) {
	reg := NewRegistry()

	cases := []struct {
		name string
		ctx  Context
		act  Action
		want []string
	}{
		{"next view", ContextGlobal, ActionNextView, []string{"]", "tab"}},
		{"prev view", ContextGlobal, ActionPrevView, []string{"[", "shift+tab"}},
		{"tag back", ContextTags, ActionTagBack, []string{"h", "esc", "backspace"}},
		{"cursor down", ContextLibrary, ActionCursorDown, []string{"j", "down"}},
		{"unbound in context", ContextLibrary, ActionTagBack, []string{}},
		{"unbound anywhere", ContextGlobal, ActionNone, []string{}},
	}

	for _, tc := range cases {
		for i := range 20 {
			got := reg.KeysFor(tc.ctx, tc.act)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("%s: keysFor(%v, %v) = %v, want %v (iteration %d)", tc.name, tc.ctx, tc.act, got, tc.want, i)
			}
		}
	}
}

// TestKeysForRanksModifierCombosLast verifies the rank ordering directly:
// alphanumeric runes first, then other single runes, then named keys, then
// modifier combos.
func TestKeysForRanksModifierCombosLast(t *testing.T) {
	cases := []struct {
		key  string
		want int
	}{
		{"j", 0},
		{"G", 0},
		{"4", 0},
		{"]", 1},
		{"/", 1},
		{"tab", 2},
		{"esc", 2},
		{"enter", 2},
		{"backspace", 2},
		{"ctrl+d", 3},
		{"shift+tab", 3},
	}

	for _, tc := range cases {
		if got := keyRank(tc.key); got != tc.want {
			t.Errorf("keyRank(%q) = %d, want %d", tc.key, got, tc.want)
		}
	}
}

// TestPlaylistsRShadowsGlobalRepeat verifies that 'r' resolves to
// actionPlaylistRename (not actionRepeatCycle) when the Playlists view is
// active, locking in the view-local shadowing behaviour.
func TestPlaylistsRShadowsGlobalRepeat(t *testing.T) {
	reg := NewRegistry()

	got := reg.Resolve(ContextPlaylists, "r")
	if got != ActionPlaylistRename {
		t.Errorf("resolve(contextPlaylists, \"r\") = %v, want actionPlaylistRename; global repeat binding must not bleed through", got)
	}

	// Sanity-check: 'r' in a view without a local binding still returns the global action.
	got = reg.Resolve(ContextQueue, "r")
	if got != ActionRepeatCycle {
		t.Errorf("resolve(contextQueue, \"r\") = %v, want actionRepeatCycle", got)
	}
}
