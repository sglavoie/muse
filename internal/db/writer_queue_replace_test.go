package db_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestReplaceQueueAtomicAndUndoPreservesRows(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	seedTrack(t, d, "old", "/old")
	seedTrack(t, d, "new", "/new")
	old, err := w.AppendTracks([]library.Track{{Hash: "old"}, {Hash: "old"}}, "Original")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE queue SET dj_owned=1, dj_reasons='[]' WHERE id=?`, old[1]); err != nil {
		t.Fatal(err)
	}
	before, err := d.ReadQueue()
	if err != nil {
		t.Fatal(err)
	}
	// Failure after the clear and first insert must roll both back.
	if _, err := w.ReplaceQueue(old, []library.Track{{Hash: "new"}, {Hash: "missing"}}, "New"); err == nil {
		t.Fatal("expected FK failure")
	}
	after, err := d.ReadQueue()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed replacement changed queue: %v %+v", err, after)
	}
	ids, err := w.ReplaceQueue(old, []library.Track{{Hash: "new"}, {Hash: "new"}}, "New")
	if err != nil {
		t.Fatal(err)
	}
	after, err = d.ReadQueue()
	if err != nil || len(after) != 2 || after[0].Hash != "new" || after[1].ID != ids[1] {
		t.Fatalf("replacement: %v %+v", err, after)
	}
	if err := w.UndoReplaceQueue(ids, old); err != nil {
		t.Fatal(err)
	}
	restored, err := d.ReadQueue()
	if err != nil || !reflect.DeepEqual(before, restored) {
		t.Fatalf("undo lost identity/order/provenance: %v %+v", err, restored)
	}
}

func TestReplaceQueueRejectsUnacknowledgedAppend(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	seedTrack(t, d, "a", "/a")
	id, err := w.AppendTrack(library.Track{Hash: "a"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.ReplaceQueue(nil, []library.Track{{Hash: "a"}}, ""); err == nil {
		t.Fatal("unacknowledged append was discarded")
	}
	rows, err := d.ReadQueue()
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("queue changed: %v %+v", err, rows)
	}
}

func TestUndoReplaceQueueFailureIsAtomicAndRetryable(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	seedTrack(t, d, "a", "/a")
	old, err := w.AppendTracks([]library.Track{{Hash: "a"}, {Hash: "a"}}, "Old")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := w.ReplaceQueue(old, []library.Track{{Hash: "a"}}, "New")
	if err != nil {
		t.Fatal(err)
	}
	// Fail on the second restore, after the first has succeeded inside the tx.
	if _, err := d.Exec(`CREATE TRIGGER fail_restore BEFORE UPDATE OF deleted_at ON queue WHEN NEW.id=` + fmt.Sprint(old[1]) + ` AND NEW.deleted_at IS NULL BEGIN SELECT RAISE(ABORT, 'restore failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := w.UndoReplaceQueue(ids, old); err == nil {
		t.Fatal("expected restore failure")
	}
	rows, err := d.ReadQueue()
	if err != nil || len(rows) != 1 || rows[0].ID != ids[0] {
		t.Fatalf("partial undo: %v %+v", err, rows)
	}
	if _, err := d.Exec(`DROP TRIGGER fail_restore`); err != nil {
		t.Fatal(err)
	}
	if err := w.UndoReplaceQueue(ids, old); err != nil {
		t.Fatal(err)
	}
	rows, err = d.ReadQueue()
	if err != nil || len(rows) != 2 || rows[0].ID != old[0] || rows[1].ID != old[1] {
		t.Fatalf("retry: %v %+v", err, rows)
	}
}
