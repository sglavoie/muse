package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
)

// openDeepCutsGate records n skipped listens so ReadDeepCutsMix stops
// reporting itself not-ready. They go against a filler track that keeps
// play_count 0, which puts it outside the candidate range: the gate opens
// without the filler ever competing for a slot in the mix.
func openDeepCutsGate(t *testing.T, d *db.DB, w *db.Writer, n int) {
	t.Helper()
	now := time.Now().Unix()
	insertFullTrack(t, d, "skipfiller", "/skipfiller.flac", "Skipped", "Artist", "Album", "Rock", 180000, 5000000, now)
	for i := range n {
		if err := w.RecordListen(db.PlayRecord{
			Hash: "skipfiller", PlayedAt: now + int64(i), ListenedSecs: 4, DurationSecs: 180,
		}); err != nil {
			t.Fatalf("RecordListen(skip %d): %v", i, err)
		}
	}
}

func TestReadDeepCutsMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadDeepCutsMix_HighCompletionRate(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	openDeepCutsGate(t, d, w, insights.DeepCutsMinSkips)

	for i := range 2 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 162, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadDeepCutsMix = %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadDeepCutsMix_LowCompletionRateExcluded(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	openDeepCutsGate(t, d, w, insights.DeepCutsMinSkips)

	for i := range 2 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 90, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0 (low completion rate)", len(tracks))
	}
}

func TestReadDeepCutsMix_ExcludesHighPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	openDeepCutsGate(t, d, w, insights.DeepCutsMinSkips)

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 162, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 6 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0 (play_count > 5)", len(tracks))
	}
}

func TestReadDeepCutsMix_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	openDeepCutsGate(t, d, w, insights.DeepCutsMinSkips)

	for i := range 2 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 162, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("precondition: expected 1 qualifying track, got %d", len(tracks))
	}

	markMissing(t, d, "h1")

	tracks, err = d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix after missing: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix returned %d tracks, want 0 (track is missing)", len(tracks))
	}
}

func TestReadDeepCutsMix_GatedUntilSkipsRecorded(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	openDeepCutsGate(t, d, w, insights.DeepCutsMinSkips-1)

	for i := range 2 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 162, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %v with %d skips recorded, want 0 (below the readiness gate)",
			trackHashes(tracks), insights.DeepCutsMinSkips-1)
	}

	// One more skip is all that separates a mix that cannot discriminate from
	// one that can.
	if err := w.RecordListen(db.PlayRecord{Hash: "skipfiller", PlayedAt: now, ListenedSecs: 4, DurationSecs: 180}); err != nil {
		t.Fatalf("RecordListen(final skip): %v", err)
	}

	tracks, err = d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix after gate opens: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadDeepCutsMix = %v at the gate threshold, want [h1]", trackHashes(tracks))
	}
}

func TestReadDeepCutsMix_SkippedListensDragAverageDown(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	openDeepCutsGate(t, d, w, insights.DeepCutsMinSkips)

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 162, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("precondition: expected h1 to qualify on its counted play alone, got %v", trackHashes(tracks))
	}

	// The skip is invisible to the play_history view, so reading it would
	// leave h1 in the mix at its unchanged 0.9 average.
	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + 1, ListenedSecs: 5, DurationSecs: 180}); err != nil {
		t.Fatalf("RecordListen(skip): %v", err)
	}

	tracks, err = d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix after skip: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %v, want 0 (a skip halves the completion average)", trackHashes(tracks))
	}
}
