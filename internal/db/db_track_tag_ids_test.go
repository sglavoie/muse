package db_test

import (
	"reflect"
	"testing"
)

func TestReadTrackTagIDs(t *testing.T) {
	d, w := openTagDB(t)
	got, err := d.ReadTrackTagIDs()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty cache = %v, %v", got, err)
	}
	seedTrack(t, d, "h1", "/a.flac")
	seedTrack(t, d, "h2", "/b.flac")
	first, err := w.CreateTag("First", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.CreateTag("Second", "")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := w.CreateTag("Deleted", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{first, second, deleted} {
		if err := w.AddTrackTag("h1", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.AddTrackTag("h2", second); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteTagSoft(deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = 'h2'`); err != nil {
		t.Fatal(err)
	}
	got, err = d.ReadTrackTagIDs()
	want := map[string]map[int64]struct{}{"h1": {first: {}, second: {}}, "h2": {second: {}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("assignments = %v, %v; want %v", got, err, want)
	}
}
