package insights

import "testing"

// toumaniPlays reproduces the divergence found in the real library: Toumani
// Diabaté is only the second most *played* artist but the most *listened to*,
// because his tracks are long, while an artist of short tracks tops the play
// count and falls two places when ranked by time.
func toumaniPlays() []PlayRow {
	shape := []struct {
		artist   string
		plays    int
		secsEach int
	}{
		{"Nightly Shorts", 100, 180},  //  5.0 h
		{"Toumani Diabaté", 58, 560},  //  9.0 h
		{"A Winged Victory", 40, 792}, //  8.8 h
		{"Filler", 10, 60},            //  0.2 h
	}
	var plays []PlayRow
	for _, s := range shape {
		for i := 0; i < s.plays; i++ {
			plays = append(plays, PlayRow{
				Hash:         s.artist,
				StartAt:      1774000000 + int64(i*3600),
				ListenedSecs: s.secsEach,
				Artist:       s.artist,
			})
		}
	}
	return plays
}

func TestRankingsRankDivergence(t *testing.T) {
	r := computeRankings(toumaniPlays())

	wantByPlays := []string{"Nightly Shorts", "Toumani Diabaté", "A Winged Victory", "Filler"}
	for i, want := range wantByPlays {
		got := r.ArtistsByPlays[i]
		if got.Name != want || got.Rank != i+1 {
			t.Errorf("ArtistsByPlays[%d] = %s/rank %d, want %s/rank %d", i, got.Name, got.Rank, want, i+1)
		}
	}
	wantByHours := []string{"Toumani Diabaté", "A Winged Victory", "Nightly Shorts", "Filler"}
	for i, want := range wantByHours {
		got := r.ArtistsByHours[i]
		if got.Name != want || got.Rank != i+1 {
			t.Errorf("ArtistsByHours[%d] = %s/rank %d, want %s/rank %d", i, got.Name, got.Rank, want, i+1)
		}
	}

	// Delta is PlayRank - HoursRank: positive means the artist climbs when
	// listening time is what is measured.
	wantDeltas := []RankDelta{
		{Name: "Nightly Shorts", PlayRank: 1, HoursRank: 3, Delta: -2},
		{Name: "Toumani Diabaté", PlayRank: 2, HoursRank: 1, Delta: 1},
		{Name: "A Winged Victory", PlayRank: 3, HoursRank: 2, Delta: 1},
		{Name: "Filler", PlayRank: 4, HoursRank: 4, Delta: 0},
	}
	if len(r.Deltas) != len(wantDeltas) {
		t.Fatalf("Deltas = %d, want %d", len(r.Deltas), len(wantDeltas))
	}
	for i, want := range wantDeltas {
		if r.Deltas[i] != want {
			t.Errorf("Deltas[%d] = %+v, want %+v", i, r.Deltas[i], want)
		}
	}
	if r.MovedCount != 3 {
		t.Errorf("MovedCount = %d, want 3", r.MovedCount)
	}
}

func TestRankingsLadderAndTopFiveShare(t *testing.T) {
	r := computeRankings(toumaniPlays())

	// Total listening time is 82,760 s; Toumani alone clears a quarter of it.
	want := Ladder{TotalArtists: 4, Artists25: 1, Artists50: 2, Artists75: 2, Artists90: 3}
	if r.Ladder != want {
		t.Errorf("Ladder = %+v, want %+v", r.Ladder, want)
	}
	if r.TopFiveShare != 1 {
		t.Errorf("TopFiveShare = %v, want 1 with only four artists", r.TopFiveShare)
	}
}

func TestRankingsCapsDeltasAtTopN(t *testing.T) {
	var plays []PlayRow
	for i := 0; i < rankDeltaTopN+5; i++ {
		plays = append(plays, PlayRow{
			Artist:       string(rune('a' + i)),
			ListenedSecs: 100 + i,
			StartAt:      1774000000,
		})
	}

	r := computeRankings(plays)

	if len(r.Deltas) != rankDeltaTopN {
		t.Errorf("Deltas = %d, want %d", len(r.Deltas), rankDeltaTopN)
	}
}

func TestRankingsIgnoresUnnamedArtists(t *testing.T) {
	r := computeRankings([]PlayRow{
		{Artist: "", ListenedSecs: 600, StartAt: 1774000000},
		{Artist: "", ListenedSecs: 600, StartAt: 1774000001},
	})
	if !reflectZeroRankings(r) {
		t.Errorf("computeRankings(unnamed only) = %+v, want zero Rankings", r)
	}
}

func TestRankingsEmptyIsZero(t *testing.T) {
	if !reflectZeroRankings(computeRankings(nil)) {
		t.Error("computeRankings(nil) is not the zero Rankings")
	}
}

func reflectZeroRankings(r Rankings) bool {
	return r.ArtistsByPlays == nil && r.ArtistsByHours == nil && r.Deltas == nil &&
		r.MovedCount == 0 && r.Ladder == (Ladder{}) && r.TopFiveShare == 0
}
