package insights

import (
	"testing"
	"time"
)

// TestRhythmBucketsByPlayStart is the whole reason this package exists:
// play_history.played_at is stamped when playback ends, so a play that began
// at 15:55 and ended at 16:05 belongs to 15:00, not 16:00.
func TestRhythmBucketsByPlayStart(t *testing.T) {
	loc := testLoc(t)
	endedAt := at(t, loc, "2026-08-20 16:05:00")

	r := computeRhythm([]PlayRow{play(endedAt, 600, "A")}, nil, loc)

	if r.Hours[15].Plays != 1 {
		t.Errorf("hour 15 = %d plays, want 1 (bucketed by start)", r.Hours[15].Plays)
	}
	if r.Hours[16].Plays != 0 {
		t.Errorf("hour 16 = %d plays, want 0 (played_at is the END of the play)", r.Hours[16].Plays)
	}
	if r.Hours[15].Hour != 15 {
		t.Errorf("Hours[15].Hour = %d, want 15", r.Hours[15].Hour)
	}
}

func TestRhythmSpringForwardDST(t *testing.T) {
	loc := testLoc(t)
	// 2026-03-08 in America/Toronto: 02:00 EST jumps straight to 03:00 EDT.
	// This play starts at 01:50 EST and ends at 03:10 EDT — 20 minutes later
	// on the clock face but four hours apart if you only read the hour field.
	endedAt := at(t, loc, "2026-03-08 03:10:00")
	plays := []PlayRow{
		play(endedAt, 1200, "A"),
		play(at(t, loc, "2026-03-08 04:00:00"), 600, "B"), // starts 03:50 EDT
	}

	r := computeRhythm(plays, nil, loc)

	if r.Hours[1].Plays != 1 {
		t.Errorf("hour 1 = %d plays, want 1 (started 01:50 EST)", r.Hours[1].Plays)
	}
	if r.Hours[3].Plays != 1 {
		t.Errorf("hour 3 = %d plays, want 1 (started 03:50 EDT)", r.Hours[3].Plays)
	}
	if r.Hours[2].Plays != 0 {
		t.Errorf("hour 2 = %d plays, want 0; 02:00 does not exist on this date", r.Hours[2].Plays)
	}
	if got := r.Weekdays[time.Sunday].Plays; got != 2 {
		t.Errorf("Sunday = %d plays, want 2", got)
	}
}

func TestRhythmFallBackDSTFoldsRepeatedHour(t *testing.T) {
	loc := testLoc(t)
	// 2026-11-01: 02:00 EDT falls back to 01:00 EST, so 01:30 happens twice.
	// Both occurrences are pinned from UTC rather than parsed, because parsing
	// an ambiguous wall clock has no defined answer.
	firstPass := time.Date(2026, time.November, 1, 5, 30, 0, 0, time.UTC).Unix()  // 01:30 EDT
	secondPass := time.Date(2026, time.November, 1, 6, 30, 0, 0, time.UTC).Unix() // 01:30 EST
	for _, ts := range []int64{firstPass, secondPass} {
		if h := time.Unix(ts, 0).In(loc).Hour(); h != 1 {
			t.Fatalf("fixture %d is local hour %d, want 1", ts, h)
		}
	}

	r := computeRhythm([]PlayRow{
		{StartAt: firstPass, ListenedSecs: 300, Artist: "A"},
		{StartAt: secondPass, ListenedSecs: 300, Artist: "A"},
	}, nil, loc)

	if r.Hours[1].Plays != 2 {
		t.Errorf("hour 1 = %d plays, want 2; both 01:30 occurrences bucket to hour 1", r.Hours[1].Plays)
	}
	if r.Hours[2].Plays != 0 {
		t.Errorf("hour 2 = %d plays, want 0", r.Hours[2].Plays)
	}
}

func TestRhythmPeakQuietAndWeekdayShares(t *testing.T) {
	loc := testLoc(t)
	var plays []PlayRow
	for i := 0; i < 3; i++ {
		plays = append(plays, play(at(t, loc, "2026-08-17 15:30:00"), 600, "A")) // Monday
	}
	plays = append(plays, play(at(t, loc, "2026-08-16 07:30:00"), 600, "B")) // Sunday

	r := computeRhythm(plays, nil, loc)

	if r.PeakHour != 15 || r.PeakHourPlays != 3 {
		t.Errorf("peak = %dh/%d plays, want 15h/3", r.PeakHour, r.PeakHourPlays)
	}
	if r.TopWeekday.Weekday != time.Monday || r.TopWeekday.Plays != 3 {
		t.Errorf("top weekday = %v/%d, want Monday/3", r.TopWeekday.Weekday, r.TopWeekday.Plays)
	}
	if got := r.Weekdays[time.Monday].Share; got != 0.75 {
		t.Errorf("Monday share = %v, want 0.75", got)
	}
	// Tuesday is the first weekday with zero plays after Sunday and Monday.
	if r.BottomWeekday.Plays != 0 {
		t.Errorf("bottom weekday = %v/%d, want a zero-play weekday", r.BottomWeekday.Weekday, r.BottomWeekday.Plays)
	}
	if len(r.QuietHours) != 22 {
		t.Errorf("quiet hours = %d, want 22 (24 minus 07h and 15h)", len(r.QuietHours))
	}
	if r.DistinctTracks != 2 {
		t.Errorf("DistinctTracks = %d, want 2", r.DistinctTracks)
	}
}

func TestRhythmEmptyIsZero(t *testing.T) {
	if got := computeRhythm(nil, nil, testLoc(t)); got.TotalPlays != 0 || got.Genres != nil {
		t.Errorf("computeRhythm(nil, nil) = %+v, want zero Rhythm", got)
	}
}

func TestRhythmFlagsDirtyGenres(t *testing.T) {
	groups := foldGenres([]GenreRow{{Genre: "Ambient", Plays: 2}, {Genre: "ambient", Plays: 1}})
	r := computeRhythm(nil, groups, testLoc(t))
	if !r.GenresUnclean {
		t.Error("GenresUnclean = false, want true when a group has multiple variants")
	}
}
