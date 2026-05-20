package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestReadMixCounts_OnRepeatExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 3 {
		if err := w.RecordPlay("h1", now-int64(i*3600), 60, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}

	counts, err := d.ReadMixCounts(db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadMixCounts: %v", err)
	}
	if got := mixCountByName(t, counts, "On Repeat"); got != 1 {
		t.Fatalf("On Repeat count before missing = %d, want 1", got)
	}

	markMissing(t, d, "h1")

	counts, err = d.ReadMixCounts(db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadMixCounts after missing: %v", err)
	}
	if got := mixCountByName(t, counts, "On Repeat"); got != 0 {
		t.Errorf("On Repeat count after missing = %d, want 0", got)
	}
}

func TestReadMixCounts_DeepCutsExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 162, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}

	counts, err := d.ReadMixCounts(db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadMixCounts: %v", err)
	}
	if got := mixCountByName(t, counts, "Deep Cuts"); got != 1 {
		t.Fatalf("Deep Cuts count before missing = %d, want 1", got)
	}

	markMissing(t, d, "h1")

	counts, err = d.ReadMixCounts(db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadMixCounts after missing: %v", err)
	}
	if got := mixCountByName(t, counts, "Deep Cuts"); got != 0 {
		t.Errorf("Deep Cuts count after missing = %d, want 0", got)
	}
}

func mixCountByName(t *testing.T, counts []db.MixStatEntry, name string) int {
	t.Helper()
	for _, count := range counts {
		if count.Name == name {
			return count.TrackCount
		}
	}
	t.Fatalf("mix %q not found", name)
	return 0
}
