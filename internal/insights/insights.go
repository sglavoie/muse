package insights

import (
	"math"
	"time"
)

// Compute turns one loaded Input into the full panel state. It is pure and
// total: no clock read, no timezone read, no panic on degenerate data, and
// Compute(Input{}) is the zero Snapshot.
func Compute(in Input) Snapshot {
	loc := location(in.Loc)

	snap := Snapshot{
		WindowLabel: in.WindowLabel,
		Since:       in.Since,
	}
	for _, p := range in.Plays {
		snap.TotalPlays++
		snap.TotalSecs += nonNegative(p.ListenedSecs)
		if snap.FirstPlayAt == 0 || p.StartAt < snap.FirstPlayAt {
			snap.FirstPlayAt = p.StartAt
		}
		if p.StartAt > snap.LastPlayAt {
			snap.LastPlayAt = p.StartAt
		}
	}
	if snap.TotalPlays > 0 {
		first := localDay(snap.FirstPlayAt, loc)
		last := localDay(snap.LastPlayAt, loc)
		snap.SpanDays = dayDiff(first, last) + 1
	}

	groups := foldGenres(in.Genres)

	snap.Rhythm = computeRhythm(in.Plays, groups, loc)
	snap.Calendar = computeCalendar(in.Timeline, in.Now, loc)
	snap.Trends = computeTrends(in.Timeline, snap.Calendar, in.Now, loc, in.Resolution)
	snap.Rankings = computeRankings(in.Plays)
	snap.SessionStats = computeSessions(in.Sessions)
	snap.Quality = computeQuality(groups, in.Quality)
	snap.Coverage = computeCoverage(in.Coverage, in.Timeline, snap.Trends)

	return snap
}

// location resolves the injected timezone. Compute must never fall back to
// time.Local: the result would depend on the host and stop being reproducible
// in tests.
func location(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}

func nonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func nonNegativeFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

// localDay returns local midnight of the day containing unix.
func localDay(unix int64, loc *time.Location) time.Time {
	t := time.Unix(unix, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// dayDiff counts calendar days from a to b. Both must be local midnights.
// Rounding absorbs the 23- and 25-hour days a DST transition produces, which
// plain division by 24h would truncate to the wrong day.
func dayDiff(a, b time.Time) int {
	return int(math.Round(b.Sub(a).Hours() / 24))
}

// safeDiv returns a/b, or 0 when b is zero or the result is not finite. Every
// ratio in this package goes through it: the panels feed these numbers
// straight into bar widths, and a NaN reaching strings.Repeat panics.
func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	r := a / b
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return 0
	}
	return r
}
