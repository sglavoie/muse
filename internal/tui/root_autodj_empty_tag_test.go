package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// The tests below pin the difference between a tag that holds nothing and a tag
// that is gone. Only the second one may retire a filter: the first is what a tag
// looks like while its files are unavailable, or while it waits for tracks the
// user has not imported yet, and an exclusion of it is a standing instruction
// about the future rather than a stale reference to the past.

// djEmptyTagTags is the cache both tests run on: two tags exist, and the second
// holds no playable track.
func djEmptyTagTags() []db.TagRow {
	return []db.TagRow{{ID: 1, Name: "chill"}, {ID: 2, Name: "hibernating"}}
}

// withEmptyTag installs that cache along with a populated — and therefore
// authoritative-looking — counts map that reports tag 2 at zero.
func withEmptyTag(m Model) Model {
	m = m.rebuildTagCache(djEmptyTagTags())
	m.tagsView.TagCounts = map[int64]int{1: 3, 2: 0}
	return m
}

func TestAutoDJEmptyActiveTagRemainsResolvable(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m = withEmptyTag(m)

	// The two lists answer two different questions, and the catalog carries both.
	cat := m.autoDJCatalog()
	if len(cat.Tags) != 1 || cat.Tags[0].ID != 1 {
		t.Fatalf("picker tags = %+v, want only the tag that holds tracks", cat.Tags)
	}
	if len(cat.KnownTags) != 2 {
		t.Fatalf("known tags = %+v, want both tags that exist", cat.KnownTags)
	}

	// The running session excludes the empty tag, which is a valid setting.
	settings := exploreSettings()
	settings.Exclude.TagIDs = []int64{2}
	m.dj.settings = settings

	m, _ = djPress(t, m, "ctrl+a")
	if got := djPanel(t, m).Unresolved(); len(got) != 0 {
		t.Fatalf("unresolved = %v, want none: tag 2 exists, it just holds nothing", got)
	}

	// It is named on the row rather than reported as a dangling id, because the
	// panel can still say what the filter means.
	page := djOverlayText(t, m)
	if !strings.Contains(page, "hibernating") {
		t.Fatalf("the exclusion does not name the empty tag:\n%s", page)
	}
	if strings.Contains(page, "#2 (missing)") {
		t.Fatalf("the empty tag is drawn as a missing reference:\n%s", page)
	}

	// And the picker offers it, so the filter can be taken off again. A row the
	// picker never draws would be a selection the user cannot reach.
	picker, _ := djPress(t, m, "down", "down", "down", "down", "down", "down", "enter")
	if out := djOverlayText(t, picker); !strings.Contains(out, "hibernating") {
		t.Fatalf("the exclusion picker does not offer the empty tag:\n%s", out)
	}

	// Applying goes through and keeps the filter.
	m, _ = djPress(t, m, "down", "left") // 80% -> 70%, so the commit is a change
	m, cmd := djPress(t, m, "ctrl+s")
	if cmd == nil {
		t.Fatal("Apply was refused over a tag that exists")
	}
	if got := m.dj.settings.Exclude.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Fatalf("exclude tags = %v, want the empty tag's filter in force", got)
	}
	m = djDeliver(m, cmd())
	panel := djPanel(t, m)
	if panel.Err() != nil {
		t.Fatalf("Apply reported %v", panel.Err())
	}
	if got := panel.AppliedSettings().Exclude.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Fatalf("applied exclude tags = %v, want the empty tag kept", got)
	}
}

// TestAutoDJDeletedTagIsStillBlocked is the other half: widening the filters
// behind the user's back is still refused, and the repair still only drops the
// reference that has nothing behind it.
func TestAutoDJDeletedTagIsStillBlocked(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m = withEmptyTag(m)
	settings := exploreSettings()
	settings.Exclude.TagIDs = []int64{2, 404}
	m.dj.settings = settings

	m, _ = djPress(t, m, "ctrl+a")
	m, cmd := djPress(t, m, "ctrl+s")

	if cmd != nil {
		t.Fatal("a draft referencing a deleted tag was committed")
	}
	panel := djPanel(t, m)
	if panel.Err() == nil {
		t.Fatal("the refusal was not shown")
	}
	if got := panel.Unresolved(); !slices.Equal(got, []int64{404}) {
		t.Fatalf("unresolved = %v, want only the tag that no longer exists", got)
	}

	m, _ = djPress(t, m, "r")
	if got := djPanel(t, m).Draft().Exclude.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Fatalf("exclude tags = %v after the repair, want the empty tag untouched", got)
	}
	if _, cmd := djPress(t, m, "ctrl+s"); cmd == nil {
		t.Fatal("the repaired draft was still refused")
	}
}

func TestAutoDJRecipeWithEmptyActiveTagLoads(t *testing.T) {
	empty := radio.DefaultSettings()
	empty.Include.TagIDs = []int64{2}
	deleted := radio.DefaultSettings()
	deleted.Include.TagIDs = []int64{404}
	store := newRecipeStore(
		storedRecipe(1, "Hibernation", empty),
		storedRecipe(2, "Ghost Tag", deleted))

	m := newDJModel(WithLibraryTracks(djTestTracks()),
		WithDJRecipeReader(store), WithDJRecipeWriter(store))
	m = withEmptyTag(m)
	m.playback.currentHash = "f2"
	m, _ = m.openAutoDJTunePanel()
	m, _ = djPress(t, m, "tab", "tab")
	if !djPanel(t, m).RecipesActive() {
		t.Fatal("the panel is not on the Recipes page")
	}

	// Exactly one of the two recipes is warned about, and it is not the one whose
	// tag merely went quiet.
	page := djOverlayText(t, m)
	if got := strings.Count(page, "no longer exist"); got != 1 {
		t.Fatalf("%d recipes warned about tags, want only the deleted one:\n%s", got, page)
	}
	hibernation, _, _ := strings.Cut(page, "Ghost Tag")
	if strings.Contains(hibernation, "no longer exist") {
		t.Fatalf("the recipe holding the empty tag is warned about:\n%s", page)
	}

	// Loading it keeps the reference, and committing it is an ordinary Start.
	m, _ = djPress(t, m, "enter")
	if got := djPanel(t, m).Draft().Include.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Fatalf("loaded include tags = %v, want the recipe's own filter", got)
	}
	m, _ = djPress(t, m, "ctrl+s")
	if !m.dj.active {
		t.Fatal("the loaded recipe was refused over a tag that exists")
	}
	if got := m.dj.settings.Include.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Fatalf("session include tags = %v, want the recipe's filter in force", got)
	}

	// The recipe that really does reference a deleted tag still cannot be applied.
	applied := m.dj.settings
	m, _ = djPress(t, m, "down", "enter")
	if got := djPanel(t, m).Draft().Include.TagIDs; !slices.Equal(got, []int64{404}) {
		t.Fatalf("loaded include tags = %v, want the deleted reference kept for the repair", got)
	}
	m, cmd := djPress(t, m, "ctrl+s")
	if cmd != nil || !m.dj.settings.Equal(applied) {
		t.Fatalf("a recipe referencing a deleted tag reached the session: %+v", m.dj.settings)
	}
	if got := djPanel(t, m).Unresolved(); !slices.Equal(got, []int64{404}) {
		t.Fatalf("unresolved = %v, want the deleted tag named", got)
	}
}
