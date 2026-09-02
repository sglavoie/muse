package statusbar

import (
	"testing"
	"time"
)

func TestLongOpStateRateAndETA(t *testing.T) {
	start := time.Unix(0, 0)
	s := LongOpState{Active: true, Label: "Scan", StartedAt: start}
	// 0 items at t=0
	s = s.Update(0, 1000, start)
	if r := s.rate(); r != 0 {
		t.Fatalf("rate with single sample must be 0; got %.2f", r)
	}
	// 100 items 1s later → 100 items/sec
	s = s.Update(100, 1000, start.Add(time.Second))
	rate := s.rate()
	if rate < 99 || rate > 101 {
		t.Fatalf("expected rate ~100, got %.2f", rate)
	}
	eta := s.etaSeconds()
	if eta < 8 || eta > 10 {
		t.Fatalf("expected ETA ~9s (remaining 900 / 100/s); got %d", eta)
	}
	// Samples older than the window are evicted.
	s = s.Update(200, 1000, start.Add(5*time.Second))
	for _, sample := range s.samples {
		if sample.at.Before(start.Add(5*time.Second - longOpRateWindow)) {
			t.Fatalf("sample at %v should have been evicted (cutoff %v)",
				sample.at, start.Add(5*time.Second-longOpRateWindow))
		}
	}
}

func TestLongOpStateETAZeroWhenFinishedOrUnknown(t *testing.T) {
	s := LongOpState{Active: true}
	if s.etaSeconds() != 0 {
		t.Fatal("ETA must be 0 when Total is unknown")
	}
	s = s.Update(10, 10, time.Now())
	if s.etaSeconds() != 0 {
		t.Fatal("ETA must be 0 when Done >= Total")
	}
}

// TestLongOpStateUpdateDoesNotAliasSnapshot guards against a regression where
// update() reused the receiver's samples backing array, allowing a later
// update on a derived state to silently mutate a previously taken snapshot.
// Bubble Tea passes Models by value and may retain prior Model copies inside
// deferred tea.Cmd closures, so any in-place mutation of the samples slice
// would corrupt those snapshots.
func TestLongOpStateUpdateDoesNotAliasSnapshot(t *testing.T) {
	start := time.Unix(0, 0)
	s := LongOpState{Active: true, StartedAt: start}
	s = s.Update(0, 1000, start)
	s = s.Update(50, 1000, start.Add(500*time.Millisecond))

	// Snapshot before further updates. snapshot.samples must remain stable
	// even as we keep updating s with new (and eventually evicting) samples.
	snapshot := s
	wantLen := len(snapshot.samples)
	wantFirstDone := snapshot.samples[0].done
	wantFirstAt := snapshot.samples[0].at

	// Push enough updates that the eviction path runs and the backing array
	// would be overwritten in-place under the old implementation.
	s = s.Update(100, 1000, start.Add(time.Second))
	s = s.Update(200, 1000, start.Add(5*time.Second)) // forces eviction of pre-3s samples
	_ = s.Update(300, 1000, start.Add(6*time.Second))

	if len(snapshot.samples) != wantLen {
		t.Fatalf("snapshot.samples length changed after later update: got %d want %d",
			len(snapshot.samples), wantLen)
	}
	if snapshot.samples[0].done != wantFirstDone || !snapshot.samples[0].at.Equal(wantFirstAt) {
		t.Fatalf("snapshot.samples[0] mutated: got %+v want done=%d at=%v",
			snapshot.samples[0], wantFirstDone, wantFirstAt)
	}
}

// TestLongOpStateRateAfterEviction verifies the rate calculation remains
// correct after the eviction path runs and the sample window is rebuilt.
func TestLongOpStateRateAfterEviction(t *testing.T) {
	start := time.Unix(0, 0)
	s := LongOpState{Active: true, StartedAt: start}
	// Seed several samples that will all be evicted.
	s = s.Update(0, 1000, start)
	s = s.Update(10, 1000, start.Add(100*time.Millisecond))
	s = s.Update(20, 1000, start.Add(200*time.Millisecond))

	// Jump well past the window — only this sample survives eviction.
	s = s.Update(500, 1000, start.Add(10*time.Second))
	if got := s.rate(); got != 0 {
		t.Fatalf("rate after eviction-down-to-one-sample must be 0; got %.2f", got)
	}
	// Add another sample within the window — rate now computable.
	s = s.Update(600, 1000, start.Add(11*time.Second))
	if got := s.rate(); got < 99 || got > 101 {
		t.Fatalf("expected rate ~100 (600-500 over 1s); got %.2f", got)
	}
}
