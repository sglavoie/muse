package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestSetQueueOrder_IgnoresSoftDeletedIDs(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "set-queue-soft-1", "/set-queue-soft-1")
	seedTrack(t, d, "set-queue-soft-2", "/set-queue-soft-2")
	seedTrack(t, d, "set-queue-soft-3", "/set-queue-soft-3")
	id1 := seedQueueEntry(t, d, "set-queue-soft-1", 1000)
	id2 := seedQueueEntry(t, d, "set-queue-soft-2", 2000)
	id3 := seedQueueEntry(t, d, "set-queue-soft-3", 3000)

	if _, err := d.Exec(`UPDATE queue SET deleted_at=? WHERE id=?`, time.Now().Unix(), id2); err != nil {
		t.Fatalf("soft-delete queue row: %v", err)
	}

	if err := w.SetQueueOrder([]int64{id3, id2, id1}); err != nil {
		t.Fatalf("SetQueueOrder: %v", err)
	}

	got, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("active queue row count = %d, want 2", len(got))
	}
	if got[0].ID != id3 || got[0].Position != 1000 || got[1].ID != id1 || got[1].Position != 2000 {
		t.Errorf("active queue order = [{id:%d pos:%d} {id:%d pos:%d}], want id3@1000 then id1@2000",
			got[0].ID, got[0].Position, got[1].ID, got[1].Position)
	}

	var deletedAt any
	if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, id2).Scan(&deletedAt); err != nil {
		t.Fatalf("query deleted row: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("soft-deleted queue row was restored; want it to stay deleted")
	}
}
