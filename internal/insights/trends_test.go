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

	tr := computeTrends(timeline, Calendar{}, now, loc, ResolutionMonthly)

	if len(tr.Buckets) != 2 {
		t.Fatalf("Buckets = %d, want 2 (April and May)", len(tr.Buckets))
	}
	april, may := tr.Buckets[0], tr.Buckets[1]
	if april.Start.Month() != time.April || april.Start.Year() != 2026 || april.Plays != 1 {
		t.Errorf("first bucket = %s/%d plays, want 2026-April/1",
			april.Start.Format("2006-01"), april.Plays)
	}
	if may.Start.Month() != time.May || may.Plays != 1 {
		t.Errorf("second bucket = %v/%d plays, want May/1", may.Start.Month(), may.Plays)
	}
}

func TestTrendsMonthsAreContiguousAcrossAYearBoundary(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{
		{StartAt: at(t, loc, "2025-11-10 12:00:00"), ListenedSecs: 600},
		{StartAt: at(t, loc, "2026-01-10 12:00:00"), ListenedSecs: 600},
	}
	now := time.Unix(at(t, loc, "2026-01-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc, ResolutionMonthly)

	want := []struct {
		month string
		plays int
	}{
		{"2025-11", 1},
		{"2025-12", 0},
		{"2026-01", 1},
	}
	if len(tr.Buckets) != len(want) {
		t.Fatalf("Buckets = %d, want %d", len(tr.Buckets), len(want))
	}
	for i, w := range want {
		got := tr.Buckets[i]
		if got.Start.Format("2006-01") != w.month || got.Plays != w.plays {
			t.Errorf("bucket %d = %s/%d, want %s/%d",
				i, got.Start.Format("2006-01"), got.Plays, w.month, w.plays)
		}
	}
}

func TestTrendsAveragesExcludeTheCurrentMonth(t *testing.T) {
	loc := testLoc(t)
	timeline := trendFixture(t, loc)
	now := time.Unix(at(t, loc, "2026-08-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc, ResolutionMonthly)

	if len(tr.Buckets) != 5 {
		t.Fatalf("Buckets = %d, want 5 (April through August)", len(tr.Buckets))
	}
	if tr.Current.Start.Month() != time.August || tr.Current.Plays != 5 {
		t.Fatalf("Current = %v/%d plays, want August/5", tr.Current.Start.Month(), tr.Current.Plays)
	}
	if tr.AvgSecs != 2400 {
		t.Errorf("AvgSecs = %v, want 2400 ((7200+1800+0+600)/4)", tr.AvgSecs)
	}
	if tr.AvgMonthlyFirstPlays != 1 {
		t.Errorf("AvgMonthlyFirstPlays = %v, want 1 (4 first plays over 4 completed months)", tr.AvgMonthlyFirstPlays)
	}
	if tr.CurrentVsAvg != 0.625 {
		t.Errorf("CurrentVsAvg = %v, want 0.625", tr.CurrentVsAvg)
	}
	if tr.CurrentFirstPlays != 2 {
		t.Errorf("CurrentFirstPlays = %d, want 2", tr.CurrentFirstPlays)
	}
	if tr.RepeatShare != 0.6 {
		t.Errorf("RepeatShare = %v, want 0.6", tr.RepeatShare)
	}
}

func TestTrendsSingleMonthHasNoBaseline(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{{StartAt: at(t, loc, "2026-08-04 12:00:00"), ListenedSecs: 600}}
	now := time.Unix(at(t, loc, "2026-08-20 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc, ResolutionMonthly)

	if tr.AvgSecs != 0 || tr.CurrentVsAvg != 0 {
		t.Errorf("avg = %v, ratio = %v; both want 0 with no completed month",
			tr.AvgSecs, tr.CurrentVsAvg)
	}
}

// TestTrendsResolutionRebucketsTheSameHistory is the core of the resolution
// control: the source span is identical at every bucket size, so the totals must
// match while the boundaries and the count of buckets do not.
func TestTrendsResolutionRebucketsTheSameHistory(t *testing.T) {
	loc := testLoc(t)
	timeline := trendFixture(t, loc)
	now := time.Unix(at(t, loc, "2026-08-20 12:00:00"), 0)

	monthly := computeTrends(timeline, Calendar{}, now, loc, ResolutionMonthly)
	weekly := computeTrends(timeline, Calendar{}, now, loc, ResolutionWeekly)
	daily := computeTrends(timeline, Calendar{}, now, loc, ResolutionDaily)

	total := func(tr Trends) (plays, secs int) {
		for _, b := range tr.Buckets {
			plays += b.Plays
			secs += b.ListenedSecs
		}
		return
	}
	wantPlays, wantSecs := total(monthly)
	for name, tr := range map[string]Trends{"weekly": weekly, "daily": daily} {
		plays, secs := total(tr)
		if plays != wantPlays || secs != wantSecs {
			t.Errorf("%s totals = %d plays/%ds, want %d/%ds — the span must not change",
				name, plays, secs, wantPlays, wantSecs)
		}
	}

	if !(len(monthly.Buckets) < len(weekly.Buckets) && len(weekly.Buckets) < len(daily.Buckets)) {
		t.Errorf("bucket counts = %d monthly, %d weekly, %d daily; want strictly increasing",
			len(monthly.Buckets), len(weekly.Buckets), len(daily.Buckets))
	}

	// The current bucket narrows with the resolution: five plays landed in
	// August, all of them on 4 August, so the current day holds none of them.
	if monthly.Current.Plays != 5 {
		t.Errorf("monthly Current.Plays = %d, want 5", monthly.Current.Plays)
	}
	if daily.Current.Plays != 0 {
		t.Errorf("daily Current.Plays = %d, want 0 (nothing played on 20 August)", daily.Current.Plays)
	}
	if got := daily.Current.Start.Format("2006-01-02"); got != "2026-08-20" {
		t.Errorf("daily Current starts %s, want 2026-08-20", got)
	}
	if got := weekly.Current.Start.Format("2006-01-02"); got != "2026-08-17" {
		t.Errorf("weekly Current starts %s, want 2026-08-17 (the Monday)", got)
	}

	// Coverage projects a finish date in months and must keep reading a monthly
	// rate whatever the panels are drawn at.
	for name, tr := range map[string]Trends{"weekly": weekly, "daily": daily} {
		if tr.AvgMonthlyFirstPlays != monthly.AvgMonthlyFirstPlays {
			t.Errorf("%s AvgMonthlyFirstPlays = %v, want %v — Coverage must not be rescaled",
				name, tr.AvgMonthlyFirstPlays, monthly.AvgMonthlyFirstPlays)
		}
	}
}

// TestResolutionBucketStartSurvivesDST pins the boundary arithmetic against the
// 23- and 25-hour days: a bucket start reached by adding a fixed duration would
// slide off local midnight, and every label and drill key is formatted from it.
func TestResolutionBucketStartSurvivesDST(t *testing.T) {
	loc := testLoc(t)
	// America/Toronto springs forward on 8 March 2026 and falls back on 1
	// November 2026.
	for _, tc := range []struct {
		name    string
		res     Resolution
		from    string
		wantDay string
	}{
		{"daily across spring forward", ResolutionDaily, "2026-03-07 12:00:00", "2026-03-08"},
		{"daily across fall back", ResolutionDaily, "2026-10-31 12:00:00", "2026-11-01"},
		{"weekly across spring forward", ResolutionWeekly, "2026-03-02 12:00:00", "2026-03-09"},
		{"weekly across fall back", ResolutionWeekly, "2026-10-26 12:00:00", "2026-11-02"},
		{"monthly across spring forward", ResolutionMonthly, "2026-03-15 12:00:00", "2026-04-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := tc.res.BucketStart(time.Unix(at(t, loc, tc.from), 0).In(loc))
			next := tc.res.next(start)
			if got := next.Format("2006-01-02 15:04:05"); got != tc.wantDay+" 00:00:00" {
				t.Errorf("next bucket = %s, want %s 00:00:00", got, tc.wantDay)
			}
			if tc.res.between(start, next) != 1 {
				t.Errorf("between(start, next) = %d, want 1", tc.res.between(start, next))
			}
		})
	}
}

// TestTrendsWeeklyBucketsStartOnMonday pins the week boundary, which the drill
// key and the ISO week label both depend on.
func TestTrendsWeeklyBucketsStartOnMonday(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{
		{StartAt: at(t, loc, "2026-08-16 23:00:00"), ListenedSecs: 600}, // Sunday
		{StartAt: at(t, loc, "2026-08-17 01:00:00"), ListenedSecs: 600}, // Monday
	}
	now := time.Unix(at(t, loc, "2026-08-18 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc, ResolutionWeekly)

	if len(tr.Buckets) != 2 {
		t.Fatalf("Buckets = %d, want 2 — Sunday must not share a week with the Monday after it", len(tr.Buckets))
	}
	for i, want := range []string{"2026-08-10", "2026-08-17"} {
		if got := tr.Buckets[i].Start.Format("2006-01-02"); got != want {
			t.Errorf("bucket %d starts %s, want %s", i, got, want)
		}
		if wd := tr.Buckets[i].Start.Weekday(); wd != time.Monday {
			t.Errorf("bucket %d starts on %v, want Monday", i, wd)
		}
	}
}

// TestTrendsTrailingCapBoundsAnEmptyRun keeps a skewed clock from generating an
// unbounded run of empty daily buckets between the last play and the present.
func TestTrendsTrailingCapBoundsAnEmptyRun(t *testing.T) {
	loc := testLoc(t)
	timeline := []TimelineRow{{StartAt: at(t, loc, "2026-08-04 12:00:00"), ListenedSecs: 600}}
	now := time.Unix(at(t, loc, "2036-08-04 12:00:00"), 0)

	tr := computeTrends(timeline, Calendar{}, now, loc, ResolutionDaily)

	if len(tr.Buckets) != 1 {
		t.Errorf("Buckets = %d, want 1 — a decade of empty days must not be materialised", len(tr.Buckets))
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

	tr := computeTrends(timeline, cal, time.Unix(at(t, loc, "2026-08-12 12:00:00"), 0), loc, ResolutionMonthly)

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
	tr := computeTrends(nil, Calendar{}, time.Unix(0, 0), testLoc(t), ResolutionMonthly)
	if tr.Buckets != nil || tr.DrySpells != nil {
		t.Errorf("computeTrends(nil) = %+v, want zero Trends", tr)
	}
}

// trendFixture is five months of history ending in the current one: April
// through August 2026, with the last five plays all on 4 August.
func trendFixture(t *testing.T, loc *time.Location) []TimelineRow {
	t.Helper()
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
	return timeline
}
