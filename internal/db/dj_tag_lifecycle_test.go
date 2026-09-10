package db_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// TestTagRename_PreservesDJReferences: settings reference tags by id, so a
// rename must be invisible to saved DJ state.
func TestTagRename_PreservesDJReferences(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	recipeID := makeRecipe(t, w, "Late Night", settings)
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(rock, live)})

	if err := w.RenameTag(rock, "post-rock"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}

	session := readDJSession(t, d)
	if want := []int64{rock}; !reflect.DeepEqual(session.Settings.Include.TagIDs, want) {
		t.Errorf("session include tag ids = %v after rename, want %v", session.Settings.Include.TagIDs, want)
	}
	if want := []int64{rock, live}; !reflect.DeepEqual(session.Seed.TagIDs, want) {
		t.Errorf("seed tag ids = %v after rename, want %v", session.Seed.TagIDs, want)
	}
	if err := session.Settings.ResolveTags(tagCatalog(t, d)); err != nil {
		t.Errorf("session settings unresolved after a rename: %v", err)
	}

	recipe := readRecipe(t, d, recipeID)
	if err := recipe.Recipe.Settings.ResolveTags(tagCatalog(t, d)); err != nil {
		t.Errorf("recipe settings unresolved after a rename: %v", err)
	}
}

// TestTagMerge_RemapsDJReferences: merging folds one tag into another, so every
// DJ reference — active session, active recipe, soft-deleted recipe, and the
// captured seed — must follow the surviving id in the same transaction.
func TestTagMerge_RemapsDJReferences(t *testing.T) {
	d, w := openTagDB(t)
	source := makeTag(t, w, "shoegaze")
	dest := makeTag(t, w, "shoe gaze")
	unrelated := makeTag(t, w, "live")

	settings := radio.Settings{
		DiscoveryPercent: 40,
		Anchor:           radio.AnchorSeed,
		Include:          radio.FilterSet{TagIDs: []int64{source}},
		Exclude:          radio.FilterSet{TagIDs: []int64{unrelated}},
	}
	activeRecipe := makeRecipe(t, w, "Active", settings)
	deletedRecipe := makeRecipe(t, w, "Deleted", settings)
	if err := w.DeleteRecipeSoft(deletedRecipe); err != nil {
		t.Fatalf("DeleteRecipeSoft: %v", err)
	}
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(source)})

	if err := w.MergeTags(source, dest); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE tag_id = ?`, source); n != 0 {
		t.Errorf("references to the merged-away tag = %d, want 0", n)
	}

	session := readDJSession(t, d)
	if want := []int64{dest}; !reflect.DeepEqual(session.Settings.Include.TagIDs, want) {
		t.Errorf("session include tag ids = %v, want %v", session.Settings.Include.TagIDs, want)
	}
	if want := []int64{dest}; !reflect.DeepEqual(session.Seed.TagIDs, want) {
		t.Errorf("seed tag ids = %v, want %v", session.Seed.TagIDs, want)
	}
	if want := []int64{unrelated}; !reflect.DeepEqual(session.Settings.Exclude.TagIDs, want) {
		t.Errorf("session exclude tag ids = %v, want %v (untouched)", session.Settings.Exclude.TagIDs, want)
	}
	// A merged reference must be usable immediately, not left dangling.
	if err := session.Settings.ResolveTags(tagCatalog(t, d)); err != nil {
		t.Errorf("session settings unresolved after a merge: %v", err)
	}

	recipe := readRecipe(t, d, activeRecipe)
	if want := []int64{dest}; !reflect.DeepEqual(recipe.Recipe.Settings.Include.TagIDs, want) {
		t.Errorf("active recipe include tag ids = %v, want %v", recipe.Recipe.Settings.Include.TagIDs, want)
	}

	// The soft-deleted recipe is remapped too, so undoing its deletion restores
	// a working recipe rather than one filtering on a tag that no longer exists.
	if err := w.UndoDeleteRecipe(deletedRecipe); err != nil {
		t.Fatalf("UndoDeleteRecipe: %v", err)
	}
	restored := readRecipe(t, d, deletedRecipe)
	if want := []int64{dest}; !reflect.DeepEqual(restored.Recipe.Settings.Include.TagIDs, want) {
		t.Errorf("restored recipe include tag ids = %v, want %v", restored.Recipe.Settings.Include.TagIDs, want)
	}
	if err := restored.Recipe.Settings.ResolveTags(tagCatalog(t, d)); err != nil {
		t.Errorf("restored recipe unresolved after a merge: %v", err)
	}
}

// TestTagMerge_CollapsesDuplicateDJReference covers an owner that already
// references the destination: the remap must not create a second row, and must
// not leave the source row behind either.
func TestTagMerge_CollapsesDuplicateDJReference(t *testing.T) {
	d, w := openTagDB(t)
	source := makeTag(t, w, "shoegaze")
	dest := makeTag(t, w, "shoe gaze")

	settings := radio.Settings{
		DiscoveryPercent: 40,
		Anchor:           radio.AnchorSeed,
		Include:          radio.FilterSet{TagIDs: []int64{source, dest}},
	}
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(source, dest)})

	if err := w.MergeTags(source, dest); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	session := readDJSession(t, d)
	if want := []int64{dest}; !reflect.DeepEqual(session.Settings.Include.TagIDs, want) {
		t.Errorf("include tag ids = %v, want %v", session.Settings.Include.TagIDs, want)
	}
	if want := []int64{dest}; !reflect.DeepEqual(session.Seed.TagIDs, want) {
		t.Errorf("seed tag ids = %v, want %v", session.Seed.TagIDs, want)
	}
}

// TestTagMerge_FailureLeavesDJReferencesUntouched: the remap shares the merge
// transaction, so a rejected merge must roll the whole thing back.
func TestTagMerge_FailureLeavesDJReferencesUntouched(t *testing.T) {
	d, w := openTagDB(t)
	source := makeTag(t, w, "shoegaze")
	dest := makeTag(t, w, "shoe gaze")

	settings := radio.Settings{
		DiscoveryPercent: 40,
		Anchor:           radio.AnchorSeed,
		Include:          radio.FilterSet{TagIDs: []int64{source}},
	}
	writeDJSession(t, w, db.DJSession{Settings: settings})

	if err := w.DeleteTagSoft(dest); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}
	if err := w.MergeTags(source, dest); err == nil {
		t.Fatal("MergeTags into a soft-deleted destination succeeded, want an error")
	}

	session := readDJSession(t, d)
	if want := []int64{source}; !reflect.DeepEqual(session.Settings.Include.TagIDs, want) {
		t.Errorf("include tag ids = %v after a failed merge, want %v", session.Settings.Include.TagIDs, want)
	}
}

// TestTagDelete_LeavesUnresolvedReferenceAndUndoRestores is the deletion half of
// the contract: the reference stays exactly where it was, reads as unresolved
// against the active catalog, and becomes valid again on undo — the filter is
// never quietly dropped, which would widen selection behind the user's back.
func TestTagDelete_LeavesUnresolvedReferenceAndUndoRestores(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	recipeID := makeRecipe(t, w, "Late Night", settings)
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(rock)})

	if err := w.DeleteTagSoft(rock); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	session := readDJSession(t, d)
	if want := []int64{rock}; !reflect.DeepEqual(session.Settings.Include.TagIDs, want) {
		t.Errorf("include tag ids = %v after deleting the tag, want %v (reference preserved)",
			session.Settings.Include.TagIDs, want)
	}

	catalog := tagCatalog(t, d)
	err := session.Settings.ResolveTags(catalog)
	if !errors.Is(err, radio.ErrUnresolvedTag) {
		t.Fatalf("ResolveTags = %v, want ErrUnresolvedTag", err)
	}
	var unresolved *radio.UnresolvedTagError
	if !errors.As(err, &unresolved) {
		t.Fatalf("ResolveTags error type = %T, want *radio.UnresolvedTagError", err)
	}
	if want := []int64{rock}; !reflect.DeepEqual(unresolved.TagIDs, want) {
		t.Errorf("unresolved ids = %v, want %v", unresolved.TagIDs, want)
	}

	recipe := readRecipe(t, d, recipeID)
	if !errors.Is(recipe.Recipe.Settings.ResolveTags(catalog), radio.ErrUnresolvedTag) {
		t.Error("recipe settings resolved despite referencing a deleted tag")
	}
	// The recipe row itself is still valid: an unresolved reference is a
	// planning-time problem to report, not corrupt storage.
	if !recipe.Usable() {
		t.Errorf("recipe marked invalid by a deleted tag reference: %v", recipe.Invalid)
	}

	if err := w.UndoDeleteTag(rock); err != nil {
		t.Fatalf("UndoDeleteTag: %v", err)
	}
	restored := tagCatalog(t, d)
	if err := readDJSession(t, d).Settings.ResolveTags(restored); err != nil {
		t.Errorf("session settings still unresolved after undo: %v", err)
	}
	if err := readRecipe(t, d, recipeID).Recipe.Settings.ResolveTags(restored); err != nil {
		t.Errorf("recipe settings still unresolved after undo: %v", err)
	}
}

// TestTagPurge_LeavesUnresolvedReference: dj_tag_refs deliberately carries no
// foreign key, so hard-deleting a tag cannot cascade a restrictive filter away.
// The reference survives and keeps reading as unresolved.
func TestTagPurge_LeavesUnresolvedReference(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	writeDJSession(t, w, db.DJSession{Settings: settings})

	if err := w.DeleteTagSoft(rock); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}
	if err := w.PurgeDeletedTags(time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("PurgeDeletedTags: %v", err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM tags WHERE id = ?`, rock); n != 0 {
		t.Fatalf("tag rows = %d after purge, want 0", n)
	}

	session := readDJSession(t, d)
	if want := []int64{rock}; !reflect.DeepEqual(session.Settings.Include.TagIDs, want) {
		t.Errorf("include tag ids = %v after purge, want %v (reference preserved)",
			session.Settings.Include.TagIDs, want)
	}
	if !errors.Is(session.Settings.ResolveTags(tagCatalog(t, d)), radio.ErrUnresolvedTag) {
		t.Error("purged tag reference resolved; it must read as unresolved")
	}
}

func TestReadTagCatalog_ExcludesDeletedTags(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	if err := w.DeleteTagSoft(live); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	catalog := tagCatalog(t, d)
	if !catalog.HasTag(rock) {
		t.Errorf("catalog is missing active tag %d", rock)
	}
	if catalog.HasTag(live) {
		t.Errorf("catalog contains soft-deleted tag %d", live)
	}
}
