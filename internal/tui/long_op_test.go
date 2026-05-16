package tui

import (
	"testing"
	"time"
)

func TestLongOpStateRateAndETA(t *testing.T) {
	start := time.Unix(0, 0)
	s := longOpState{Active: true, Label: "Scan", StartedAt: start}
	// 0 items at t=0
	s = s.update(0, 1000, start)
	if r := s.rate(); r != 0 {
		t.Fatalf("rate with single sample must be 0; got %.2f", r)
	}
	// 100 items 1s later → 100 items/sec
	s = s.update(100, 1000, start.Add(time.Second))
	rate := s.rate()
	if rate < 99 || rate > 101 {
		t.Fatalf("expected rate ~100, got %.2f", rate)
	}
	eta := s.etaSeconds()
	if eta < 8 || eta > 10 {
		t.Fatalf("expected ETA ~9s (remaining 900 / 100/s); got %d", eta)
	}
	// Samples older than the window are evicted.
	s = s.update(200, 1000, start.Add(5*time.Second))
	for _, sample := range s.samples {
		if sample.at.Before(start.Add(5*time.Second - longOpRateWindow)) {
			t.Fatalf("sample at %v should have been evicted (cutoff %v)",
				sample.at, start.Add(5*time.Second-longOpRateWindow))
		}
	}
}

func TestLongOpStateETAZeroWhenFinishedOrUnknown(t *testing.T) {
	s := longOpState{Active: true}
	if s.etaSeconds() != 0 {
		t.Fatal("ETA must be 0 when Total is unknown")
	}
	s = s.update(10, 10, time.Now())
	if s.etaSeconds() != 0 {
		t.Fatal("ETA must be 0 when Done >= Total")
	}
}
