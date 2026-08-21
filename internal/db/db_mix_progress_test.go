package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
)

// Deep Cuts is the one mix that more listening alone never makes ready: its
// readiness is measured in skips, so its progress entry counts those and
// nothing else.
func TestReadAllMixProgressDeepCutsCountsSkips(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 4 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + int64(i), ListenedSecs: 170, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}
	for i := range 3 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now + 10 + int64(i), ListenedSecs: 5, DurationSecs: 180}); err != nil {
			t.Fatalf("RecordListen(skip): %v", err)
		}
	}

	progress, err := d.ReadAllMixProgress()
	if err != nil {
		t.Fatalf("ReadAllMixProgress: %v", err)
	}

	got := progress[db.MixDeepCuts]
	want := db.MixProgress{
		Current: 3,
		Needed:  insights.DeepCutsMinSkips,
		Description: fmt.Sprintf("3 skipped tracks recorded (need %d+ before completion can discriminate)",
			insights.DeepCutsMinSkips),
	}
	if got != want {
		t.Errorf("deep cuts progress = %+v, want %+v", got, want)
	}
}
