package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
)

// A recipe name box opened before a merge lands, over a real database.
//
// The Recipes tab is the one place in the overlay where a gesture that is not
// about filters at all rewrites a whole stored settings row. Opening the box
// freezes the settings the write will carry — the draft for a Save-as, the
// recipe's own snapshot for a Rename — and the merge the user confirmed a
// moment earlier is answered asynchronously, so it can land in between. Storage
// remapped dj_tag_refs inside the merge transaction; submitting a box that kept
// the old id hands the merged-away tag straight back to CreateRecipe or
// UpdateRecipe, and a Rename in particular overwrites the durable remap with a
// tag that no longer exists. The recipe then cannot be applied without repair.
//
// These use the journey harness because only there are the merge writer, the
// recipe writer, the dj_tag_refs rows, and a relaunch that re-reads them all
// real at once.

// saveRecipe stores a recipe straight through the writer, behind whatever the
// panel is holding, and returns its id.
func (j *djJourney) saveRecipe(name string, s radio.Settings) int64 {
	j.t.Helper()
	id, err := j.w.CreateRecipe(radio.Recipe{Name: name, Settings: s})
	if err != nil {
		j.t.Fatalf("save recipe %q: %v", name, err)
	}
	return id
}

// recipeNamed returns the stored recipe with the given name.
func (j *djJourney) recipeNamed(name string) radio.Recipe {
	j.t.Helper()
	var names []string
	for _, row := range j.recipes() {
		if row.Recipe.Name == name {
			if row.Invalid != nil {
				j.t.Fatalf("stored recipe %q is unreadable: %v", name, row.Invalid)
			}
			return row.Recipe
		}
		names = append(names, row.Recipe.Name)
	}
	j.t.Fatalf("no stored recipe named %q; storage holds %v", name, names)
	return radio.Recipe{}
}

// djTagRefs reads the durable tag references one owner holds in one role,
// ascending. These are the rows the merge transaction remapped and that any
// later write of the same owner rebuilds from whatever the Model was holding.
func (j *djJourney) djTagRefs(scope string, owner int64, role string) []int64 {
	j.t.Helper()
	rows, err := j.d.Query(
		`SELECT tag_id FROM dj_tag_refs
		 WHERE scope = ? AND owner_id = ? AND role = ? ORDER BY tag_id`, scope, owner, role)
	if err != nil {
		j.t.Fatalf("read %s %s tag refs: %v", scope, role, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			j.t.Fatalf("scan %s %s tag ref: %v", scope, role, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		j.t.Fatalf("read %s %s tag refs: %v", scope, role, err)
	}
	return ids
}

// recipeTagRefs reads one stored recipe's durable tag references in one role.
func (j *djJourney) recipeTagRefs(id int64, role string) []int64 {
	j.t.Helper()
	return j.djTagRefs("recipe", id, role)
}

// mergeFixtureTags seeds three tags, attaches each to tracks, and returns the
// two sides of the merge plus an unrelated tag that must survive it untouched.
func (j *djJourney) mergeFixtureTags() (src, dest, other int64) {
	j.t.Helper()
	src, dest, other = j.createTag("Late Night"), j.createTag("Loud"), j.createTag("Mellow")
	j.loadTags()
	j.tagTracks(src, j.m.playback.currentHash, "jt02")
	j.tagTracks(dest, "jt04")
	j.tagTracks(other, "jt06")
	return src, dest, other
}

func TestAutoDJTagMergeRemapsOpenRecipePrompts(t *testing.T) {
	t.Run("a Save-as prompt opened before the merge stores the kept tag", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		s := radio.DefaultSettings()
		// The inclusion already names both sides, which is the case a blind
		// substitution would store as the same tag twice.
		s.Include = radio.FilterSet{TagIDs: []int64{src, dest}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)

		// The box opens on a copy of the draft taken right here — before the merge
		// the user confirmed a moment ago has been answered.
		j.press("ctrl+a", "tab", "tab", "s")
		j.m = djTypeName(t, j.m, "Late Mix")

		j.merge(src, dest)
		j.press("enter")

		saved := j.recipeNamed("Late Mix")
		if got := saved.Settings.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the saved inclusion is %v, want the kept tag once [%d]", got, dest)
		}
		if got := saved.Settings.Exclude.TagIDs; !slices.Equal(got, []int64{other}) {
			t.Errorf("the save rewrote the unrelated exclusion to %v, want [%d]", got, other)
		}
		if got := j.recipeTagRefs(saved.ID, "include"); !slices.Equal(got, []int64{dest}) {
			t.Errorf("the durable include refs are %v, want [%d]", got, dest)
		}
		if got := j.recipeTagRefs(saved.ID, "exclude"); !slices.Equal(got, []int64{other}) {
			t.Errorf("the durable exclude refs are %v, want [%d]", got, other)
		}
		if err := saved.Settings.ResolveTags(j.m.djTagCatalog()); err != nil {
			t.Errorf("the saved recipe references a tag the catalog cannot resolve: %v", err)
		}

		// Reopening the file is what proves the row itself is right rather than
		// the list the panel happens to be holding.
		j.relaunch()
		j.loadTags()
		restored := j.recipeNamed("Late Mix")
		if got := restored.Settings.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the reopened database holds inclusion %v, want [%d]", got, dest)
		}
		if err := restored.Settings.ResolveTags(j.m.djTagCatalog()); err != nil {
			t.Errorf("the restored recipe cannot be applied: %v", err)
		}
	})

	t.Run("a Rename prompt opened before the merge does not undo the durable remap", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		stored := radio.DefaultSettings()
		stored.Include = radio.FilterSet{TagIDs: []int64{other}}
		// The exclusion names both sides: an exclusion stays an exclusion, and the
		// pair collapses onto the kept tag rather than being listed twice.
		stored.Exclude = radio.FilterSet{TagIDs: []int64{src, dest}}
		id := j.saveRecipe("Late Night", stored)

		// A rename carries the recipe's own snapshot, not the draft, so the draft
		// is deliberately left on unrelated settings.
		j.startFiltered(radio.DefaultSettings())
		j.press("ctrl+a", "tab", "tab", "n")

		j.merge(src, dest)

		if got := j.recipeTagRefs(id, "exclude"); !slices.Equal(got, []int64{dest}) {
			t.Fatalf("the merge transaction left the recipe's exclude refs %v, want [%d]", got, dest)
		}

		j.press("ctrl+u")
		j.m = djTypeName(t, j.m, "Very Late")
		j.press("enter")

		renamed := j.recipeNamed("Very Late")
		if renamed.ID != id {
			t.Fatalf("the rename wrote recipe %d, want the one under the cursor %d", renamed.ID, id)
		}
		if got := renamed.Settings.Exclude.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the rename stored exclusion %v, want the kept tag once [%d]", got, dest)
		}
		if got := renamed.Settings.Include.TagIDs; !slices.Equal(got, []int64{other}) {
			t.Errorf("the rename rewrote the unrelated inclusion to %v, want [%d]", got, other)
		}
		if got := j.recipeTagRefs(id, "exclude"); !slices.Equal(got, []int64{dest}) {
			t.Errorf("the rename wrote %v back over the merge's remap, want [%d]", got, dest)
		}
		if got := len(j.recipes()); got != 1 {
			t.Errorf("storage holds %d recipes, want only the renamed one", got)
		}

		j.relaunch()
		j.loadTags()
		restored := j.recipeNamed("Very Late")
		if got := restored.Settings.Exclude.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the reopened database holds exclusion %v, want [%d]", got, dest)
		}
		if err := restored.Settings.ResolveTags(j.m.djTagCatalog()); err != nil {
			t.Errorf("the restored recipe references a tag the catalog cannot resolve: %v", err)
		}
	})

	t.Run("a refused save reopens its prompt and the retry cannot restore the merged-away tag", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.mergeFixtureTags()

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)

		j.press("ctrl+a", "tab", "tab", "s")
		j.m = djTypeName(t, j.m, "Ghost")

		// The name is claimed after this panel read the list, so only storage can
		// refuse it — which is the path that reopens the box from the request.
		taken := j.saveRecipe("Ghost", exploreSettings())
		j.press("enter")

		out := djOverlayText(t, j.m)
		if !strings.Contains(out, "Save recipe as") || !strings.Contains(out, "Ghost") {
			t.Fatalf("the refusal did not reopen the name box:\n%s", out)
		}
		if got := len(j.recipes()); got != 1 {
			t.Fatalf("the refused save still wrote: storage holds %d recipes", got)
		}

		// The merge lands on the reopened box, which is holding the same frozen
		// settings the refused attempt carried.
		j.merge(src, dest)

		j.press("ctrl+u")
		j.m = djTypeName(t, j.m, "Ghost Two")
		j.press("enter")

		retried := j.recipeNamed("Ghost Two")
		if got := retried.Settings.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the retry stored inclusion %v, want the kept tag [%d]", got, dest)
		}
		if got := retried.Settings.Exclude.TagIDs; !slices.Equal(got, []int64{other}) {
			t.Errorf("the retry rewrote the unrelated exclusion to %v, want [%d]", got, other)
		}
		if got := j.recipeTagRefs(retried.ID, "include"); !slices.Equal(got, []int64{dest}) {
			t.Errorf("the durable include refs are %v, want [%d]", got, dest)
		}
		// The recipe that collided is none of the retry's business.
		if got := j.recipeNamed("Ghost").ID; got != taken {
			t.Errorf("the retry overwrote the recipe it collided with: id %d, want %d", got, taken)
		}

		j.relaunch()
		j.loadTags()
		restored := j.recipeNamed("Ghost Two")
		if got := restored.Settings.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the reopened database holds inclusion %v, want [%d]", got, dest)
		}
		if err := restored.Settings.ResolveTags(j.m.djTagCatalog()); err != nil {
			t.Errorf("the restored recipe cannot be applied: %v", err)
		}
	})
}
