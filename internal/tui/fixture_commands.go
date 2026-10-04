package tui

import (
	"fmt"
	"slices"

	"github.com/sglavoie/muse/internal/tui/sorting"
)

// This file holds the fixture-safe palette command allowlist. It lives in
// package tui rather than tui/uitest because the production update loop
// consults it directly (root_update.go gates palette execution on it for
// fixture-backed models), and tui cannot import tui/uitest.

// safeDirectCommandIDs is the allowlist of palette command identifiers that are
// safe to invoke directly against a fixture: internal navigation and UI/state
// toggles whose handlers never touch real config, databases, music files, mpv,
// Finder, or the clipboard. Every ID here must exist in the built command set;
// TestSafeDirectCommandIDsExist catches a renamed catalog ID that would
// otherwise silently unlist a fixture-safe command.
func safeDirectCommandIDs() map[string]struct{} {
	return map[string]struct{}{
		"messages.show":          {},
		"help.show":              {},
		"help.modes_legend":      {},
		"debug.show":             {},
		"radio.toggle":           {},
		"autodj.tune":            {},
		"nav.playlists":          {},
		"nav.queue":              {},
		"nav.favorites":          {},
		"nav.tags":               {},
		"nav.mixes":              {},
		"nav.library.all":        {},
		"nav.library.recent":     {},
		"nav.library.history":    {},
		"nav.browse.artists":     {},
		"nav.browse.albums":      {},
		"nav.browse.genres":      {},
		"ui.toggle.footer_hints": {},
		"ui.toggle.theme":        {},
		"ui.toggle.track_notify": {},
		"queue.toggle.follow":    {},
	}
}

// IsSafeDirectCommand reports whether the palette command identifier is on the
// fixture-safe direct-command allowlist.
func IsSafeDirectCommand(id string) bool {
	// Sort choices are view-dependent, so they cannot all appear in the
	// default fixture palette checked by safeDirectCommandIDs.
	fields := slices.Concat(sorting.LibrarySortFields, sorting.RecentlyAddedSortFields,
		sorting.HistorySortFields, sorting.QueueSortFields, sorting.PlaylistListSortFields)
	for _, field := range fields {
		if id == fmt.Sprintf("sort.field.%d", field) {
			return true
		}
	}
	_, ok := safeDirectCommandIDs()[id]
	return ok
}
