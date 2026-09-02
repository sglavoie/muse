package statusbar

import "time"

// longOpRateWindow is the duration of the rolling sample window used to
// compute the items-per-second rate for ETA. Two seconds is short enough to
// react to throughput changes (e.g. ExifTool warming up) without being so
// short that a single slow batch dominates the rate.
const longOpRateWindow = 2 * time.Second

// longOpSample is one point in the rate window: how many items had been
// processed at the given time. The rate is computed as the slope between the
// oldest and newest samples in the window.
type longOpSample struct {
	at   time.Time
	done int
}

// LongOpState describes the currently active long-running operation. It is
// generic over the kind of operation (library scan today, audit/import in
// future) — the renderer reads Label, Done, Total, and the rate ring buffer.
// Active is false (zero value) when no long op is running, which is the
// signal the status bar uses to skip the line-2 progress render.
type LongOpState struct {
	Active    bool
	Label     string
	Done      int
	Total     int
	StartedAt time.Time
	samples   []longOpSample
}

// Update records the latest progress and pushes a sample onto the rate-window
// ring buffer, evicting samples older than longOpRateWindow.
//
// The new sample slice is always allocated independently of the receiver's
// backing array. This prevents aliasing bugs when an earlier snapshot of
// longOpState is still alive elsewhere (e.g. a prior Model value retained by
// a deferred tea.Cmd): mutating the shared backing array via append/copy
// would otherwise corrupt the snapshot's view of its samples.
func (s LongOpState) Update(done, total int, now time.Time) LongOpState {
	s.Done = done
	s.Total = total
	cutoff := now.Add(-longOpRateWindow)
	// Find the first prior sample within the window. Anything before is evicted.
	i := 0
	for ; i < len(s.samples); i++ {
		if !s.samples[i].at.Before(cutoff) {
			break
		}
	}
	kept := s.samples[i:]
	// Allocate a fresh slice sized exactly for the retained samples + the new
	// one. Copying into a brand-new backing array guarantees the receiver's
	// (and any prior snapshot's) samples are not mutated.
	fresh := make([]longOpSample, len(kept), len(kept)+1)
	copy(fresh, kept)
	fresh = append(fresh, longOpSample{at: now, done: done})
	s.samples = fresh
	return s
}

// rate returns the items-per-second throughput estimated from the rate-window
// samples. Returns 0 when fewer than two samples are available or the window
// span is zero.
func (s LongOpState) rate() float64 {
	if len(s.samples) < 2 {
		return 0
	}
	oldest := s.samples[0]
	newest := s.samples[len(s.samples)-1]
	dt := newest.at.Sub(oldest.at).Seconds()
	if dt <= 0 {
		return 0
	}
	delta := newest.done - oldest.done
	if delta <= 0 {
		return 0
	}
	return float64(delta) / dt
}

// etaSeconds returns the estimated seconds remaining based on the current
// rate. Returns 0 when the rate is unknown or the operation has finished.
func (s LongOpState) etaSeconds() int {
	if s.Total <= 0 || s.Done >= s.Total {
		return 0
	}
	r := s.rate()
	if r <= 0 {
		return 0
	}
	return int(float64(s.Total-s.Done) / r)
}
