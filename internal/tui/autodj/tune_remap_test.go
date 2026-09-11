package autodj

import (
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// A tag merge can land while this overlay is open, because the confirmation is
// answered asynchronously and the user is free to open the panel in between.
// Everything the panel holds is a snapshot taken before that happened, and each
// one has a write path behind it — the draft commits, a recipe rename rewrites
// its whole stored row, a confirmed picker writes back into the draft — so a
// snapshot left naming the merged-away tag is a way to put that id back on disk
// after storage remapped it.

func TestRemapTagMovesTheDraftAndTheAppliedBaseline(t *testing.T) {
	p := newTestPanel(true)

	p = p.RemapTag(1, 2)

	if got := p.Draft().Include.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Errorf("the draft includes %v, want the kept tag [2]", got)
	}
	if got := p.AppliedSettings().Include.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Errorf("the baseline includes %v, want [2]", got)
	}
	if p.Dirty() {
		t.Error("the remap reads as an unapplied edit, so the panel would offer an Apply the user never asked for")
	}
	// A merge is about one tag and nothing else.
	if got := p.Draft().Include.Genres; !slices.Equal(got, []string{"jazz"}) {
		t.Errorf("the genre filter became %v, want it untouched", got)
	}
	if got := p.Draft().Exclude.Artists; !slices.Equal(got, []string{"Zoe"}) {
		t.Errorf("the artist filter became %v, want it untouched", got)
	}
}

func TestRemapTagDropsTheMergedAwayTagFromThePickerChoices(t *testing.T) {
	p := newTestPanel(true).RemapTag(1, 2)

	var ids []int64
	for _, tag := range p.catalog.Tags {
		ids = append(ids, tag.ID)
	}
	if slices.Contains(ids, int64(1)) {
		t.Errorf("the catalog still offers the merged-away tag: %v", ids)
	}
	if !slices.Equal(ids, []int64{2, 3}) {
		t.Errorf("the catalog offers %v, want the surviving tags [2 3] in their original order", ids)
	}
	if got := p.Unresolved(); len(got) != 0 {
		t.Errorf("the panel now refuses its own draft over %v", got)
	}
}

func TestRemapTagCollapsesADuplicateDestination(t *testing.T) {
	s := testSettings()
	// The filter already names both sides of the merge, which is the case a
	// blind substitution turns into the same tag listed twice.
	s.Include = radio.FilterSet{TagIDs: []int64{1, 2}}
	s.Exclude = radio.FilterSet{TagIDs: []int64{1, 3}}
	p := New(Options{Applied: s, Catalog: testCatalog(), Active: true})

	p = p.RemapTag(1, 2)

	if got := p.Draft().Include.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Errorf("the inclusion is %v, want the kept tag once [2]", got)
	}
	if got := p.Draft().Exclude.TagIDs; !slices.Equal(got, []int64{2, 3}) {
		t.Errorf("the exclusion is %v, want [2 3]: an exclusion stays an exclusion", got)
	}
}

func TestRemapTagMovesTheStoredRecipeSnapshots(t *testing.T) {
	saved := testSettings()
	saved.Include = radio.FilterSet{TagIDs: []int64{1}}
	saved.Exclude = radio.FilterSet{TagIDs: []int64{3}}
	p := New(Options{
		Applied: testSettings(),
		Catalog: testCatalog(),
		Active:  true,
		Recipes: []Recipe{{ID: 7, Name: "late night", Settings: saved}},
	})

	p = p.RemapTag(1, 2)

	// Renaming a recipe rewrites its whole stored row from this snapshot, so a
	// stale id here reaches storage on a gesture that is not about filters at
	// all.
	got := p.recipes[0].Settings
	if !slices.Equal(got.Include.TagIDs, []int64{2}) {
		t.Errorf("the recipe includes %v, want the kept tag [2]", got.Include.TagIDs)
	}
	if !slices.Equal(got.Exclude.TagIDs, []int64{3}) {
		t.Errorf("the recipe's unrelated exclusion became %v, want [3]", got.Exclude.TagIDs)
	}
}

func TestRemapTagFollowsAnOpenTagPickerIntoTheDraft(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeTags)
	p, _ = press(t, p, "enter")
	if p.picker == nil {
		t.Fatal("enter did not open the tag picker")
	}

	p = p.RemapTag(1, 2)

	if _, on := p.picker.chosen["1"]; on {
		t.Errorf("the picker still has the merged-away tag selected: %v", p.picker.chosen)
	}
	if _, on := p.picker.chosen["2"]; !on {
		t.Errorf("the picker's selection is %v, want the kept tag chosen in its place", p.picker.chosen)
	}
	for _, opt := range p.picker.options {
		if opt.key == "1" {
			t.Error("the picker still offers the merged-away tag as a row the user could re-add")
		}
	}

	// Confirming writes the picker's selection back over the draft, which is the
	// path that would otherwise reinstate the id storage just remapped.
	p, _ = press(t, p, "enter")
	if got := p.Draft().Include.TagIDs; !slices.Equal(got, []int64{2}) {
		t.Errorf("confirming the picker left the inclusion at %v, want [2]", got)
	}
}

func TestRemapTagLeavesAnUnrelatedPanelAlone(t *testing.T) {
	p := newTestPanel(true)

	// Neither side of this merge is anything the panel references.
	got := p.RemapTag(99, 100)

	if !got.Draft().Equal(p.Draft()) || !got.AppliedSettings().Equal(p.AppliedSettings()) {
		t.Errorf("an unrelated merge rewrote the settings: %+v", got.Draft())
	}
	if !slices.EqualFunc(got.catalog.Tags, p.catalog.Tags, func(a, b db.TagRow) bool { return a.ID == b.ID }) {
		t.Errorf("an unrelated merge changed the catalog: %v", got.catalog.Tags)
	}
}
