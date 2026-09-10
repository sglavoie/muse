package tui

import (
	"reflect"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// djQueueModel puts the planning fixture's tracks in the queue view. Queue rows
// are the strictest case for seed resolution: they carry no genre and no play
// history at all, so an unhydrated seed built from them is visibly wrong.
func djQueueModel(hashes ...string) Model {
	m := newDJModel()
	m.ActiveView = viewQueue
	for i, h := range hashes {
		track, _ := m.library.Index.FindByHash(h)
		m.queue.Entries = append(m.queue.Entries, queueEntryFor(int64(i+1), track))
	}
	return m
}

// queueEntryFor adapts a library track the way the queue actually stores one:
// with the columns the queue table has, and none of the ones it does not.
func queueEntryFor(id int64, t library.Track) queue.Entry {
	return queue.Entry{
		ID:         id,
		Hash:       t.Hash,
		Path:       t.Path,
		Artist:     t.Artist,
		Title:      t.Title,
		Album:      t.Album,
		DurationMs: int64(t.Duration * 1000),
	}
}

// TestDJOrdinaryStartUsesTheCurrentTrack pins the global shortcut: `A` means
// the same thing in every view, so what the active view happens to be showing —
// even an explicit selection — does not change it.
func TestDJOrdinaryStartUsesTheCurrentTrack(t *testing.T) {
	m := djQueueModel("f1", "f2", "d1")
	m.queue.Entries[0].Selected = true
	m.queue.Cursor = 2
	m.playback.currentHash = "f2"

	seed, label := m.djResolveSeed(djSeedCurrent, djFixedNow)

	if want := djSeedFor(m, "f2"); !reflect.DeepEqual(seed, want) {
		t.Errorf("seed = %+v, want the current track %+v", seed, want)
	}
	if want := seed.Label(); label != want {
		t.Errorf("label = %q, want %q", label, want)
	}
}

// TestDJStartWithNothingCurrentUsesTheWholeLibrary is the "no current seed"
// case. It has to produce a real, restorable snapshot rather than the zero
// value, because the planner refuses an empty seed and starting from nothing in
// particular must survive a restart like any other start.
func TestDJStartWithNothingCurrentUsesTheWholeLibrary(t *testing.T) {
	m := newDJModel()

	seed, label := m.djResolveSeed(djSeedCurrent, djFixedNow)

	if seed.Kind() != radio.SeedKindLibrary {
		t.Errorf("seed = %+v, want the whole-library seed", seed)
	}
	if seed.IsZero() {
		t.Error("the whole-library seed is zero, so it cannot be persisted")
	}
	if label != "the whole library" {
		t.Errorf("label = %q, want %q", label, "the whole library")
	}

	// A current hash the index no longer holds is the same situation: there is
	// no metadata to capture, so it must not produce a half-empty track seed.
	m.playback.currentHash = "gone"
	if seed, _ := m.djResolveSeed(djSeedCurrent, djFixedNow); seed.Kind() != radio.SeedKindLibrary {
		t.Errorf("an unindexed current track produced %+v, want the whole-library seed", seed)
	}
}

// TestDJContextualPrecedence walks the whole ladder in one view. An explicit
// bulk selection is the most specific statement the user can make, the row
// under the cursor is next, and the list on screen is the least.
func TestDJContextualPrecedence(t *testing.T) {
	t.Run("a bulk selection outranks everything", func(t *testing.T) {
		// The unselected middle row is the only Jazz one, and it is also the
		// focused row and the current track — so a seed that reaches past the
		// selection by any route shows up in the genres below.
		m := djQueueModel("f1", "f3", "d1")
		m.queue.Entries[0].Selected = true
		m.queue.Entries[2].Selected = true
		m.queue.Cursor = 1
		m.playback.currentHash = "f3"

		seed, label := m.djResolveSeed(djSeedContextual, djFixedNow)

		if seed.Kind() != radio.SeedKindCollection {
			t.Fatalf("seed = %+v, want a collection seed over the selection", seed)
		}
		if label != "Queue (selection)" {
			t.Errorf("label = %q, want the selection to be named", label)
		}
		if want := []string{"Rock"}; !reflect.DeepEqual(seed.Genres, want) {
			t.Errorf("Genres = %v, want %v — the seed covered rows outside the selection",
				seed.Genres, want)
		}
	})

	t.Run("the focused row outranks the list", func(t *testing.T) {
		m := djQueueModel("f1", "f2", "d1")
		m.queue.Cursor = 2

		seed, label := m.djResolveSeed(djSeedContextual, djFixedNow)

		if want := djSeedFor(m, "d1"); !reflect.DeepEqual(seed, want) {
			t.Errorf("seed = %+v, want the focused row %+v", seed, want)
		}
		if want := seed.Label(); label != want {
			t.Errorf("label = %q, want %q", label, want)
		}
	})

	t.Run("an explicitly targeted collection skips the focused row", func(t *testing.T) {
		m := djQueueModel("f1", "f2", "d1")
		m.queue.Cursor = 2

		seed, label := m.djResolveSeed(djSeedCollection, djFixedNow)

		if seed.Kind() != radio.SeedKindCollection {
			t.Fatalf("seed = %+v, want a collection seed", seed)
		}
		if label != "Queue" {
			t.Errorf("label = %q, want the collection's own name", label)
		}
	})

	t.Run("a selection still outranks a targeted collection", func(t *testing.T) {
		m := djQueueModel("f1", "f2", "d1")
		m.queue.Entries[1].Selected = true

		seed, label := m.djResolveSeed(djSeedCollection, djFixedNow)

		// One selected row is one distinct track, so it seeds as that track.
		if want := djSeedFor(m, "f2"); !reflect.DeepEqual(seed, want) {
			t.Errorf("seed = %+v, want the selected row %+v", seed, want)
		}
		if want := seed.Label(); label != want {
			t.Errorf("label = %q, want %q", label, want)
		}
	})

	t.Run("a view with no track list falls back to the current track", func(t *testing.T) {
		// Insights at the briefing level shows bars, not tracks.
		m := newDJModel()
		m.ActiveView = viewInsights
		m.playback.currentHash = "f1"

		seed, _ := m.djResolveSeed(djSeedContextual, djFixedNow)

		if want := djSeedFor(m, "f1"); !reflect.DeepEqual(seed, want) {
			t.Errorf("seed = %+v, want the current track %+v", seed, want)
		}
	})
}

// TestDJCollectionSeedHydratesItsRows is the reason collection seeds go through
// the library index. A queue row carries no Genre, PlayCount, or LastPlayedAt
// column at all, so a seed built straight from one would be genre-less — every
// genre inclusion would fail against it and the whole batch would classify as
// pure discovery.
func TestDJCollectionSeedHydratesItsRows(t *testing.T) {
	m := djQueueModel("f1", "f3")

	seed, _ := m.djResolveSeed(djSeedCollection, djFixedNow)

	if want := []string{"Jazz", "Rock"}; !reflect.DeepEqual(seed.Genres, want) {
		t.Errorf("Genres = %v, want %v recovered from the library index", seed.Genres, want)
	}
}

// TestDJCollectionSeedDeduplicatesRows: a queue holding the same track twice
// must not give its artist twice the weight, or a repeat would decide what the
// session is seeded from.
func TestDJCollectionSeedDeduplicatesRows(t *testing.T) {
	m := djQueueModel("d1", "d1", "f1")

	seed, _ := m.djResolveSeed(djSeedCollection, djFixedNow)

	// Deduplicated the tally is 1-1 between Dana and Alice, and the name breaks
	// the tie; counted twice Dana would win outright.
	if seed.Artist != "Alice" {
		t.Errorf("Artist = %q, want Alice: the repeated row was counted twice", seed.Artist)
	}
}

// TestDJSingleTrackCollectionSeedsAsThatTrack. An aggregate of one is strictly
// worse than the track itself: it carries no hash, so it loses co-occurrence
// and no longer excludes the seed from its own batch.
func TestDJSingleTrackCollectionSeedsAsThatTrack(t *testing.T) {
	m := djQueueModel("f1", "f1")

	seed, label := m.djResolveSeed(djSeedCollection, djFixedNow)

	if want := djSeedFor(m, "f1"); !reflect.DeepEqual(seed, want) {
		t.Errorf("seed = %+v, want the single track %+v", seed, want)
	}
	if want := seed.Label(); label != want {
		t.Errorf("label = %q, want %q", label, want)
	}
}

// TestDJStartFromTracksHonoursTheSelection covers the entry point Mixes and
// Insights use, where the view resolves its own collection. Targeting a mix
// outranks the row under the cursor, but a bulk selection inside it is more
// specific still.
func TestDJStartFromTracksHonoursTheSelection(t *testing.T) {
	base := newDJModel(WithDJWriter(&djStubWriter{}))
	mix := []library.Track{
		mustTrack(base, "d1"),
		mustTrack(base, "d2"),
		mustTrack(base, "d3"),
	}

	t.Run("the collection when nothing is selected", func(t *testing.T) {
		m, label := base.djStartFromTracks("Mixes ▸ Late night", mix, djFixedNow)

		if !m.DJActive() {
			t.Fatal("the session did not start")
		}
		if got, ok := m.DJSeed().CollectionLabel(); !ok || got != "Mixes ▸ Late night" {
			t.Errorf("seed = %+v, want one aggregated from the mix", m.DJSeed())
		}
		if label != "Mixes ▸ Late night" {
			t.Errorf("label = %q, want the mix's name", label)
		}
	})

	t.Run("the selection when there is one", func(t *testing.T) {
		sel := djQueueModel("f1", "f2", "d1")
		sel.djWriter = &djStubWriter{}
		sel.queue.Entries[1].Selected = true

		m, _ := sel.djStartFromTracks("Mixes ▸ Late night", mix, djFixedNow)

		if want := djSeedFor(sel, "f2"); !reflect.DeepEqual(m.DJSeed(), want) {
			t.Errorf("seed = %+v, want the selected row %+v", m.DJSeed(), want)
		}
	})

	t.Run("an empty collection falls back to the current track", func(t *testing.T) {
		m := base
		m.playback.currentHash = "f1"

		m, _ = m.djStartFromTracks("Mixes ▸ Empty", nil, djFixedNow)

		if want := djSeedFor(base, "f1"); !reflect.DeepEqual(m.DJSeed(), want) {
			t.Errorf("seed = %+v, want the current track %+v", m.DJSeed(), want)
		}
	})
}

// TestDJBulkSelectionIsRecognisedPerView guards the one thing that cannot be
// read from a single field: every view records its selection differently, so a
// view added without a case here would silently seed from its whole list when
// the user had picked three rows.
func TestDJBulkSelectionIsRecognisedPerView(t *testing.T) {
	t.Run("queue", func(t *testing.T) {
		m := djQueueModel("f1", "f2")
		if m.djHasBulkSelection() {
			t.Error("an unselected queue reported a bulk selection")
		}
		m.queue.Entries[1].Selected = true
		if !m.djHasBulkSelection() {
			t.Error("a selected queue row was not recognised")
		}
	})

	t.Run("library", func(t *testing.T) {
		m := newDJModel()
		m.ActiveView = viewLibrary
		if m.djHasBulkSelection() {
			t.Error("an unselected library reported a bulk selection")
		}
		m.library.SelectedHashes = map[string]struct{}{"f1": {}}
		if !m.djHasBulkSelection() {
			t.Error("a selected library row was not recognised")
		}
	})

	t.Run("a view with no selection concept", func(t *testing.T) {
		m := newDJModel()
		m.ActiveView = viewInsights
		if m.djHasBulkSelection() {
			t.Error("a view with no track list reported a bulk selection")
		}
	})
}

// mustTrack returns a fixture track by hash.
func mustTrack(m Model, hash string) library.Track {
	track, ok := m.library.Index.FindByHash(hash)
	if !ok {
		panic("fixture track missing: " + hash)
	}
	return track
}
