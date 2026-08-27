package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// TestWithRadioRecentHashesSeedsRing verifies that the restore-time option
// seeds radio's recent-history ring, so avoidance of tracks heard in the
// previous session survives a restart instead of starting empty.
func TestWithRadioRecentHashesSeedsRing(t *testing.T) {
	m := New(context.Background(), WithRadioRecentHashes([]string{"h1", "h2"}))

	set := m.radioRecentSet()
	if _, ok := set["h1"]; !ok {
		t.Errorf("expected seeded hash h1 in recent set, got %v", set)
	}
	if _, ok := set["h2"]; !ok {
		t.Errorf("expected seeded hash h2 in recent set, got %v", set)
	}
}

// TestWithRadioRecentHashesTrimsToCap verifies that an oversized seed keeps the
// newest entries (input is oldest-first) and never exceeds the ring capacity.
func TestWithRadioRecentHashesTrimsToCap(t *testing.T) {
	hashes := make([]string, RadioRecentCap+5)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("h%d", i)
	}

	m := New(context.Background(), WithRadioRecentHashes(hashes))

	if len(m.radio.recentHashes) != RadioRecentCap {
		t.Fatalf("ring length = %d, want %d", len(m.radio.recentHashes), RadioRecentCap)
	}
	if m.radio.recentHashes[0] != "h5" {
		t.Errorf("oldest kept = %q, want %q (oldest 5 dropped)", m.radio.recentHashes[0], "h5")
	}
	last := hashes[len(hashes)-1]
	if m.radio.recentHashes[RadioRecentCap-1] != last {
		t.Errorf("newest kept = %q, want %q", m.radio.recentHashes[RadioRecentCap-1], last)
	}
}

// TestSeededRecentHashPenalisedOnFirstBatch verifies the point of seeding: on
// the first post-restart scoring pass, a track carried in from play history
// scores below an otherwise identical track that was not recently heard.
func TestSeededRecentHashPenalisedOnFirstBatch(t *testing.T) {
	m := New(context.Background(), WithRadioRecentHashes([]string{"recent"}))

	tracks := []library.Track{
		{Hash: "recent", Artist: "Alice", Album: "A", Genre: "jazz"},
		{Hash: "fresh", Artist: "Alice", Album: "A", Genre: "jazz"},
	}
	scored := radio.ScoreAll(radio.ScoreInput{
		Seed:         radio.SeedTrack{Hash: "seed", Artist: "Alice", Album: "A", Genres: []string{"jazz"}},
		AllTracks:    tracks,
		RecentHashes: m.radioRecentSet(),
	})

	byHash := map[string]float64{}
	for _, s := range scored {
		byHash[s.Track.Hash] = s.Score
	}
	if byHash["recent"] >= byHash["fresh"] {
		t.Errorf("recently played track scored %v, want below fresh track %v",
			byHash["recent"], byHash["fresh"])
	}
}

// TestRadioBatchAppendsToSeededRing verifies that a radio batch extends the
// seeded ring rather than replacing it, so history-derived avoidance survives
// the first enqueue of the session.
func TestRadioBatchAppendsToSeededRing(t *testing.T) {
	m := newModelWithTracks()
	m.radio.enabled = true
	m = applyOptions(m, WithRadioRecentHashes([]string{"seeded"}))

	m, _ = m.handleRadioBatchEnqueuedMsg(RadioBatchEnqueuedMsg{
		Entries:  []QueueEntry{{ID: 1, Hash: "h1", Path: "/a1.flac"}},
		QueueIDs: []int64{1},
		WasEmpty: true,
	})

	set := m.radioRecentSet()
	if _, ok := set["seeded"]; !ok {
		t.Errorf("expected seeded hash to survive a batch, got %v", set)
	}
	if _, ok := set["h1"]; !ok {
		t.Errorf("expected enqueued hash h1 in recent set, got %v", set)
	}
}

func applyOptions(m Model, opts ...Option) Model {
	for _, opt := range opts {
		opt(&m)
	}
	return m
}
