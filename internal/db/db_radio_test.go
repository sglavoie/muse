package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestReadRecentPlayedHashes_OldestFirstDeduped verifies that the reader
// returns one entry per track, ordered oldest-first by the track's most recent
// counted play, so the result can be appended straight into radio's ring.
func TestReadRecentPlayedHashes_OldestFirstDeduped(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	for _, h := range []string{"h1", "h2", "h3"} {
		insertTrack(t, d, h, "/"+h+".flac")
	}
	w := newWriter(t, d)

	plays := []db.PlayRecord{
		{Hash: "h1", PlayedAt: now - 300, ListenedSecs: 60, Counted: true},
		{Hash: "h2", PlayedAt: now - 200, ListenedSecs: 60, Counted: true},
		{Hash: "h3", PlayedAt: now - 100, ListenedSecs: 60, Counted: true},
		// h1 heard again most recently: it must sort last, exactly once.
		{Hash: "h1", PlayedAt: now - 50, ListenedSecs: 60, Counted: true},
	}
	for _, p := range plays {
		if err := w.RecordListen(p); err != nil {
			t.Fatalf("RecordListen %s: %v", p.Hash, err)
		}
	}

	got, err := d.ReadRecentPlayedHashes(10)
	if err != nil {
		t.Fatalf("ReadRecentPlayedHashes: %v", err)
	}
	want := []string{"h2", "h3", "h1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestReadRecentPlayedHashes_SkipsExcluded verifies that uncounted plays
// (skips) are not treated as "recently heard": they are filtered out by the
// counted-only play_history view.
func TestReadRecentPlayedHashes_SkipsExcluded(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now - 100, ListenedSecs: 60, Counted: true}); err != nil {
		t.Fatalf("RecordListen h1: %v", err)
	}
	if err := w.RecordListen(db.PlayRecord{Hash: "h2", PlayedAt: now - 10, ListenedSecs: 5, Counted: false}); err != nil {
		t.Fatalf("RecordListen h2: %v", err)
	}

	got, err := d.ReadRecentPlayedHashes(10)
	if err != nil {
		t.Fatalf("ReadRecentPlayedHashes: %v", err)
	}
	if len(got) != 1 || got[0] != "h1" {
		t.Fatalf("got %v, want [h1]", got)
	}
}

// TestReadRecentPlayedHashes_LimitKeepsNewest verifies that the limit trims the
// oldest tracks rather than the newest ones.
func TestReadRecentPlayedHashes_LimitKeepsNewest(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	for _, h := range []string{"h1", "h2", "h3"} {
		insertTrack(t, d, h, "/"+h+".flac")
		if err := newWriter(t, d).RecordListen(db.PlayRecord{
			Hash: h, PlayedAt: now, ListenedSecs: 60, Counted: true,
		}); err != nil {
			t.Fatalf("RecordListen %s: %v", h, err)
		}
		now += 10
	}

	got, err := d.ReadRecentPlayedHashes(2)
	if err != nil {
		t.Fatalf("ReadRecentPlayedHashes: %v", err)
	}
	want := []string{"h2", "h3"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestReadRecentPlayedHashes_NonPositiveLimit verifies the reader short-circuits
// rather than emitting `LIMIT 0` / `LIMIT -1` semantics.
func TestReadRecentPlayedHashes_NonPositiveLimit(t *testing.T) {
	d := openMigratedDB(t)
	got, err := d.ReadRecentPlayedHashes(0)
	if err != nil {
		t.Fatalf("ReadRecentPlayedHashes: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}
