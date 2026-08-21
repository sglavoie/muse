package insights

import (
	"testing"
	"time"
)

// TestTrendsMonthBoundaryUsesPlayStart pins the month-boundary case the hour
// bug creates: a play that ended twenty seconds into May began in April and
// belongs to April.
func TestTrendsMonthBoundaryUsesPlayStart(t *testing.T) {
	loc := testLoc(t)
	endedAt := at(t, loc, "2026-05-01 00:00:20")
	timeline := []TimelineRow{
		{StartAt: endedAt - 40, ListenedSecs: 40},
		{StartAt: at(t, loc, "2026-05-15 12:00:00"), ListenedSecs: 600},
	}
	now := time.Unix(at(t, loc, "2026-05-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc)

	if len(tr.Months) != 2 {
		t.Fatalf("Months = %d, want 2 (April and May)", len(tr.Months))
	}
	april, may := tr.Months[0], tr.Months[1]
	if april.Month != time.April || april.Year != 2026 || april.Plays != 1 {
		t.Errorf("first month = %d-%v/%d plays, want 2026-April/1", april.Year, april.Month, april.Plays)
	}
	if may.Month != time.May || may.Plays != 1 {
		t.Errorf("second month = %v/%d plays, want May/1", may.Month, may.Plays)
	}
}

func TestTrendsMonthsAreContiguousAcrossAYearBoundary(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{
		{StartAt: at(t, loc, "2025-11-10 12:00:00"), ListenedSecs: 600},
		{StartAt: at(t, loc, "2026-01-10 12:00:00"), ListenedSecs: 600},
	}
	now := time.Unix(at(t, loc, "2026-01-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc)

	want := []struct {
		year  int
		month time.Month
		plays int
	}{
		{2025, time.November, 1},
		{2025, time.December, 0},
		{2026, time.January, 1},
	}
	if len(tr.Months) != len(want) {
		t.Fatalf("Months = %d, want %d", len(tr.Months), len(want))
	}
	for i, w := range want {
		got := tr.Months[i]
		if got.Year != w.year || got.Month != w.month || got.Plays != w.plays {
			t.Errorf("month %d = %d-%v/%d, want %d-%v/%d",
				i, got.Year, got.Month, got.Plays, w.year, w.month, w.plays)
		}
	}
}

func TestTrendsAveragesExcludeTheCurrentMonth(t *testing.T) {
	loc := testLoc(t)
	var timeline []TimelineRow
	add := func(wall string, secs int, first bool) {
		timeline = append(timeline, TimelineRow{
			StartAt: at(t, loc, wall), ListenedSecs: secs, FirstEver: first,
		})
	}
	add("2026-04-02 12:00:00", 3600, true)
	add("2026-04-20 12:00:00", 3600, true)
	add("2026-05-04 12:00:00", 1800, true)
	add("2026-07-04 12:00:00", 600, true)
	for i := 0; i < 5; i++ {
		add("2026-08-04 12:00:00", 300, i < 2)
	}
	now := time.Unix(at(t, loc, "2026-08-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc)

	if len(tr.Months) != 5 {
		t.Fatalf("Months = %d, want 5 (April through August)", len(tr.Months))
	}
	if tr.Current.Month != time.August || tr.Current.Plays != 5 {
		t.Fatalf("Current = %v/%d plays, want August/5", tr.Current.Month, tr.Current.Plays)
	}
	if tr.AvgMonthlySecs != 2400 {
		t.Errorf("AvgMonthlySecs = %v, want 2400 ((7200+1800+0+600)/4)", tr.AvgMonthlySecs)
	}
	if tr.AvgMonthlyFirstPlays != 1 {
		t.Errorf("AvgMonthlyFirstPlays = %v, want 1 (4 first plays over 4 completed months)", tr.AvgMonthlyFirstPlays)
	}
	if tr.CurrentVsAvg != 0.625 {
		t.Errorf("CurrentVsAvg = %v, want 0.625", tr.CurrentVsAvg)
	}
	if tr.FirstPlaysMonth != 2 {
		t.Errorf("FirstPlaysMonth = %d, want 2", tr.FirstPlaysMonth)
	}
	if tr.RepeatShare != 0.6 {
		t.Errorf("RepeatShare = %v, want 0.6", tr.RepeatShare)
	}
}

func TestTrendsSingleMonthHasNoBaseline(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{{StartAt: at(t, loc, "2026-08-04 12:00:00"), ListenedSecs: 600}}
	now := time.Unix(at(t, loc, "2026-08-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc)

	if tr.AvgMonthlySecs != 0 || tr.CurrentVsAvg != 0 {
		t.Errorf("avg = %v, ratio = %v; both want 0 with no completed month",
			tr.AvgMonthlySecs, tr.CurrentVsAvg)
	}
}

func TestTrendsDrySpells(t *testing.T) {
	loc := testLoc(t)
	day := func(wall string, plays int) CalendarDay {
		return CalendarDay{Date: time.Unix(at(t, loc, wall), 0).In(loc), Plays: plays}
	}
	cal := Calendar{Days: []CalendarDay{
		day("2026-08-01 00:00:00", 3),
		day("2026-08-02 00:00:00", 0),
		day("2026-08-03 00:00:00", 0),
		day("2026-08-04 00:00:00", 1),
		day("2026-08-05 00:00:00", 0),
		day("2026-08-06 00:00:00", 0),
		day("2026-08-07 00:00:00", 0),
		day("2026-08-08 00:00:00", 0),
		day("2026-08-09 00:00:00", 2),
		day("2026-08-10 00:00:00", 0),
		day("2026-08-11 00:00:00", 0),
		day("2026-08-12 00:00:00", 0),
	}}
	timeline := []TimelineRow{{StartAt: at(t, loc, "2026-08-01 12:00:00"), ListenedSecs: 600}}

	tr := computeTrends(timeline, cal, time.Unix(at(t, loc, "2026-08-12 12:00:00"), 0), loc)

	// The two-day gap is below the threshold; the four-day and the trailing
	// three-day runs are not.
	if len(tr.DrySpells) != 2 {
		t.Fatalf("DrySpells = %d, want 2: %+v", len(tr.DrySpells), tr.DrySpells)
	}
	if tr.LongestDry.Days != 4 {
		t.Errorf("LongestDry = %d days, want 4", tr.LongestDry.Days)
	}
	if got := tr.LongestDry.Start.Format("2006-01-02"); got != "2026-08-05" {
		t.Errorf("LongestDry starts %s, want 2026-08-05", got)
	}
	if got := tr.DrySpells[1].Days; got != 3 {
		t.Errorf("trailing spell = %d days, want 3", got)
	}
}

func TestTrendsEmptyIsZero(t *testing.T) {
	tr := computeTrends(nil, Calendar{}, time.Unix(0, 0), testLoc(t))
	if tr.Months != nil || tr.DrySpells != nil {
		t.Errorf("computeTrends(nil) = %+v, want zero Trends", tr)
	}
}
