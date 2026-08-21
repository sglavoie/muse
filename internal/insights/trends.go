package insights

import "time"

// drySpellMinDays is the shortest silent run worth calling a dry spell. A
// single quiet day is just a Tuesday.
const drySpellMinDays = 3

// computeTrends builds the all-time series behind Momentum and Discovery. Both
// panels ignore the active window — the window control only chooses their
// bucket size — so they read the whole timeline at res.
func computeTrends(timeline []TimelineRow, cal Calendar, now time.Time, loc *time.Location, res Resolution) Trends {
	if len(timeline) == 0 {
		return Trends{Resolution: res}
	}

	s := buildSeries(timeline, now, loc, res)
	t := Trends{
		Resolution:    res,
		Buckets:       s.buckets,
		Current:       s.current,
		AvgSecs:       s.avgSecs,
		AvgFirstPlays: s.avgFirstPlays,
	}
	t.CurrentVsAvg = safeDiv(float64(t.Current.ListenedSecs), t.AvgSecs)
	t.CurrentFirstPlays = t.Current.FirstPlays
	t.RepeatShare = safeDiv(float64(t.Current.Plays-t.Current.FirstPlays), float64(t.Current.Plays))

	// Coverage projects a finish date in months, so the monthly rate is derived
	// separately whenever the panels are drawn at some other bucket size.
	t.AvgMonthlyFirstPlays = t.AvgFirstPlays
	if res != ResolutionMonthly {
		t.AvgMonthlyFirstPlays = buildSeries(timeline, now, loc, ResolutionMonthly).avgFirstPlays
	}

	t.DrySpells = computeDrySpells(cal)
	for _, spell := range t.DrySpells {
		if spell.Days > t.LongestDry.Days {
			t.LongestDry = spell
		}
	}

	return t
}

// trendSeries is one bucketing of the timeline: the contiguous run, the bucket
// the present falls in, and the baselines the run establishes.
type trendSeries struct {
	buckets       []TrendBucket
	current       TrendBucket
	avgSecs       float64
	avgFirstPlays float64
}

// buildSeries folds the timeline into contiguous buckets of size res, gaps
// included, from the first play through the bucket holding now.
func buildSeries(timeline []TimelineRow, now time.Time, loc *time.Location, res Resolution) trendSeries {
	agg := make(map[time.Time]TrendBucket, 16)
	var first, last time.Time
	for _, row := range timeline {
		start := res.BucketStart(time.Unix(row.StartAt, 0).In(loc))

		b := agg[start]
		b.Start = start
		b.Plays++
		b.ListenedSecs += nonNegative(row.ListenedSecs)
		if row.FirstEver {
			b.FirstPlays++
		}
		agg[start] = b

		if first.IsZero() || start.Before(first) {
			first = start
		}
		if start.After(last) {
			last = start
		}
	}
	if first.IsZero() {
		return trendSeries{}
	}

	current := res.BucketStart(now.In(loc))
	end := last
	if current.After(end) && res.between(end, current) <= res.trailingCap() {
		end = current
	}

	// The current bucket is still accumulating, so averaging it in would drag
	// the baseline it is being compared against.
	var s trendSeries
	var completedSecs, completedFirsts, completedCount float64
	for start := first; !start.After(end); start = res.next(start) {
		b, ok := agg[start]
		if !ok {
			b = TrendBucket{Start: start}
		}
		s.buckets = append(s.buckets, b)
		if start.Equal(current) {
			s.current = b
			continue
		}
		completedSecs += float64(b.ListenedSecs)
		completedFirsts += float64(b.FirstPlays)
		completedCount++
	}
	s.avgSecs = safeDiv(completedSecs, completedCount)
	s.avgFirstPlays = safeDiv(completedFirsts, completedCount)
	return s
}

// computeDrySpells finds the silent runs inside the calendar. The run is
// bounded by the calendar itself, so a spell never starts before the first
// play or extends past today.
func computeDrySpells(cal Calendar) []DrySpell {
	var spells []DrySpell
	var run []CalendarDay
	flush := func() {
		if len(run) >= drySpellMinDays {
			spells = append(spells, DrySpell{
				Start: run[0].Date,
				End:   run[len(run)-1].Date,
				Days:  len(run),
			})
		}
		run = nil
	}
	for _, day := range cal.Days {
		if day.Plays == 0 {
			run = append(run, day)
			continue
		}
		flush()
	}
	flush()
	return spells
}
