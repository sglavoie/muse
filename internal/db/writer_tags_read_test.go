package db_test

import "testing"

func TestReadTags_OrderedByName(t *testing.T) {
	d, w := openTagDB(t)

	for _, name := range []string{"Zebra", "apple", "Mango"} {
		if _, err := w.CreateTag(name, ""); err != nil {
			t.Fatalf("CreateTag %q: %v", name, err)
		}
	}

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("got %d tags, want 3", len(tags))
	}
	want := []string{"apple", "Mango", "Zebra"}
	for i, tg := range tags {
		if tg.Name != want[i] {
			t.Errorf("tags[%d].Name = %q, want %q", i, tg.Name, want[i])
		}
	}
}

func TestReadTags_EmptyReturnsNonNilSlice(t *testing.T) {
	d, _ := openTagDB(t)

	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if tags == nil {
		t.Fatal("ReadTags returned nil slice, want non-nil empty slice")
	}
	if len(tags) != 0 {
		t.Fatalf("got %d tags, want 0", len(tags))
	}
}

func TestReadTagCounts(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	id, _ := w.CreateTag("chill", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.AddTrackTag("h2", id)

	counts, err := d.ReadTagCounts()
	if err != nil {
		t.Fatalf("ReadTagCounts: %v", err)
	}
	if counts[id] != 2 {
		t.Errorf("count[%d] = %d, want 2", id, counts[id])
	}
}

func TestReadTagCounts_Empty(t *testing.T) {
	d, _ := openTagDB(t)

	counts, err := d.ReadTagCounts()
	if err != nil {
		t.Fatalf("ReadTagCounts empty: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("got %d entries, want 0", len(counts))
	}
}

func TestReadTracksByTag(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")

	id, _ := w.CreateTag("focus", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.AddTrackTag("h2", id)

	rows, err := d.ReadTracksByTag(id)
	if err != nil {
		t.Fatalf("ReadTracksByTag: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
}

func TestReadTracksByTag_SoftDeletedTagExcluded(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("gone", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.DeleteTagSoft(id)

	rows, err := d.ReadTracksByTag(id)
	if err != nil {
		t.Fatalf("ReadTracksByTag: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows for soft-deleted tag, want 0", len(rows))
	}
}

func TestReadTagsForTrack_ExcludesSoftDeleted(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	id, _ := w.CreateTag("temp", "")
	_ = w.AddTrackTag("h1", id)
	_ = w.DeleteTagSoft(id)

	tags, err := d.ReadTagsForTrack("h1")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags for soft-deleted tag, want 0", len(tags))
	}
}

func TestReadTagsForTrack_EmptyReturnsNonNilSlice(t *testing.T) {
	d, _ := openTagDB(t)
	seedTrack(t, d, "h1", "/a.flac")

	tags, err := d.ReadTagsForTrack("h1")
	if err != nil {
		t.Fatalf("ReadTagsForTrack: %v", err)
	}
	if tags == nil {
		t.Fatal("ReadTagsForTrack returned nil slice, want non-nil empty slice")
	}
	if len(tags) != 0 {
		t.Fatalf("got %d tags, want 0", len(tags))
	}
}
