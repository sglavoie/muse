package tui

// The fixtures below are duplicates of the ones that travelled into
// internal/tui/insights with insights_panels_test.go and
// insights_rankings_test.go. The Model-bound insights tests that stayed here
// still call them, and a test helper cannot be shared across a package
// boundary without exporting it.

import (
	"time"

	"github.com/sglavoie/muse/internal/insights"
	instui "github.com/sglavoie/muse/internal/tui/insights"
)

// panelsTestLoc is fixed so a bucket's hour, weekday and month cannot depend on
// where the test runs.
func panelsTestLoc() *time.Location { return time.UTC }

// at builds a UTC instant for the fixed 2026 dates these tests bucket against.
func at(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 30, 0, 0, panelsTestLoc())
}

func sectionLabels(rows []instui.Row, section string) []string {
	var out []string
	for _, r := range rows {
		if r.Section == section {
			out = append(out, r.Label)
		}
	}
	return out
}

// rankingsFixture is four artists whose two orderings deliberately disagree:
// Dana is played often but briefly, Carol rarely but at length.
func rankingsFixture() insights.Rankings {
	return insights.Rankings{
		ArtistsByPlays: []insights.EntityStat{
			{Name: "Alice", Plays: 40, ListenedSecs: 9000, Rank: 1},
			{Name: "Dana", Plays: 30, ListenedSecs: 1200, Rank: 2},
			{Name: "Bob", Plays: 20, ListenedSecs: 6000, Rank: 3},
			{Name: "Carol", Plays: 5, ListenedSecs: 4000, Rank: 4},
		},
		ArtistsByHours: []insights.EntityStat{
			{Name: "Alice", Plays: 40, ListenedSecs: 9000, Rank: 1},
			{Name: "Bob", Plays: 20, ListenedSecs: 6000, Rank: 2},
			{Name: "Carol", Plays: 5, ListenedSecs: 4000, Rank: 3},
			{Name: "Dana", Plays: 30, ListenedSecs: 1200, Rank: 4},
		},
		Deltas: []insights.RankDelta{
			{Name: "Alice", PlayRank: 1, HoursRank: 1, Delta: 0},
			{Name: "Dana", PlayRank: 2, HoursRank: 4, Delta: -2},
			{Name: "Bob", PlayRank: 3, HoursRank: 2, Delta: 1},
			{Name: "Carol", PlayRank: 4, HoursRank: 3, Delta: 1},
		},
		MovedCount:   3,
		TopFiveShare: 0.75,
		Ladder:       insights.Ladder{TotalArtists: 40, Artists25: 1, Artists50: 3, Artists75: 8, Artists90: 17},
	}
}
