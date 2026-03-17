package tui

import "testing"

// TestPlaylistsBindingsResolve verifies that every key registered under
// contextPlaylists resolves to the expected action via registry.resolve.
func TestPlaylistsBindingsResolve(t *testing.T) {
	reg := newRegistry()

	cases := []struct {
		key  string
		want action
	}{
		{"j", actionCursorDown},
		{"k", actionCursorUp},
		{"up", actionCursorUp},
		{"down", actionCursorDown},
		{"enter", actionPlaylistOpen},
		{"esc", actionPlaylistBack},
		{"r", actionPlaylistRename},
		{"c", actionPlaylistCreate},
		{"d", actionPlaylistDelete},
		{"x", actionBulkSelectionToggle},
		{"a", actionBulkRemovePlaylistTracks},
	}

	for _, tc := range cases {
		got := reg.resolve(contextPlaylists, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextPlaylists, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestQueueBindingsResolve verifies that every key registered under
// contextQueue resolves to the expected action via registry.resolve.
func TestQueueBindingsResolve(t *testing.T) {
	reg := newRegistry()

	cases := []struct {
		key  string
		want action
	}{
		{"j", actionCursorDown},
		{"k", actionCursorUp},
		{"up", actionCursorUp},
		{"down", actionCursorDown},
		{"enter", actionSelect},
		{"d", actionDequeue},
		{"x", actionBulkSelectionToggle},
		{"C", actionClearQueue},
		{"K", actionMoveTrackUp},
		{"J", actionMoveTrackDown},
	}

	for _, tc := range cases {
		got := reg.resolve(contextQueue, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextQueue, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestArtistsBindingsResolve verifies that every key registered under
// contextArtists resolves to the expected action via registry.resolve.
func TestArtistsBindingsResolve(t *testing.T) {
	reg := newRegistry()

	cases := []struct {
		key  string
		want action
	}{
		{"j", actionCursorDown},
		{"k", actionCursorUp},
		{"up", actionCursorUp},
		{"down", actionCursorDown},
		{"enter", actionArtistOpen},
		{"esc", actionArtistBack},
		{"ctrl+d", actionHalfPageDown},
		{"ctrl+u", actionHalfPageUp},
		{"ctrl+f", actionFullPageDown},
		{"ctrl+b", actionFullPageUp},
		{"g", actionGoToTop},
		{"G", actionGoToBottom},
	}

	for _, tc := range cases {
		got := reg.resolve(contextArtists, tc.key)
		if got != tc.want {
			t.Errorf("resolve(contextArtists, %q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestSwitchArtistsGlobalBinding verifies that '4' resolves to
// actionSwitchArtists in the global context.
func TestSwitchArtistsGlobalBinding(t *testing.T) {
	reg := newRegistry()
	got := reg.resolve(contextGlobal, "4")
	if got != actionSwitchArtists {
		t.Errorf("resolve(contextGlobal, \"4\") = %v, want actionSwitchArtists", got)
	}
}

// TestPlaylistsRShadowsGlobalRepeat verifies that 'r' resolves to
// actionPlaylistRename (not actionRepeatCycle) when the Playlists view is
// active, locking in the view-local shadowing behaviour.
func TestPlaylistsRShadowsGlobalRepeat(t *testing.T) {
	reg := newRegistry()

	got := reg.resolve(contextPlaylists, "r")
	if got != actionPlaylistRename {
		t.Errorf("resolve(contextPlaylists, \"r\") = %v, want actionPlaylistRename; global repeat binding must not bleed through", got)
	}

	// Sanity-check: 'r' in a view without a local binding still returns the global action.
	got = reg.resolve(contextQueue, "r")
	if got != actionRepeatCycle {
		t.Errorf("resolve(contextQueue, \"r\") = %v, want actionRepeatCycle", got)
	}
}
