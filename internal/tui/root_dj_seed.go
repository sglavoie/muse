package tui

import (
	"time"

	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// djSeedRequest names how a start was asked for. It is the caller's statement
// of intent, not a guess made from the model: the same view can be the source
// of an ordinary start and of a contextual one, and only the entry point knows
// which the user pressed.
type djSeedRequest int

const (
	// djSeedCurrent is the ordinary `A` press. It starts from the current
	// track regardless of what the active view happens to be showing, so the
	// global shortcut means the same thing everywhere.
	djSeedCurrent djSeedRequest = iota

	// djSeedContextual is a start made from a view showing tracks: an explicit
	// bulk selection first, then the row under the cursor, then the whole list
	// on screen.
	djSeedContextual

	// djSeedCollection is a start that explicitly targets a collection — the
	// open mix, the Insights drilldown. The collection outranks the focused
	// row, because targeting the collection is what the user asked for; only
	// an explicit bulk selection inside it is more specific.
	djSeedCollection
)

// djResolveSeed decides what a DJ session started now would be seeded from and
// returns the snapshot together with a label for the notification.
//
// The precedence is the same in every view: an explicit bulk selection is the
// most specific statement the user can make, a focused row is the next, and the
// list on screen is the least. What differs is only where a request enters that
// ladder, which is what djSeedRequest carries.
//
// There is always a seed. When nothing above resolves — an empty library, a
// view with no track list, a first launch with nothing playing — the result is
// the whole-library seed rather than the zero snapshot, because the planner
// refuses an empty seed and "start from nothing in particular" has to be
// expressible.
func (m Model) djResolveSeed(req djSeedRequest, now time.Time) (radio.SeedSnapshot, string) {
	switch req {
	case djSeedContextual:
		if seed, label, ok := m.djSelectionSeed(now); ok {
			return seed, label
		}
		if seed, label, ok := m.djFocusedSeed(now); ok {
			return seed, label
		}
		if seed, label, ok := m.djCollectionSeed(now); ok {
			return seed, label
		}
	case djSeedCollection:
		if seed, label, ok := m.djSelectionSeed(now); ok {
			return seed, label
		}
		if seed, label, ok := m.djCollectionSeed(now); ok {
			return seed, label
		}
	case djSeedCurrent:
	}
	return m.djCurrentSeed(now)
}

// djCurrentSeed seeds from the track playing now, falling back to the whole
// library when nothing is current or the current row is not in the index.
func (m Model) djCurrentSeed(now time.Time) (radio.SeedSnapshot, string) {
	hash := m.playback.currentHash
	if hash != "" {
		if track, ok := m.library.Index.FindByHash(hash); ok {
			seed := radio.CaptureSeed(track, m.trackTags[hash], radio.FixedClock(now))
			return seed, seed.Label()
		}
	}
	return radio.WholeLibrarySeed(radio.FixedClock(now)), "the whole library"
}

// djSelectionSeed seeds from an explicit bulk selection in the active view.
//
// visibleTracks already narrows its result to the selection whenever one
// exists, so the selection is read back from it rather than re-derived here;
// hasBulkSelection only has to answer whether that narrowing happened.
func (m Model) djSelectionSeed(now time.Time) (radio.SeedSnapshot, string, bool) {
	if !m.hasBulkSelection() {
		return radio.SeedSnapshot{}, "", false
	}
	tracks, label, ok := m.visibleTracks()
	if !ok || len(tracks) == 0 {
		return radio.SeedSnapshot{}, "", false
	}
	seed, label := m.djSeedFromTracks(label+" (selection)", tracks, now)
	return seed, label, true
}

// djFocusedSeed seeds from the row under the cursor.
func (m Model) djFocusedSeed(now time.Time) (radio.SeedSnapshot, string, bool) {
	track, ok := m.focusedTrackForPreview()
	if !ok || track.Hash == "" {
		return radio.SeedSnapshot{}, "", false
	}
	seed := radio.CaptureSeed(m.hydrateTrack(track), m.trackTags[track.Hash], radio.FixedClock(now))
	return seed, seed.Label(), true
}

// djCollectionSeed seeds from the whole track list the active view is showing.
func (m Model) djCollectionSeed(now time.Time) (radio.SeedSnapshot, string, bool) {
	tracks, label, ok := m.visibleTracks()
	if !ok || len(tracks) == 0 {
		return radio.SeedSnapshot{}, "", false
	}
	seed, label := m.djSeedFromTracks(label, tracks, now)
	return seed, label, true
}

// djSeedFromTracks builds a seed for a set of rows.
//
// Two things happen before the aggregate is computed. Every row is hydrated
// through the library index, because a row adapted from a queue entry or a
// playlist row carries no genre and no play history at all — an unhydrated
// collection seed would be genre-less and would classify as pure discovery.
// Duplicates are then collapsed by content hash, so a track listed twice in the
// collection does not give its artist twice the weight.
//
// A collection that turns out to hold exactly one distinct track is seeded as
// that track: it *is* a track, and an aggregate of one is a strictly worse
// description of it (no hash, so no co-occurrence and no seed exclusion).
func (m Model) djSeedFromTracks(label string, tracks []library.Track, now time.Time) (radio.SeedSnapshot, string) {
	hydrated := make([]library.Track, 0, len(tracks))
	seen := make(map[string]struct{}, len(tracks))
	for _, t := range tracks {
		row := m.hydrateTrack(t)
		if row.Hash != "" {
			if _, dup := seen[row.Hash]; dup {
				continue
			}
			seen[row.Hash] = struct{}{}
		}
		hydrated = append(hydrated, row)
	}

	if len(hydrated) == 1 {
		seed := radio.CaptureSeed(hydrated[0], m.trackTags[hydrated[0].Hash], radio.FixedClock(now))
		return seed, seed.Label()
	}
	seed := radio.CaptureCollectionSeed(label, hydrated, m.trackTags, radio.FixedClock(now))
	return seed, label
}

// hasBulkSelection reports whether the active view holds an explicit bulk
// selection. Each view records its selection differently — by hash, by display
// index, by playlist row id, or on the row itself — so there is no single field
// to read. The views here keep it in one set that is meaningful whenever it is
// non-empty; the rest are answered by hasScopedSelection.
func (m Model) hasBulkSelection() bool {
	switch m.ActiveView {
	case viewLibrary:
		return len(m.library.SelectedHashes) > 0
	case viewMixes:
		return len(m.mixes.TrackView.SelectedHashes) > 0
	case viewInsights:
		return len(m.insights.TrackView.SelectedHashes) > 0
	case viewFavorites:
		return len(m.favorites.SelectedItems) > 0
	default:
		return m.hasScopedSelection()
	}
}

// hasScopedSelection answers hasBulkSelection for the views whose selection is
// only offered inside a particular level or mode, or is recorded on the rows
// themselves rather than in a set beside them.
func (m Model) hasScopedSelection() bool {
	switch m.ActiveView {
	case viewBrowse:
		return m.browseHasSelection()
	case viewTags:
		return m.tagsView.Level == tagtui.TrackLevel && len(m.tagsView.SelectedHashes) > 0
	case viewPlaylists:
		return m.playlists.Level == pltui.TrackLevel && len(m.playlists.SelectedTrackIDs) > 0
	case viewQueue:
		return queueHasSelection(m.queue.FilteredEntries())
	default:
		return false
	}
}

// browseHasSelection answers hasBulkSelection for Browse, whose three modes
// each keep their own selection and only offer one at track level.
func (m Model) browseHasSelection() bool {
	switch m.browse.Mode {
	case browse.BrowseModeAlbums:
		return m.browse.Albums.Level == albtui.TrackLevel && len(m.browse.Albums.SelectedHashes) > 0
	case browse.BrowseModeGenres:
		return m.browse.Genres.Level == gentui.TrackLevel && len(m.browse.Genres.SelectedHashes) > 0
	default:
		return m.browse.Artists.Level == arttui.TrackLevel && len(m.browse.Artists.SelectedHashes) > 0
	}
}

// queueHasSelection reports whether any visible queue row is selected.
func queueHasSelection(entries []queue.Entry) bool {
	for _, e := range entries {
		if e.Selected {
			return true
		}
	}
	return false
}
