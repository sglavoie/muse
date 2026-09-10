package radio

import (
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// DiscoveryCutoffDays is the age at which a counted play stops counting as
// familiar. A track last played exactly this many days ago is discovery.
const DiscoveryCutoffDays = 90

// Clock supplies the current time. Injecting it keeps the 90-day cutoff
// testable without sleeping or freezing the process clock.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function to Clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (f ClockFunc) Now() time.Time { return f() }

// SystemClock returns a Clock backed by time.Now.
func SystemClock() Clock { return ClockFunc(time.Now) }

// FixedClock returns a Clock that always reports t.
func FixedClock(t time.Time) Clock {
	return ClockFunc(func() time.Time { return t })
}

// nowFrom reads clock, falling back to the system clock when nil so a missing
// dependency degrades to real time instead of panicking mid-playback.
func nowFrom(clock Clock) time.Time {
	if clock == nil {
		return time.Now()
	}
	return clock.Now()
}

// DiscoveryClass labels a track's familiarity.
type DiscoveryClass string

const (
	// DiscoveryNever means the track has no counted plays.
	DiscoveryNever DiscoveryClass = "never"
	// DiscoveryStale means the last counted play is at least
	// DiscoveryCutoffDays old.
	DiscoveryStale DiscoveryClass = "stale"
	// DiscoveryFamiliar means the track was counted-played recently.
	DiscoveryFamiliar DiscoveryClass = "familiar"
)

// IsDiscovery reports whether the class counts toward the discovery target.
func (c DiscoveryClass) IsDiscovery() bool {
	return c == DiscoveryNever || c == DiscoveryStale
}

// DiscoveryCutoff returns the instant at or before which a counted play is
// considered stale.
func DiscoveryCutoff(now time.Time) time.Time {
	return now.AddDate(0, 0, -DiscoveryCutoffDays)
}

// Classify labels a track from its counted-play fields. playCount and
// lastPlayedAt come from the counted play_history view — raw play events do
// not advance them, so a skipped track stays discovery.
//
// A positive play count with a missing (non-positive) timestamp is treated as
// familiar: there is positive evidence the track was played and no evidence of
// its age, and missing metadata must never quietly satisfy a target.
func Classify(playCount int, lastPlayedAt int64, clock Clock) DiscoveryClass {
	if playCount <= 0 {
		return DiscoveryNever
	}
	if lastPlayedAt <= 0 {
		return DiscoveryFamiliar
	}
	if lastPlayedAt <= DiscoveryCutoff(nowFrom(clock)).Unix() {
		return DiscoveryStale
	}
	return DiscoveryFamiliar
}

// ClassifyTrack labels a library track.
func ClassifyTrack(t library.Track, clock Clock) DiscoveryClass {
	return Classify(t.PlayCount, t.LastPlayedAt, clock)
}

// IsDiscovery reports whether t counts toward the discovery target.
func IsDiscovery(t library.Track, clock Clock) bool {
	return ClassifyTrack(t, clock).IsDiscovery()
}

// DiscoveryTargetCount returns how many of batchSize DJ selections should be
// discovery tracks at the given target, rounding halves up.
func DiscoveryTargetCount(batchSize, discoveryPercent int) int {
	if batchSize <= 0 {
		return 0
	}
	pct := ClampDiscoveryPercent(discoveryPercent)
	count := (batchSize*pct + 50) / 100
	if count > batchSize {
		return batchSize
	}
	return count
}

// CountDiscovery returns how many of the tracks classify as discovery.
func CountDiscovery(tracks []library.Track, clock Clock) int {
	count := 0
	for _, t := range tracks {
		if IsDiscovery(t, clock) {
			count++
		}
	}
	return count
}
