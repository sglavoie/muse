package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
)

// A recipe write that lands between the merge transaction and the completion the
// Model is still waiting for, over a real database.
//
// The sibling file covers the prompt that is still open when the completion
// arrives: RemapTag moves its frozen settings, so submitting it afterwards
// stores the kept tag. This is the other half of the same ordering. The
// confirmation is answered off the update loop, so MergeTags commits — tags,
// track_tags, and every dj_tag_refs row it owns — strictly before tagMergedMsg
// is delivered, and a Save-as or a Rename submitted in that gap runs through the
// synchronous recipe writer with the snapshot its prompt froze *before* the
// merge. replaceDJFilters rewrites that recipe's whole filter set from it and
// stores a positive id whether or not a tag still carries it, so the durable
// remap is replaced by the merged-away reference.
//
// Nothing downstream notices on its own: the write succeeded, the panel is told
// so, and the completion that lands a moment later moves panel memory and the
// session row only. The recipe stays stale on disk and reopens on a filter that
// resolves to nothing.
//
// These use the journey harness because the window only exists where all of it
// is real at once: the writer's MergeTags, the recipe writer, the dj_tag_refs
// rows, the tick, and a relaunch that reads them back.

// assertStaleRecipeRef fails unless the recipe's stored references still name the
// merged-away tag. It is a precondition rather than an assertion about the
// product: a write that never stored the stale id would leave the rest of the
// test proving nothing.
func (j *djJourney) assertStaleRecipeRef(id int64, role string, src int64) {
	j.t.Helper()
	if got := j.recipeTagRefs(id, role); !slices.Contains(got, src) {
		j.t.Fatalf("the write inside the window stored %s refs %v; this test needs the stale %d",
			role, got, src)
	}
}

// assertMergedRecipeRefs checks everything one repaired recipe owes the user: one
// tag on each side of the filter, the merged pair collapsed onto the kept tag
// exactly once rather than listed twice, and settings the catalog can resolve.
func (j *djJourney) assertMergedRecipeRefs(name string, include, exclude int64) {
	j.t.Helper()
	recipe := j.recipeNamed(name)
	if got := recipe.Settings.Include.TagIDs; !slices.Equal(got, []int64{include}) {
		j.t.Errorf("%q holds inclusion %v, want [%d]", name, got, include)
	}
	if got := recipe.Settings.Exclude.TagIDs; !slices.Equal(got, []int64{exclude}) {
		j.t.Errorf("%q holds exclusion %v, want [%d]", name, got, exclude)
	}
	if got := j.recipeTagRefs(recipe.ID, "include"); !slices.Equal(got, []int64{include}) {
		j.t.Errorf("%q has durable include refs %v, want [%d]", name, got, include)
	}
	if got := j.recipeTagRefs(recipe.ID, "exclude"); !slices.Equal(got, []int64{exclude}) {
		j.t.Errorf("%q has durable exclude refs %v, want [%d]", name, got, exclude)
	}
	if err := recipe.Settings.ResolveTags(j.m.djTagCatalog()); err != nil {
		j.t.Errorf("%q references a tag the catalog cannot resolve: %v", name, err)
	}
}

// assertRestoredRecipeResolves reopens the file and checks the recipe a restart
// would list. Nothing between the completion and this call writes, so what it
// reads is what the repair left behind rather than what a later gesture fixed.
func (j *djJourney) assertRestoredRecipeResolves(name string, include, exclude int64) {
	j.t.Helper()
	j.relaunch()
	j.loadTags()
	j.assertMergedRecipeRefs(name, include, exclude)
}

// djMergeSettings is a snapshot filtered on one tag in each direction, which is
// what makes a merge visible on both sides of a stored recipe.
func djMergeSettings(include, exclude int64) radio.Settings {
	s := radio.DefaultSettings()
	s.Include = radio.FilterSet{TagIDs: []int64{include}}
	s.Exclude = radio.FilterSet{TagIDs: []int64{exclude}}
	return s
}

// TestAutoDJTagMergeSurvivesRecipeWriteBeforeResult drives a real recipe write
// through the window the merge command leaves open, and holds that the stored
// filters are correct once the completion is delivered — without any later recipe
// gesture to rescue them.
func TestAutoDJTagMergeSurvivesRecipeWriteBeforeResult(t *testing.T) {
	t.Run("a Save-as submitted before the completion cannot strand the remap", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		s := djMergeSettings(src, other)
		// The inclusion already names both sides, which is the case a blind
		// substitution would store as the same tag twice.
		s.Include.TagIDs = []int64{src, dest}
		j.startFiltered(s)

		// The box opens on a copy of the draft taken here, before the merge the
		// user confirms a moment later.
		j.press("ctrl+a", "tab", "tab", "s")
		j.m = djTypeName(t, j.m, "Late Mix")

		// The transaction commits here; the Model does not hear about it yet.
		merged := j.mergeCommit(src, dest)

		// The user presses enter inside that window. The write is synchronous, so
		// it reaches storage before the completion does.
		j.press("enter")
		saved := j.recipeNamed("Late Mix")
		j.assertStaleRecipeRef(saved.ID, "include", src)

		j.deliverMerge(merged)

		j.assertMergedRecipeRefs("Late Mix", dest, other)
		if got := j.recipeNamed("Late Mix").ID; got != saved.ID {
			t.Errorf("the repair rewrote recipe %d, want the one the save created %d", got, saved.ID)
		}
		if got := len(j.recipes()); got != 1 {
			t.Errorf("storage holds %d recipes, want only the saved one", got)
		}
		if j.m.dj.staleRefs.pending() {
			t.Errorf("the repair reads outstanding after a write that landed: %v", j.m.dj.staleRefs.err)
		}

		j.assertRestoredRecipeResolves("Late Mix", dest, other)
	})

	t.Run("a Rename submitted before the completion cannot strand the remap", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		stored := djMergeSettings(other, src)
		// The exclusion names both sides: an exclusion stays an exclusion, and the
		// pair collapses onto the kept tag rather than being listed twice.
		stored.Exclude.TagIDs = []int64{src, dest}
		id := j.saveRecipe("Late Night", stored)

		// A rename carries the recipe's own snapshot, not the draft, so the draft
		// is deliberately left on unrelated settings.
		j.startFiltered(radio.DefaultSettings())
		j.press("ctrl+a", "tab", "tab", "n")

		merged := j.mergeCommit(src, dest)
		if got := j.recipeTagRefs(id, "exclude"); !slices.Equal(got, []int64{dest}) {
			t.Fatalf("the merge transaction left the recipe's exclude refs %v, want [%d]", got, dest)
		}

		j.press("ctrl+u")
		j.m = djTypeName(t, j.m, "Very Late")
		j.press("enter")
		j.assertStaleRecipeRef(id, "exclude", src)

		j.deliverMerge(merged)

		renamed := j.recipeNamed("Very Late")
		if renamed.ID != id {
			t.Fatalf("the rename wrote recipe %d, want the one under the cursor %d", renamed.ID, id)
		}
		j.assertMergedRecipeRefs("Very Late", other, dest)
		if got := len(j.recipes()); got != 1 {
			t.Errorf("storage holds %d recipes, want only the renamed one", got)
		}

		j.assertRestoredRecipeResolves("Very Late", other, dest)
	})

	t.Run("a retry of a refused save cannot restore the merged-away tag either", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		j.startFiltered(djMergeSettings(src, other))

		j.press("ctrl+a", "tab", "tab", "s")
		j.m = djTypeName(t, j.m, "Ghost")

		// The name is claimed after this panel read the list, so only storage can
		// refuse it — which is the path that reopens the box from the request.
		taken := j.saveRecipe("Ghost", exploreSettings())
		j.press("enter")
		if out := djOverlayText(t, j.m); !strings.Contains(out, "Save recipe as") {
			t.Fatalf("the refusal did not reopen the name box:\n%s", out)
		}

		// The merge commits while the reopened box is holding the same frozen
		// settings the refused attempt carried, and the retry is submitted before
		// the completion lands.
		merged := j.mergeCommit(src, dest)

		j.press("ctrl+u")
		j.m = djTypeName(t, j.m, "Ghost Two")
		j.press("enter")
		retried := j.recipeNamed("Ghost Two")
		j.assertStaleRecipeRef(retried.ID, "include", src)

		j.deliverMerge(merged)

		j.assertMergedRecipeRefs("Ghost Two", dest, other)
		// The recipe that collided is none of the retry's business, and the repair
		// moves only the reference the merge decided about.
		collided := j.recipeNamed("Ghost")
		if collided.ID != taken {
			t.Errorf("the retry overwrote the recipe it collided with: id %d, want %d", collided.ID, taken)
		}
		if got := collided.Settings.DiscoveryPercent; got != exploreSettings().DiscoveryPercent {
			t.Errorf("the repair rewrote the untouched recipe's settings: discovery %d, want %d",
				got, exploreSettings().DiscoveryPercent)
		}

		j.assertRestoredRecipeResolves("Ghost Two", dest, other)
	})

	t.Run("a refused repair is reported and retried", func(t *testing.T) {
		j, w := newRefusingJourney(t)
		src, dest, other := j.mergeFixtureTags()

		j.startFiltered(djMergeSettings(src, other))

		j.press("ctrl+a", "tab", "tab", "s")
		j.m = djTypeName(t, j.m, "Late Mix")

		merged := j.mergeCommit(src, dest)
		j.press("enter")
		saved := j.recipeNamed("Late Mix")
		j.assertStaleRecipeRef(saved.ID, "include", src)

		// The database refuses the repair the completion performs. The merge is
		// still a success — it is the stored recipe that is now behind.
		w.refFails = 1
		j.deliverMerge(merged)

		if !j.m.dj.staleRefs.pending() {
			t.Fatal("a refused repair was not recorded, so nothing will retry it")
		}
		if !djStatusMentions(j.m.autoDJPanelStatus(), "Merged tag not applied") {
			t.Errorf("panel status lines = %+v, want a standing line about the merged tag",
				j.m.autoDJPanelStatus())
		}
		if got := j.recipeTagRefs(saved.ID, "include"); !slices.Equal(got, []int64{src}) {
			t.Errorf("the refused repair left include refs %v; a refusal must not be reported as a repair", got)
		}

		// The bounded path the refusal is kept for.
		w.refFails = 0
		j.m = djDeliver(j.m, tickMsg{})
		if j.m.dj.staleRefs.pending() {
			t.Fatalf("the retry did not repair the stored references: %v", j.m.dj.staleRefs.err)
		}

		j.assertMergedRecipeRefs("Late Mix", dest, other)
		if djStatusMentions(j.m.autoDJPanelStatus(), "Merged tag not applied") {
			t.Errorf("panel status lines = %+v, want the repaired state to clear the notice",
				j.m.autoDJPanelStatus())
		}
		j.assertRestoredRecipeResolves("Late Mix", dest, other)
	})
}
