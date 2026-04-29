package db_test

import "testing"

func TestDismissTrack_Idempotent(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	if err := w.DismissTrack("h1"); err != nil {
		t.Fatalf("first DismissTrack: %v", err)
	}
	if err := w.DismissTrack("h1"); err != nil {
		t.Fatalf("second DismissTrack (idempotent): %v", err)
	}

	hashes, err := d.ReadDismissedTrackHashes()
	if err != nil {
		t.Fatalf("ReadDismissedTrackHashes: %v", err)
	}
	if _, ok := hashes["h1"]; !ok {
		t.Error("expected h1 in dismissed set after idempotent dismiss")
	}
	if len(hashes) != 1 {
		t.Errorf("expected 1 dismissed hash, got %d", len(hashes))
	}
}

func TestUndismissTrack_NoOp(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	if err := w.UndismissTrack("nonexistent"); err != nil {
		t.Fatalf("UndismissTrack on nonexistent hash: %v", err)
	}

	hashes, err := d.ReadDismissedTrackHashes()
	if err != nil {
		t.Fatalf("ReadDismissedTrackHashes: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("expected empty dismissed set, got %d", len(hashes))
	}
}

func TestDismissUndismiss_RoundTrip(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	// Dismiss two tracks.
	for _, h := range []string{"h1", "h2"} {
		if err := w.DismissTrack(h); err != nil {
			t.Fatalf("DismissTrack(%s): %v", h, err)
		}
	}

	hashes, err := d.ReadDismissedTrackHashes()
	if err != nil {
		t.Fatalf("ReadDismissedTrackHashes: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("expected 2 dismissed, got %d", len(hashes))
	}

	// Undismiss one.
	if err := w.UndismissTrack("h1"); err != nil {
		t.Fatalf("UndismissTrack(h1): %v", err)
	}

	hashes, err = d.ReadDismissedTrackHashes()
	if err != nil {
		t.Fatalf("ReadDismissedTrackHashes after undismiss: %v", err)
	}
	if _, ok := hashes["h1"]; ok {
		t.Error("h1 should no longer be dismissed")
	}
	if _, ok := hashes["h2"]; !ok {
		t.Error("h2 should still be dismissed")
	}
}

func TestReadDismissedTrackHashes_EmptyReturnsNonNil(t *testing.T) {
	d := openMigratedDB(t)

	hashes, err := d.ReadDismissedTrackHashes()
	if err != nil {
		t.Fatalf("ReadDismissedTrackHashes: %v", err)
	}
	if hashes == nil {
		t.Error("expected non-nil empty map, got nil")
	}
}
