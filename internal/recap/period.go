// Package recap builds the yearly listening recap ("Your year in muse") from
// the same start-corrected, counted-only play history the Insights view reads.
// Like internal/insights it is pure: no database, no TUI and no clock. The
// display timezone and every period boundary are injected, so a recap is
// reproducible in tests and the same on every host.
//
// The play-counting rules are inherited, not re-implemented: rows come from
// the play_history view, which only holds listens long enough to count as a
// play, so a skip never reaches this package. A counted play that stopped
// short contributes the seconds actually heard, never the track's length.
package recap

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/sglavoie/muse/internal/insights"
)

// Period is the span one recap covers: a calendar year, or the rolling twelve
// months up to today. Start is inclusive and End exclusive; both are local
// midnights in the location the period was built in.
type Period struct {
	Year  int // 0 for the rolling period
	Start time.Time
	End   time.Time
}

// YearPeriod is the calendar year in loc. A nil loc is UTC.
func YearPeriod(year int, loc *time.Location) Period {
	loc = location(loc)
	return Period{
		Year:  year,
		Start: time.Date(year, time.January, 1, 0, 0, 0, 0, loc),
		End:   time.Date(year+1, time.January, 1, 0, 0, 0, 0, loc),
	}
}

// RollingPeriod is the twelve months that end with the day now falls on, so
// today's plays are in it and the same day a year ago is not.
func RollingPeriod(now time.Time, loc *time.Location) Period {
	loc = location(loc)
	t := now.In(loc)
	end := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
	start := time.Date(end.Year()-1, end.Month(), end.Day(), 0, 0, 0, 0, loc)
	return Period{Start: start, End: end}
}

// Rolling reports whether p is the rolling twelve months rather than a year.
func (p Period) Rolling() bool { return p.Year == 0 }

// Label names the period the way the recap's header does.
func (p Period) Label() string {
	if p.Rolling() {
		return "Last 12 months"
	}
	return strconv.Itoa(p.Year)
}

// Previous is the period of the same length just before p: the year before,
// or the twelve months before the rolling twelve.
func (p Period) Previous() Period {
	if !p.Rolling() {
		return YearPeriod(p.Year-1, p.Start.Location())
	}
	loc := p.Start.Location()
	start := time.Date(p.Start.Year()-1, p.Start.Month(), p.Start.Day(), 0, 0, 0, 0, loc)
	return Period{Start: start, End: p.Start}
}

// Contains reports whether a play that started at unix falls inside p.
func (p Period) Contains(unix int64) bool {
	return unix >= p.Start.Unix() && unix < p.End.Unix()
}

// Days is how many calendar days p spans.
func (p Period) Days() int { return dayDiff(p.Start, p.End) }

// YearsWithPlays lists, oldest first, every local calendar year at least one
// play started in.
func YearsWithPlays(plays []insights.PlayRow, loc *time.Location) []int {
	loc = location(loc)
	seen := map[int]struct{}{}
	for _, p := range plays {
		seen[time.Unix(p.StartAt, 0).In(loc).Year()] = struct{}{}
	}
	years := make([]int, 0, len(seen))
	for y := range seen {
		years = append(years, y)
	}
	sort.Ints(years)
	return years
}

// location mirrors internal/insights: never fall back to time.Local, which
// would make the result depend on the host.
func location(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}

// localDay returns local midnight of the day containing unix.
func localDay(unix int64, loc *time.Location) time.Time {
	t := time.Unix(unix, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// dayDiff counts calendar days from a to b, both local midnights. Rounding
// absorbs the 23- and 25-hour days around a DST change.
func dayDiff(a, b time.Time) int {
	return int(math.Round(b.Sub(a).Hours() / 24))
}

func nextDay(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location())
}
