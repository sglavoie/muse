package insights

import (
	"testing"
	"time"
)

func TestCalendarIsContiguousAcrossDST(t *testing.T) {
	loc := testLoc(t)
	// 2026-03-08 is 23 hours long in this zone; naive 24-hour arithmetic
	// drops or duplicates a day here.
	timeline := []TimelineRow{
		{StartAt: at(t, loc, "2026-03-07 20:00:00"), ListenedSecs: 600},
		{StartAt: at(t, loc, "2026-03-08 15:00:00"), ListenedSecs: 600},
		{StartAt: at(t, loc, "2026-03-08 16:00:00"), ListenedSecs: 600},
		{StartAt: at(t, loc, "2026-03-10 09:00:00"), ListenedSecs: 600},
	}
	now := time.Unix(at(t, loc, "2026-03-10 12:00:00"), 0)

	cal := computeCalendar(timeline, now, loc)

	wantDays := []struct {
		date  string
		plays int
	}{
		{"2026-03-07", 1},
		{"2026-03-08", 2},
		{"2026-03-09", 0},
		{"2026-03-10", 1},
	}
	if len(cal.Days) != len(wantDays) {
		t.Fatalf("Days = %d, want %d", len(cal.Days), len(wantDays))
	}
	for i, want := range wantDays {
		got := cal.Days[i]
		if got.Date.Format("2006-01-02") != want.date || got.Plays != want.plays {
			t.Errorf("day %d = %s/%d plays, want %s/%d",
				i, got.Date.Format("2006-01-02"), got.Plays, want.date, want.plays)
		}
		if h, m, s := got.Date.Clock(); h != 0 || m != 0 || s != 0 {
			t.Errorf("day %d is %v, want local midnight", i, got.Date)
		}
	}
	if cal.MaxPlays != 2 {
		t.Errorf("MaxPlays = %d, want 2", cal.MaxPlays)
	}
	// 2026-03-07 is a Saturday, so the run spans two Sunday-aligned columns.
	if cal.Weeks != 2 {
		t.Errorf("Weeks = %d, want 2", cal.Weeks)
	}
}

func TestCalendarExtendsToTodayThroughADrySpell(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{{StartAt: at(t, loc, "2026-08-01 10:00:00"), ListenedSecs: 600}}
	now := time.Unix(at(t, loc, "2026-08-05 10:00:00"), 0)

	cal := computeCalendar(timeline, now, loc)

	if len(cal.Days) != 5 {
		t.Fatalf("Days = %d, want 5 (Aug 1 through today)", len(cal.Days))
	}
	if last := cal.Days[len(cal.Days)-1]; last.Date.Format("2006-01-02") != "2026-08-05" {
		t.Errorf("last day = %s, want 2026-08-05", last.Date.Format("2006-01-02"))
	}
}

func TestCalendarIgnoresAWildlySkewedClock(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{{StartAt: at(t, loc, "2026-08-01 10:00:00"), ListenedSecs: 600}}
	now := time.Unix(at(t, loc, "2099-08-01 10:00:00"), 0)

	cal := computeCalendar(timeline, now, loc)

	if len(cal.Days) != 1 {
		t.Errorf("Days = %d, want 1; a clock decades ahead must not extend the calendar", len(cal.Days))
	}
}

func TestCalendarEmptyIsZero(t *testing.T) {
	cal := computeCalendar(nil, time.Unix(0, 0), testLoc(t))
	if cal.Days != nil || cal.Weeks != 0 || cal.MaxPlays != 0 {
		t.Errorf("computeCalendar(nil) = %+v, want zero Calendar", cal)
	}
}
