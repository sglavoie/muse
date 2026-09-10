package radio

import "sort"

// ShortageKind names a quota the eligible candidates could not fill.
type ShortageKind string

const (
	// ShortageDiscovery means fewer discovery tracks were available than the
	// target asked for. The remaining slots were filled with familiar tracks.
	ShortageDiscovery ShortageKind = "discovery"
	// ShortageFamiliar means fewer familiar tracks were available than the
	// non-discovery remainder of the batch.
	ShortageFamiliar ShortageKind = "familiar"
	// ShortageBatch means the batch itself came back under its requested
	// size, with both pools exhausted.
	ShortageBatch ShortageKind = "batch"
)

// Shortage records one unfilled quota. It is a factual statement about a single
// batch and is never carried forward: see Allocation for why a shortage creates
// no future debt.
type Shortage struct {
	Kind ShortageKind
	Want int
	Got  int
}

// Missing returns how many selections the quota came up short.
func (s Shortage) Missing() int {
	if s.Got >= s.Want {
		return 0
	}
	return s.Want - s.Got
}

// Shortages is an ordered list of unfilled quotas.
type Shortages []Shortage

// Has reports whether any shortage is of the given kind.
func (ss Shortages) Has(kind ShortageKind) bool {
	_, ok := ss.Get(kind)
	return ok
}

// Get returns the shortage of the given kind.
func (ss Shortages) Get(kind ShortageKind) (Shortage, bool) {
	for _, s := range ss {
		if s.Kind == kind {
			return s, true
		}
	}
	return Shortage{}, false
}

// Rejection counts how many candidates one reason removed from consideration.
// It is what lets the Preview tab say "412 tracks excluded by genre" instead of
// only reporting an empty result.
type Rejection struct {
	Code  ReasonCode
	Value string
	Count int
}

// Reason renders the rejection back as a structured Reason.
func (r Rejection) Reason() Reason {
	return Reason{Code: r.Code, Value: r.Value}
}

// Rejections is a tally of removed candidates, ordered by descending count and
// then by code and value so the same library always renders the same list.
type Rejections []Rejection

// Total returns how many candidates were removed in all.
func (rs Rejections) Total() int {
	total := 0
	for _, r := range rs {
		total += r.Count
	}
	return total
}

// Count returns how many candidates the given code removed, across values.
func (rs Rejections) Count(code ReasonCode) int {
	total := 0
	for _, r := range rs {
		if r.Code == code {
			total += r.Count
		}
	}
	return total
}

// Top returns the most frequent rejection, or false when nothing was rejected.
func (rs Rejections) Top() (Rejection, bool) {
	if len(rs) == 0 {
		return Rejection{}, false
	}
	return rs[0], true
}

// rejectionTally accumulates rejections keyed by code and value.
type rejectionTally struct {
	counts map[Rejection]int
}

func newRejectionTally() *rejectionTally {
	return &rejectionTally{counts: make(map[Rejection]int)}
}

// add records one rejected candidate. The reason's free-form Detail is dropped
// so tallies group by cause rather than by prose.
func (t *rejectionTally) add(reason Reason) {
	if reason.IsZero() {
		return
	}
	t.counts[Rejection{Code: reason.Code, Value: reason.Value}]++
}

// result returns the stably ordered tally.
func (t *rejectionTally) result() Rejections {
	if len(t.counts) == 0 {
		return nil
	}
	out := make(Rejections, 0, len(t.counts))
	for key, count := range t.counts {
		key.Count = count
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Value < out[j].Value
	})
	return out
}
