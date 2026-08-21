package insights

import "time"

// drySpellMinDays is the shortest silent run worth calling a dry spell. A
// single quiet day is just a Tuesday.
const drySpellMinDays = 3

// trendsTrailingMonths bounds how far past the last play the monthly series is
// extended toward the current month, so a skewed clock cannot generate an
// unbounded run of empty bars.
const trendsTrailingMonths = 24

type monthKey struct {
	year  int
	month time.Month
}

// computeTrends builds the all-time monthly series behind Momentum and
// Discovery. Both panels ignore the active window — the window only chooses
// their resolution — so they read the all-time timeline.
func computeTrends(timeline []TimelineRow, cal Calendar, now time.Time, loc *time.Location) Trends {
	if len(timeline) == 0 {
		return Trends{}
	}

	buckets := make(map[monthKey]MonthBucket, 12)
	var first, last monthKey
	seenAny := false
	for _, row := range timeline {
		start := time.Unix(row.StartAt, 0).In(loc)
		key := monthKey{start.Year(), start.Month()}

		b := buckets[key]
		b.Year, b.Month = key.year, key.month
		b.Plays++
		b.ListenedSecs += nonNegative(row.ListenedSecs)
		if row.FirstEver {
			b.FirstPlays++
		}
		buckets[key] = b

		if !seenAny || monthBefore(key, first) {
			first = key
		}
		if !seenAny || monthBefore(last, key) {
			last = key
		}
		seenAny = true
	}

	nowLocal := now.In(loc)
	current := monthKey{nowLocal.Year(), nowLocal.Month()}
	end := last
	if monthBefore(end, current) && monthsBetween(end, current) <= trendsTrailingMonths {
		end = current
	}

	var t Trends
	for key := first; !monthBefore(end, key); key = nextMonth(key) {
		b, ok := buckets[key]
		if !ok {
			b = MonthBucket{Year: key.year, Month: key.month}
		}
		t.Months = append(t.Months, b)
	}

	// The current month is still accumulating, so averaging it in would drag
	// the baseline it is being compared against.
	var completedSecs, completedFirsts, completedCount float64
	for _, b := range t.Months {
		if (monthKey{b.Year, b.Month}) == current {
			t.Current = b
			continue
		}
		completedSecs += float64(b.ListenedSecs)
		completedFirsts += float64(b.FirstPlays)
		completedCount++
	}
	t.AvgMonthlySecs = safeDiv(completedSecs, completedCount)
	t.AvgMonthlyFirstPlays = safeDiv(completedFirsts, completedCount)
	t.CurrentVsAvg = safeDiv(float64(t.Current.ListenedSecs), t.AvgMonthlySecs)
	t.FirstPlaysMonth = t.Current.FirstPlays
	t.RepeatShare = safeDiv(float64(t.Current.Plays-t.Current.FirstPlays), float64(t.Current.Plays))

	t.DrySpells = computeDrySpells(cal)
	for _, spell := range t.DrySpells {
		if spell.Days > t.LongestDry.Days {
			t.LongestDry = spell
		}
	}

	return t
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

func monthBefore(a, b monthKey) bool {
	if a.year != b.year {
		return a.year < b.year
	}
	return a.month < b.month
}

func monthsBetween(a, b monthKey) int {
	return (b.year-a.year)*12 + int(b.month) - int(a.month)
}

func nextMonth(k monthKey) monthKey {
	if k.month == time.December {
		return monthKey{k.year + 1, time.January}
	}
	return monthKey{k.year, k.month + 1}
}
