package db_test

import (
	"testing"
	"time"
)

func TestReadSession_NoRow(t *testing.T) {
	d := openMigratedDB(t)

	if _, err := d.Exec(`DELETE FROM session WHERE id = 1`); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row != nil {
		t.Errorf("got non-nil row, want nil when no session row exists")
	}
}

func TestReadSession_Default(t *testing.T) {
	d := openMigratedDB(t)

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row == nil {
		t.Fatal("got nil row, want seeded session row")
	}
	if row.RepeatMode != "none" {
		t.Errorf("RepeatMode = %q, want %q", row.RepeatMode, "none")
	}
	if row.Shuffle {
		t.Errorf("Shuffle = true, want false")
	}
	if row.CurrentHash != "" {
		t.Errorf("CurrentHash = %q, want empty", row.CurrentHash)
	}
	if row.PositionMs != 0 {
		t.Errorf("PositionMs = %d, want 0", row.PositionMs)
	}
}

func TestReadSession_WithValues(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hashX", "/music/x.flac")

	if _, err := d.Exec(`
UPDATE session SET repeat_mode='all', shuffle=1, current_hash='hashX', position_ms=42000, updated_at=?
WHERE id=1`, time.Now().Unix()); err != nil {
		t.Fatalf("update session: %v", err)
	}

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row == nil {
		t.Fatal("got nil row")
	}
	if row.RepeatMode != "all" {
		t.Errorf("RepeatMode = %q, want %q", row.RepeatMode, "all")
	}
	if !row.Shuffle {
		t.Errorf("Shuffle = false, want true")
	}
	if row.CurrentHash != "hashX" {
		t.Errorf("CurrentHash = %q, want %q", row.CurrentHash, "hashX")
	}
	if row.PositionMs != 42000 {
		t.Errorf("PositionMs = %d, want 42000", row.PositionMs)
	}
}
