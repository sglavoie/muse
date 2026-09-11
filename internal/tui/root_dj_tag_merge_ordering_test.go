package tui

import (
	"errors"
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// A whole-session write that lands between the merge transaction and the
// completion the Model is still waiting for, over a real database.
//
// The confirmation is answered off the update loop: MergeTags commits — tags,
// track_tags, and every dj_tag_refs row it owns — and only then does the runtime
// deliver tagMergedMsg. In that gap the Model is still the pre-merge copy of the
// same references, and it is the copy every whole-session write is rebuilt from.
// A Follow reseed, an Apply, or the tick reconciling an earlier refusal therefore
// hands the merged-away id straight back to replaceDJFilters, which stores a
// positive id whether or not a tag still carries it.
//
// Nothing downstream notices on its own: the live state is about to be correct,
// so no unsaved marker is raised, and the row stays stale until some unrelated
// later write happens to rewrite it — or until a restart restores a filter that
// resolves to nothing and parks the refill.
//
// These use the journey harness because the window only exists where all of it
// is real at once: the writer's MergeTags, the session row, the dj_tag_refs
// rows, the tick, and a relaunch that reads them back.

// playQueueRow delivers the player's confirmation that it moved onto a queue
// row. This is the production path a Follow session reseeds from, and the reseed
// is a whole-session write.
func (j *djJourney) playQueueRow(idx int) {
	j.t.Helper()
	if idx < 0 || idx >= len(j.m.queue.Entries) {
		j.t.Fatalf("queue row %d is outside the %d-row queue", idx, len(j.m.queue.Entries))
	}
	e := j.m.queue.Entries[idx]
	j.player.current = idx
	j.m = djDeliver(j.m, playback.TrackChangedMsg{
		Hash: e.Hash, Artist: e.Artist, Title: e.Title, Index: idx, QueueID: e.ID,
	})
	if got := j.m.playback.currentHash; got != e.Hash {
		j.t.Fatalf("the track change was discarded: playing %q, want the row %q", got, e.Hash)
	}
}

// assertMergedSessionRefs checks every durable reference the session owns after
// a merge: both filter sides and the captured seed.
func (j *djJourney) assertMergedSessionRefs(dest, other int64) {
	j.t.Helper()
	if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{dest}) {
		j.t.Errorf("the durable include refs are %v, want the kept tag [%d]", got, dest)
	}
	if got := j.sessionTagRefs("exclude"); !slices.Equal(got, []int64{other}) {
		j.t.Errorf("the durable exclude refs are %v, want the untouched [%d]", got, other)
	}
	if got := j.sessionTagRefs("seed"); !slices.Equal(got, []int64{dest}) {
		j.t.Errorf("the durable seed refs are %v, want the kept tag [%d]", got, dest)
	}
}

// assertRestoredSessionResolves reopens the file and checks the session a
// restart would run: the references it names, and whether the catalog can still
// resolve them. Nothing between the merge completion and this call writes, so
// what it reads is what the merge left behind.
func (j *djJourney) assertRestoredSessionResolves(dest, other int64) {
	j.t.Helper()
	j.relaunch()
	j.loadTags()

	if err := j.m.DJRestoreErr(); err != nil {
		j.t.Fatalf("the restarted launch refused the saved session: %v", err)
	}
	restored := j.m.DJSettings()
	if got := restored.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
		j.t.Errorf("the restored inclusion is %v, want the kept tag [%d]", got, dest)
	}
	if got := restored.Exclude.TagIDs; !slices.Equal(got, []int64{other}) {
		j.t.Errorf("the restored exclusion is %v, want [%d]", got, other)
	}
	if got := j.m.DJSeed().TagIDs; !slices.Equal(got, []int64{dest}) {
		j.t.Errorf("the restored seed references %v, want the kept tag [%d]", got, dest)
	}
	if err := restored.ResolveTags(j.m.djTagCatalog()); err != nil {
		j.t.Errorf("the restored session references a tag the catalog cannot resolve: %v", err)
	}
}

// TestAutoDJTagMergeSurvivesPersistBeforeResult drives a real whole-session write
// through the window the merge command leaves open, and holds that the durable
// references are correct once the completion is delivered — without any later
// lifecycle gesture to rescue them.
func TestAutoDJTagMergeSurvivesPersistBeforeResult(t *testing.T) {
	t.Run("a Follow reseed before the completion cannot strand the remap", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		s := radio.DefaultSettings()
		// Follow is what turns an ordinary confirmed track change into a
		// whole-session write, with no gesture from the user at all.
		s.Anchor = radio.AnchorFollow
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)
		j.refill()

		seeded := j.m.DJSeed().TrackHash
		if got := j.sessionTagRefs("seed"); !slices.Equal(got, []int64{src}) {
			t.Fatalf("the started session stored seed refs %v, want [%d]", got, src)
		}

		// The transaction commits here; the Model does not hear about it yet.
		merged := j.mergeCommit(src, dest)
		if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{dest}) {
			t.Fatalf("the merge transaction left include refs %v, want [%d]", got, dest)
		}

		// The player moves onto the next row inside that window.
		next := len(j.m.queue.Entries) - 1
		if next == 0 {
			t.Fatal("the refill added no row, so there is nothing to reseed onto")
		}
		wantSeed := j.m.queue.Entries[next].Hash
		j.playQueueRow(next)
		if got := j.m.DJSeed().TrackHash; got != wantSeed {
			t.Fatalf("the Follow session did not reseed: seed is %q, want %q", got, wantSeed)
		}
		if got := j.session().Seed.TrackHash; got != wantSeed {
			t.Fatalf("the reseed did not reach the database: stored seed is %q, want %q", got, wantSeed)
		}

		j.deliverMerge(merged)

		j.assertMergedSessionRefs(dest, other)
		// The reseed happened after the merge, so it is the newer fact. Repairing
		// the references by restoring a session captured before the merge would
		// put the previous seed back.
		if got := j.session().Seed.TrackHash; got != wantSeed {
			t.Errorf("the stored seed is %q, want the reseeded %q rather than the pre-merge %q",
				got, wantSeed, seeded)
		}
		if j.m.dj.unsaved.pending() {
			t.Errorf("the session reads unsaved after a write that landed: %v", j.m.dj.unsaved.err)
		}

		j.assertRestoredSessionResolves(dest, other)
		if got := j.m.DJSeed().TrackHash; got != wantSeed {
			t.Errorf("the restored seed is %q, want the reseeded %q", got, wantSeed)
		}
		// The point of all of it: the restored session can still plan.
		j.refill()
		if j.m.DJPlanBlocked() {
			t.Errorf("the restored session parked the refill: %v", j.m.djPlanBlockReasons())
		}
	})

	t.Run("a tick reconciling an outstanding write before the completion cannot strand the remap", func(t *testing.T) {
		j, w := newRefusingJourney(t)
		src, dest, other := j.mergeFixtureTags()

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)

		// An Apply the database refuses leaves the running session ahead of the
		// saved one, which is what arms the reconciliation on the tick.
		w.fails = 1
		applied, err := j.m.DJSettings().WithPreset(radio.PresetExplore)
		if err != nil {
			t.Fatalf("WithPreset: %v", err)
		}
		j.m, err = j.m.djApplySettings(applied)
		if err != nil {
			t.Fatalf("djApplySettings: %v", err)
		}
		if !j.m.dj.unsaved.pending() {
			t.Fatal("the refused Apply left nothing outstanding to reconcile")
		}
		// A carry an accept transaction committed on its own, mirrored into the
		// Model without rewriting the session row. It is the third thing the
		// reconciliation has to carry forward.
		j.m = j.m.djNoteAllocation(radio.Allocation{CarryUnits: 20})

		// The database recovers, and the merge commits into the same window.
		w.fails = 0
		merged := j.mergeCommit(src, dest)
		if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{dest}) {
			t.Fatalf("the merge transaction left include refs %v, want [%d]", got, dest)
		}

		// The tick is not a gesture the user made: it rewrites the whole row from
		// a Model that is still holding the pre-merge id.
		j.m = djDeliver(j.m, tickMsg{})
		if j.m.dj.unsaved.pending() {
			t.Fatalf("the reconciliation did not land, so no stale write reached the row: %v",
				j.m.dj.unsaved.err)
		}
		if got := j.session().Settings.Preset(); got != radio.PresetExplore {
			t.Fatalf("the reconciled settings are %q, want the applied Explore", got)
		}

		j.deliverMerge(merged)

		j.assertMergedSessionRefs(dest, other)
		// Both newer facts survive: the settings the reconciliation saved, and the
		// carry the accepted batch left behind.
		stored := j.session()
		if got := stored.Settings.Preset(); got != radio.PresetExplore {
			t.Errorf("the stored preset is %q, want the applied Explore", got)
		}
		if got := stored.Allocation.CarryUnits; got != 20 {
			t.Errorf("the stored carry is %d, want the adopted 20", got)
		}
		if j.m.dj.unsaved.pending() {
			t.Errorf("the session reads unsaved after a write that landed: %v", j.m.dj.unsaved.err)
		}

		j.assertRestoredSessionResolves(dest, other)
		if got := j.m.DJSettings().Preset(); got != radio.PresetExplore {
			t.Errorf("the restored preset is %q, want Explore", got)
		}
		if got := j.m.DJAllocation().CarryUnits; got != 20 {
			t.Errorf("the restored carry is %d, want 20", got)
		}
	})

	t.Run("a reconciliation the database refuses is reported and retried", func(t *testing.T) {
		j, w := newRefusingJourney(t)
		src, dest, other := j.mergeFixtureTags()

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)

		merged := j.mergeCommit(src, dest)
		// Adopting an accepted batch's carry rewrites the whole row, seed
		// references and all, from whatever the Model is holding.
		j.m = j.m.djAdoptAllocation(radio.Allocation{CarryUnits: 20})
		if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{src}) {
			t.Fatalf("the write in the window left include refs %v; this subtest needs the stale %v",
				got, []int64{src})
		}

		// The database refuses the write the completion performs. The merge is
		// still a success — it is the session row that is now behind.
		w.fails = 1
		j.deliverMerge(merged)

		if got := j.m.DJSettings().Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the running session still names %v, want the kept tag [%d]", got, dest)
		}
		err := j.m.djUnsavedErr()
		if err == nil {
			t.Fatal("a refused reconciliation was not recorded, so nothing will retry it")
		}
		if !errors.Is(err, errDJUnsaved) || !errors.Is(err, errDJSessionRefused) {
			t.Errorf("unsaved error = %v, want the refusal reported as unsaved", err)
		}
		if !djStatusMentions(j.m.autoDJPanelStatus(), "Not saved") {
			t.Errorf("panel status lines = %+v, want a standing unsaved line", j.m.autoDJPanelStatus())
		}

		// The bounded path the refusal is kept for.
		w.fails = 0
		j.m = djDeliver(j.m, tickMsg{})
		if j.m.dj.unsaved.pending() {
			t.Fatalf("the retry did not reconcile the row: %v", j.m.dj.unsaved.err)
		}

		j.assertMergedSessionRefs(dest, other)
		if got := j.session().Allocation.CarryUnits; got != 20 {
			t.Errorf("the stored carry is %d, want the adopted 20", got)
		}
		j.assertRestoredSessionResolves(dest, other)
	})
}
