package insights

import "time"

// calendarTrailingDays bounds how far past the last play the calendar is
// extended toward today. It keeps a skewed clock from turning the heatmap into
// a multi-year allocation while still showing an honest current dry spell.
const calendarTrailingDays = 366

// computeCalendar lays out every local day from the first play through today
// as a contiguous run, so a gap renders as a gap instead of collapsing. It
// reads the all-time timeline and ignores the active window.
func computeCalendar(timeline []TimelineRow, now time.Time, loc *time.Location) Calendar {
	if len(timeline) == 0 {
		return Calendar{}
	}

	type agg struct {
		plays int
		secs  int
	}
	byDay := make(map[time.Time]agg, len(timeline))
	var first, last time.Time
	for _, row := range timeline {
		day := localDay(row.StartAt, loc)
		a := byDay[day]
		a.plays++
		a.secs += nonNegative(row.ListenedSecs)
		byDay[day] = a

		if first.IsZero() || day.Before(first) {
			first = day
		}
		if day.After(last) {
			last = day
		}
	}

	end := last
	if today := localDay(now.Unix(), loc); today.After(end) && dayDiff(end, today) <= calendarTrailingDays {
		end = today
	}

	cal := Calendar{Days: make([]CalendarDay, 0, dayDiff(first, end)+1)}
	for day := first; !day.After(end); day = nextDay(day, loc) {
		a := byDay[day]
		cal.Days = append(cal.Days, CalendarDay{Date: day, Plays: a.plays, ListenedSecs: a.secs})
		if a.plays > cal.MaxPlays {
			cal.MaxPlays = a.plays
		}
	}
	cal.Weeks = (int(first.Weekday()) + len(cal.Days) + 6) / 7

	return cal
}

// nextDay advances one calendar day and re-normalizes to local midnight, which
// a DST transition can otherwise shift off.
func nextDay(day time.Time, loc *time.Location) time.Time {
	n := day.AddDate(0, 0, 1)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
}
