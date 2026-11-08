package recap

import (
	"sort"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/insights"
)

// clock buckets plays by the local hour and weekday of their start. Ties go
// to the earlier hour and to the earlier weekday in a Monday-first week.
func clock(plays []insights.PlayRow, loc *time.Location) Clock {
	var c Clock
	for _, p := range plays {
		t := time.Unix(p.StartAt, 0).In(loc)
		c.Hours[t.Hour()]++
		c.Weekdays[t.Weekday()]++
	}
	for h, n := range c.Hours {
		if n > c.Hours[c.PeakHour] {
			c.PeakHour = h
		}
	}
	c.PeakWeekday = time.Monday
	for _, d := range MondayFirst {
		if c.Weekdays[d] > c.Weekdays[c.PeakWeekday] {
			c.PeakWeekday = d
		}
	}
	return c
}

// MondayFirst is the weekday order the recap draws in.
var MondayFirst = [7]time.Weekday{
	time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday,
}

// daily lays the period out one local day at a time, silent days included.
func daily(plays []insights.PlayRow, p Period, loc *time.Location) []DayCount {
	counts := map[time.Time]int{}
	for _, row := range plays {
		counts[localDay(row.StartAt, loc)]++
	}
	start := p.Start.In(loc)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	out := make([]DayCount, 0, p.Days())
	for day := start; day.Before(p.End); day = nextDay(day) {
		out = append(out, DayCount{Date: day, Plays: counts[day]})
	}
	return out
}

// longestStreak finds the longest run of days with plays; the earliest run
// wins a tie.
func longestStreak(days []DayCount) Streak {
	var best, run Streak
	for _, d := range days {
		if d.Plays == 0 {
			run = Streak{}
			continue
		}
		if run.Days == 0 {
			run.Start = d.Date
		}
		run.Days++
		run.End = d.Date
		if run.Days > best.Days {
			best = run
		}
	}
	return best
}

// discovery picks, among the tracks whose first play ever started inside the
// period, the one played most within it. tracks must be the period's ranking,
// so its order is the tie-break.
func discovery(all []insights.PlayRow, p Period, tracks []Entry, loc *time.Location) Discovery {
	first := map[string]int64{}
	for _, row := range all {
		if at, ok := first[row.Hash]; !ok || row.StartAt < at {
			first[row.Hash] = row.StartAt
		}
	}
	var d Discovery
	for _, t := range tracks {
		at, ok := first[t.Hash]
		if !ok || !p.Contains(at) {
			continue
		}
		d.NewTracks++
		if d.Hash == "" {
			d.Entry, d.FirstHeard = t, localDay(at, loc)
		}
	}
	return d
}

// repeatDay finds the most plays one track got in a single local day. One
// play is no repeat, so a period without a second play of anything on the
// same day has none.
func repeatDay(plays []insights.PlayRow, loc *time.Location) RepeatDay {
	type key struct {
		day  time.Time
		hash string
	}
	byKey := map[key]*RepeatDay{}
	var order []*RepeatDay
	for _, p := range plays {
		if p.Hash == "" {
			continue
		}
		k := key{localDay(p.StartAt, loc), p.Hash}
		r := byKey[k]
		if r == nil {
			r = &RepeatDay{Entry: Entry{Name: p.Title, Artist: p.Artist, Album: p.Album, Hash: p.Hash}, Date: k.day}
			byKey[k] = r
			order = append(order, r)
		}
		r.Plays++
		r.Secs += secsOf(p)
	}
	var best RepeatDay
	for _, r := range order {
		if r.Plays < 2 {
			continue
		}
		if best.Plays == 0 || r.Plays > best.Plays || (r.Plays == best.Plays && r.Date.Before(best.Date)) {
			best = *r
		}
	}
	return best
}

// genreMix shares the period's plays out by genre. A track tagged with two
// genres counts toward both, as the Insights genre readers do, and spellings
// that differ only in case or spacing are one genre named after its most
// common spelling.
func genreMix(plays []insights.PlayRow, genresOf func(string) []string) []GenreShare {
	if genresOf == nil {
		return nil
	}
	type group struct {
		plays    int
		spelling map[string]int
	}
	groups := map[string]*group{}
	total := 0
	for _, p := range plays {
		for _, g := range genresOf(p.Hash) {
			k := strings.ToLower(strings.TrimSpace(g))
			if k == "" {
				continue
			}
			gr := groups[k]
			if gr == nil {
				gr = &group{spelling: map[string]int{}}
				groups[k] = gr
			}
			gr.plays++
			gr.spelling[strings.TrimSpace(g)]++
			total++
		}
	}
	out := make([]GenreShare, 0, len(groups))
	for _, gr := range groups {
		out = append(out, GenreShare{Name: commonest(gr.spelling), Plays: gr.plays, Share: float64(gr.plays) / float64(total)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Plays != out[j].Plays {
			return out[i].Plays > out[j].Plays
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > GenreN {
		out = out[:GenreN]
	}
	return out
}

func commonest(counts map[string]int) string {
	best, n := "", -1
	for s, c := range counts {
		if c > n || (c == n && s < best) {
			best, n = s, c
		}
	}
	return best
}
