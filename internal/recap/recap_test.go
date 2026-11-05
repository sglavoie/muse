package recap

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	// tzdata is embedded so LoadLocation works on hosts without a zoneinfo
	// database; the time-zone cases are meaningless without it.
	_ "time/tzdata"

	"github.com/sglavoie/muse/internal/insights"
)

func loadLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

// at parses a local wall-clock start time in loc.
func at(t *testing.T, loc *time.Location, wall string) int64 {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04", wall, loc)
	if err != nil {
		t.Fatalf("ParseInLocation(%q): %v", wall, err)
	}
	return ts.Unix()
}

// row is a counted play of track hash that started at start and ran secs.
func row(hash string, start int64, secs int) insights.PlayRow {
	return insights.PlayRow{
		Hash: hash, StartAt: start, ListenedSecs: secs,
		Title: "T-" + hash, Artist: "A-" + hash[:1], Album: "L-" + hash[:1],
	}
}

func TestYearBoundariesFollowLocalTime(t *testing.T) {
	toronto := loadLoc(t, "America/Toronto")
	tokyo := loadLoc(t, "Asia/Tokyo")
	cases := []struct {
		name      string
		loc       *time.Location
		wall      string
		wantIn    int // year the play belongs to
		wantPlays map[int]int
	}{
		{"last minute of the year", toronto, "2024-12-31 23:59", 2024, map[int]int{2024: 1, 2025: 0}},
		{"first minute of the year", toronto, "2025-01-01 00:00", 2025, map[int]int{2024: 0, 2025: 1}},
		// 2025-01-01 03:00 UTC is still New Year's Eve in Toronto: UTC would
		// file it under the wrong year.
		{"utc new year is local new year's eve", toronto, "2024-12-31 22:00", 2024, map[int]int{2024: 1, 2025: 0}},
		{"ahead of utc", tokyo, "2025-01-01 08:00", 2025, map[int]int{2024: 0, 2025: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plays := []insights.PlayRow{row("a1", at(t, tc.loc, tc.wall), 200)}
			if got := YearsWithPlays(plays, tc.loc); !reflect.DeepEqual(got, []int{tc.wantIn}) {
				t.Fatalf("YearsWithPlays = %v, want [%d]", got, tc.wantIn)
			}
			for year, want := range tc.wantPlays {
				r := Compute(Input{Plays: plays, Loc: tc.loc, Period: YearPeriod(year, tc.loc)})
				if r.Totals.Plays != want {
					t.Errorf("%d plays = %d, want %d", year, r.Totals.Plays, want)
				}
			}
		})
	}
}

func TestRollingPeriodIncludesTodayAndStopsAYearBack(t *testing.T) {
	loc := loadLoc(t, "America/Toronto")
	now := time.Unix(at(t, loc, "2025-03-10 09:00"), 0)
	p := RollingPeriod(now, loc)
	if !p.Rolling() || p.Label() != "Last 12 months" {
		t.Fatalf("period = %+v, want the rolling period", p)
	}
	cases := []struct {
		wall string
		in   bool
	}{
		{"2025-03-10 23:30", true},  // later today
		{"2024-03-11 00:00", true},  // first day of the window
		{"2024-03-10 23:59", false}, // the same day a year ago
		{"2025-03-11 00:00", false}, // tomorrow
	}
	for _, c := range cases {
		if got := p.Contains(at(t, loc, c.wall)); got != c.in {
			t.Errorf("Contains(%s) = %v, want %v", c.wall, got, c.in)
		}
	}
	prev := p.Previous()
	// The twelve months before span Feb 29, 2024, so they are a day longer.
	if !prev.End.Equal(p.Start) || prev.Start.Format(time.DateOnly) != "2023-03-11" || prev.Days() != 366 {
		t.Errorf("Previous = %v..%v (%d days), want 2023-03-11 up to %v", prev.Start, prev.End, prev.Days(), p.Start)
	}
}

func TestEmptyPeriods(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		name  string
		input Input
	}{
		{"zero input", Input{}},
		{"no plays at all", Input{Loc: loc, Period: YearPeriod(2025, loc)}},
		{"plays only in another year", Input{
			Loc:    loc,
			Period: YearPeriod(2025, loc),
			Plays:  []insights.PlayRow{row("a1", at(t, loc, "2024-06-01 10:00"), 100)},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute(tc.input)
			if !r.Empty() {
				t.Fatalf("recap = %+v, want empty", r.Totals)
			}
			if r.TopArtists != nil || r.TopTracks != nil || r.Playlist != nil || r.Streak.Days != 0 || r.Discovery.Hash != "" {
				t.Errorf("empty recap carries data: %+v", r)
			}
		})
	}
}

func TestTotalsAndComparisonWithThePreviousYear(t *testing.T) {
	loc := loadLoc(t, "America/Toronto")
	plays := []insights.PlayRow{
		row("a1", at(t, loc, "2024-05-01 10:00"), 300),
		row("a1", at(t, loc, "2025-01-02 10:00"), 300),
		row("a2", at(t, loc, "2025-01-02 11:00"), 200),
		// A counted play that stopped early adds what was heard, not the
		// track's length; a negative reading never subtracts.
		row("b1", at(t, loc, "2025-02-03 10:00"), 45),
		row("b1", at(t, loc, "2025-02-03 12:00"), -5),
	}
	r := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)})
	want := Totals{Plays: 4, Secs: 545, Tracks: 3, Artists: 2, Albums: 2, Days: 2}
	if r.Totals != want {
		t.Errorf("Totals = %+v, want %+v", r.Totals, want)
	}
	if prev := (Totals{Plays: 1, Secs: 300, Tracks: 1, Artists: 1, Albums: 1, Days: 1}); r.Previous != prev {
		t.Errorf("Previous = %+v, want %+v", r.Previous, prev)
	}
	if r.FirstPeriod {
		t.Error("2025 is not the first period: 2024 had plays")
	}
	first := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2024, loc)})
	if first.Previous != (Totals{}) || !first.FirstPeriod {
		t.Errorf("2024 Previous = %+v first=%v, want zero and first: nothing was played before 2024", first.Previous, first.FirstPeriod)
	}
	// A silent year between two busy ones is not a first year.
	gap := slices.Concat(plays, []insights.PlayRow{row("c1", at(t, loc, "2027-03-01 10:00"), 100)})
	if r := Compute(Input{Plays: gap, Loc: loc, Period: YearPeriod(2027, loc)}); r.Previous.Plays != 0 || r.FirstPeriod {
		t.Errorf("2027 after a silent 2026: Previous %+v, first %v", r.Previous, r.FirstPeriod)
	}
}

func TestRankingsAndPlaylist(t *testing.T) {
	loc := time.UTC
	day := func(d int, h int) int64 { return time.Date(2025, 4, d, h, 0, 0, 0, loc).Unix() }
	var plays []insights.PlayRow
	add := func(hash string, n int) {
		for i := range n {
			plays = append(plays, row(hash, day(1+i, 9), 100))
		}
	}
	add("a1", 5)
	add("a2", 3)
	add("b1", 4)
	add("c1", 1)
	r := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)})

	if got := hashes(r.TopTracks); !reflect.DeepEqual(got, []string{"a1", "b1", "a2", "c1"}) {
		t.Errorf("TopTracks = %v", got)
	}
	if !reflect.DeepEqual(r.Playlist, []string{"a1", "b1", "a2", "c1"}) {
		t.Errorf("Playlist = %v", r.Playlist)
	}
	wantArtist := Entry{Name: "A-a", Album: "L-a", Hash: "a1", Plays: 8, Secs: 800}
	if len(r.TopArtists) != 3 || r.TopArtists[0] != wantArtist {
		t.Errorf("TopArtists = %+v, want first %+v", r.TopArtists, wantArtist)
	}
	if r.TopAlbum.Name != "L-a" || r.TopAlbum.Hash != "a1" || r.TopAlbum.Plays != 8 {
		t.Errorf("TopAlbum = %+v", r.TopAlbum)
	}
}

func TestPlaylistStopsAtFifty(t *testing.T) {
	loc := time.UTC
	var plays []insights.PlayRow
	for i := range 60 {
		plays = append(plays, row(string(rune('a'+i%26))+itoa3(i), time.Date(2025, 1, 1, 0, i, 0, 0, loc).Unix(), 60))
	}
	r := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)})
	if len(r.Playlist) != PlaylistSize || len(r.TopTracks) != TopN {
		t.Errorf("playlist %d, top tracks %d; want %d and %d", len(r.Playlist), len(r.TopTracks), PlaylistSize, TopN)
	}
}

func itoa3(i int) string { return fmt.Sprintf("%03d", i) }

func TestClockUsesLocalStartTime(t *testing.T) {
	loc := loadLoc(t, "America/Toronto")
	plays := []insights.PlayRow{
		// Sunday evening in Toronto, already Monday in UTC.
		row("a1", at(t, loc, "2025-03-02 21:10"), 200),
		row("a1", at(t, loc, "2025-03-09 21:40"), 200),
		row("a2", at(t, loc, "2025-03-04 08:00"), 200),
	}
	c := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)}).Clock
	if c.PeakHour != 21 || c.Hours[21] != 2 || c.Hours[8] != 1 {
		t.Errorf("hours: peak %d, %v", c.PeakHour, c.Hours)
	}
	if c.PeakWeekday != time.Sunday || c.Weekdays[time.Sunday] != 2 {
		t.Errorf("weekdays: peak %v, %v", c.PeakWeekday, c.Weekdays)
	}
}

func TestLongestStreak(t *testing.T) {
	loc := loadLoc(t, "America/Toronto")
	cases := []struct {
		name       string
		walls      []string
		days       int
		start, end string
	}{
		{"single day", []string{"2025-06-01 10:00"}, 1, "2025-06-01", "2025-06-01"},
		{"three in a row beats two", []string{
			"2025-06-01 10:00", "2025-06-02 10:00",
			"2025-06-10 10:00", "2025-06-11 23:30", "2025-06-12 00:10",
		}, 3, "2025-06-10", "2025-06-12"},
		{"earliest run wins a tie", []string{
			"2025-02-01 10:00", "2025-02-02 10:00", "2025-05-01 10:00", "2025-05-02 10:00",
		}, 2, "2025-02-01", "2025-02-02"},
		// The 23-hour day of the spring DST change is still one day.
		{"across a dst change", []string{"2025-03-08 12:00", "2025-03-09 12:00", "2025-03-10 12:00"}, 3, "2025-03-08", "2025-03-10"},
		{"runs stop at the year's edge", []string{"2024-12-30 12:00", "2024-12-31 12:00", "2025-01-01 12:00"}, 1, "2025-01-01", "2025-01-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var plays []insights.PlayRow
			for _, w := range tc.walls {
				plays = append(plays, row("a1", at(t, loc, w), 100))
			}
			s := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)}).Streak
			if s.Days != tc.days || s.Start.Format(time.DateOnly) != tc.start || s.End.Format(time.DateOnly) != tc.end {
				t.Errorf("streak = %d days %s..%s, want %d days %s..%s",
					s.Days, s.Start.Format(time.DateOnly), s.End.Format(time.DateOnly), tc.days, tc.start, tc.end)
			}
		})
	}
}

func TestDailyCoversEveryDayOfTheYear(t *testing.T) {
	loc := loadLoc(t, "America/Toronto")
	plays := []insights.PlayRow{row("a1", at(t, loc, "2024-02-29 10:00"), 100)}
	r := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2024, loc)})
	if len(r.Daily) != 366 {
		t.Fatalf("Daily has %d days, want 366 in a leap year", len(r.Daily))
	}
	if d := r.Daily[59]; d.Date.Format(time.DateOnly) != "2024-02-29" || d.Plays != 1 {
		t.Errorf("Daily[59] = %v %d", d.Date, d.Plays)
	}
}

func TestDiscoveryNeedsTheFirstPlayEverInThePeriod(t *testing.T) {
	loc := time.UTC
	ts := func(m time.Month, d int) int64 { return time.Date(2025, m, d, 12, 0, 0, 0, loc).Unix() }
	plays := []insights.PlayRow{
		// a1 is the year's most played, but was first heard in 2024.
		row("a1", time.Date(2024, 12, 31, 23, 0, 0, 0, loc).Unix(), 100),
		row("a1", ts(1, 1), 100), row("a1", ts(1, 2), 100), row("a1", ts(1, 3), 100), row("a1", ts(1, 4), 100),
		row("b1", ts(3, 5), 100), row("b1", ts(3, 6), 100), row("b1", ts(4, 1), 100),
		row("c1", ts(2, 1), 100),
	}
	d := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)}).Discovery
	if d.Hash != "b1" || d.Plays != 3 || d.NewTracks != 2 || d.FirstHeard.Format(time.DateOnly) != "2025-03-05" {
		t.Errorf("Discovery = %+v", d)
	}
	none := Compute(Input{Plays: plays[:5], Loc: loc, Period: YearPeriod(2025, loc)}).Discovery
	if none.Hash != "" || none.NewTracks != 0 {
		t.Errorf("Discovery with nothing new = %+v, want none", none)
	}
}

func TestRepeatDay(t *testing.T) {
	loc := loadLoc(t, "America/Toronto")
	cases := []struct {
		name  string
		plays []insights.PlayRow
		hash  string
		count int
		date  string
	}{
		{"one play is no repeat", []insights.PlayRow{row("a1", at(t, loc, "2025-04-01 10:00"), 100)}, "", 0, ""},
		{"most plays in one local day", []insights.PlayRow{
			row("a1", at(t, loc, "2025-04-01 10:00"), 100),
			row("a1", at(t, loc, "2025-04-01 23:50"), 100),
			// Same UTC day as the two above, but the next local day.
			row("b1", at(t, loc, "2025-04-02 00:05"), 100),
			row("b1", at(t, loc, "2025-04-02 00:15"), 100),
			row("b1", at(t, loc, "2025-04-02 09:00"), 100),
		}, "b1", 3, "2025-04-02"},
		{"earlier day wins a tie", []insights.PlayRow{
			row("b1", at(t, loc, "2025-05-09 10:00"), 100),
			row("b1", at(t, loc, "2025-05-09 11:00"), 100),
			row("a1", at(t, loc, "2025-05-08 10:00"), 100),
			row("a1", at(t, loc, "2025-05-08 11:00"), 100),
		}, "a1", 2, "2025-05-08"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute(Input{Plays: tc.plays, Loc: loc, Period: YearPeriod(2025, loc)}).RepeatDay
			date := ""
			if !r.Date.IsZero() {
				date = r.Date.Format(time.DateOnly)
			}
			if r.Hash != tc.hash || r.Plays != tc.count || date != tc.date {
				t.Errorf("RepeatDay = %s ×%d on %q, want %s ×%d on %q", r.Hash, r.Plays, date, tc.hash, tc.count, tc.date)
			}
		})
	}
}

func TestGenreMixFoldsSpellingsAndCountsEveryGenre(t *testing.T) {
	loc := time.UTC
	genres := map[string][]string{
		"a1": {"Ambient", "Electronic"},
		"a2": {"ambient "},
		"b1": {"Jazz"},
		"c1": nil,
	}
	ts := time.Date(2025, 7, 1, 12, 0, 0, 0, loc).Unix()
	plays := []insights.PlayRow{row("a1", ts, 100), row("a1", ts+1, 100), row("a2", ts+2, 100), row("b1", ts+3, 100), row("c1", ts+4, 100)}
	got := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc), GenresOf: func(h string) []string { return genres[h] }}).Genres
	want := []GenreShare{
		{Name: "Ambient", Plays: 3, Share: 0.5},
		{Name: "Electronic", Plays: 2, Share: 2.0 / 6},
		{Name: "Jazz", Plays: 1, Share: 1.0 / 6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Genres = %+v, want %+v", got, want)
	}
	if g := Compute(Input{Plays: plays, Loc: loc, Period: YearPeriod(2025, loc)}).Genres; g != nil {
		t.Errorf("Genres without a lookup = %+v, want none", g)
	}
}
