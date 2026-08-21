package insights

import (
	"math"
	"reflect"
	"testing"
)

func TestCoverageReproducesTheLibraryShape(t *testing.T) {
	const total, heard = 2382, 916
	rows := make([]CoverageRow, 0, total)
	for i := 0; i < total; i++ {
		row := CoverageRow{Hash: string(rune(i)), Artist: "A", Album: "B", DurationSecs: 361.7}
		if i < heard {
			row.PlayCount = 1
		}
		rows = append(rows, row)
	}
	timeline := []TimelineRow{{StartAt: 1774000000, ListenedSecs: 549720}} // 152.7 h

	c := computeCoverage(rows, timeline, Trends{AvgMonthlyFirstPlays: 38})

	if c.TotalTracks != total || c.HeardTracks != heard || c.UnheardTracks != total-heard {
		t.Fatalf("coverage = %d total / %d heard / %d unheard, want %d/%d/%d",
			c.TotalTracks, c.HeardTracks, c.UnheardTracks, total, heard, total-heard)
	}
	if math.Abs(c.HeardShare-0.3845) > 0.001 {
		t.Errorf("HeardShare = %v, want ~0.3845", c.HeardShare)
	}
	// The unheard part is about the same size as everything heard so far.
	if math.Abs(c.UnheardVsHeard-0.964) > 0.01 {
		t.Errorf("UnheardVsHeard = %v, want ~0.96", c.UnheardVsHeard)
	}
	if math.Abs(c.MonthsToFinish-38.6) > 0.1 {
		t.Errorf("MonthsToFinish = %v, want ~38.6", c.MonthsToFinish)
	}
}

func TestCoverageDivisionByZero(t *testing.T) {
	cases := []struct {
		name     string
		rows     []CoverageRow
		timeline []TimelineRow
		trends   Trends
	}{
		{
			name: "no listening time at all",
			rows: []CoverageRow{{Hash: "a", Artist: "A", DurationSecs: 300}},
		},
		{
			name:     "history but no library rows",
			timeline: []TimelineRow{{StartAt: 1774000000, ListenedSecs: 600}},
		},
		{
			name:   "no discovery rate to project with",
			rows:   []CoverageRow{{Hash: "a", Artist: "A", DurationSecs: 300}},
			trends: Trends{AvgMonthlyFirstPlays: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := computeCoverage(tc.rows, tc.timeline, tc.trends)
			for name, v := range map[string]float64{
				"HeardShare":     c.HeardShare,
				"UnheardVsHeard": c.UnheardVsHeard,
				"MonthsToFinish": c.MonthsToFinish,
			} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("%s = %v, want a finite number", name, v)
				}
			}
		})
	}
}

func TestCoverageSanitizesDurations(t *testing.T) {
	rows := []CoverageRow{
		{Hash: "a", Artist: "A", DurationSecs: math.NaN()},
		{Hash: "b", Artist: "A", DurationSecs: math.Inf(1)},
		{Hash: "c", Artist: "A", DurationSecs: -300},
		{Hash: "d", Artist: "A", DurationSecs: 300},
	}

	c := computeCoverage(rows, nil, Trends{})

	if c.UnheardSecs != 300 {
		t.Errorf("UnheardSecs = %v, want 300; non-finite and negative durations count as 0", c.UnheardSecs)
	}
}

func TestCoverageRanksDustiestShelvesFirst(t *testing.T) {
	rows := []CoverageRow{
		{Hash: "a", Artist: "Dusty", Album: "One", DurationSecs: 300},
		{Hash: "b", Artist: "Dusty", Album: "One", DurationSecs: 300},
		{Hash: "c", Artist: "Dusty", Album: "One", DurationSecs: 300},
		{Hash: "d", Artist: "Half", Album: "Two", DurationSecs: 300},
		{Hash: "e", Artist: "Half", Album: "Two", DurationSecs: 300, PlayCount: 4},
		{Hash: "f", Artist: "Played", Album: "Three", DurationSecs: 300, PlayCount: 9},
	}

	c := computeCoverage(rows, nil, Trends{})

	if len(c.DustyArtists) != 2 {
		t.Fatalf("DustyArtists = %+v, want only the two with unheard tracks", c.DustyArtists)
	}
	if c.DustyArtists[0].Name != "Dusty" || c.DustyArtists[0].Unheard != 3 || c.DustyArtists[0].Tracks != 3 {
		t.Errorf("dustiest artist = %+v, want Dusty with 3 of 3 unheard", c.DustyArtists[0])
	}
	if c.DustyArtists[1].Name != "Half" || c.DustyArtists[1].Unheard != 1 || c.DustyArtists[1].Tracks != 2 {
		t.Errorf("second artist = %+v, want Half with 1 of 2 unheard", c.DustyArtists[1])
	}
	if len(c.DustyAlbums) != 2 {
		t.Errorf("DustyAlbums = %+v, want two", c.DustyAlbums)
	}
}

func TestCoverageEmptyIsZero(t *testing.T) {
	if got := computeCoverage(nil, nil, Trends{}); !reflect.DeepEqual(got, Coverage{}) {
		t.Errorf("computeCoverage(nil, nil) = %+v, want zero Coverage", got)
	}
}
