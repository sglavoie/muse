package db_test

import "testing"

func TestCreateTag_Basic(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("workout", "#ff0000")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if id <= 0 {
		t.Errorf("id = %d, want > 0", id)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("got %d tags, want 1", len(tags))
	}
	if tags[0].Name != "workout" || tags[0].Color != "#ff0000" {
		t.Errorf("unexpected tag %+v", tags[0])
	}
}

func TestCreateTag_DuplicateNameErrors(t *testing.T) {
	_, w := openTagDB(t)

	if _, err := w.CreateTag("chill", ""); err != nil {
		t.Fatalf("first CreateTag: %v", err)
	}
	if _, err := w.CreateTag("chill", ""); err == nil {
		t.Error("expected error on duplicate name, got nil")
	}
}

func TestCreateTag_CaseInsensitiveDuplicateErrors(t *testing.T) {
	_, w := openTagDB(t)

	if _, err := w.CreateTag("Chill", ""); err != nil {
		t.Fatalf("first CreateTag: %v", err)
	}
	if _, err := w.CreateTag("chill", ""); err == nil {
		t.Error("expected error on case-insensitive duplicate, got nil")
	}
}

func TestRenameTag(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("old", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.RenameTag(id, "new"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "new" {
		t.Errorf("got %+v, want name=new", tags)
	}
}

func TestUpdateTagColor(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("focus", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.UpdateTagColor(id, "#00ff00"); err != nil {
		t.Fatalf("UpdateTagColor: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Color != "#00ff00" {
		t.Errorf("got %+v, want color=#00ff00", tags)
	}
}

func TestDeleteTagSoft_HidesFromReadTags(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("driving", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags after soft-delete, want 0", len(tags))
	}
}

func TestUndoDeleteTag_RestoresTag(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("driving", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}
	if err := w.UndoDeleteTag(id); err != nil {
		t.Fatalf("UndoDeleteTag: %v", err)
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 1 {
		t.Errorf("got %d tags after undo, want 1", len(tags))
	}
}

func TestPurgeDeletedTags_RemovesOld(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("old-tag", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	if err := w.PurgeDeletedTags(9999999999); err != nil {
		t.Fatalf("PurgeDeletedTags: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tags WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("tag still exists after purge")
	}
}

func TestPurgeDeletedTags_SparesFutureDeletions(t *testing.T) {
	d, w := openTagDB(t)

	id, err := w.CreateTag("keep-me", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	if err := w.PurgeDeletedTags(0); err != nil {
		t.Fatalf("PurgeDeletedTags: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tags WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("tag was incorrectly purged")
	}
}
