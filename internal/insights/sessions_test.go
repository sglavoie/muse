package insights

import (
	"reflect"
	"testing"
)

func sessionRows() []SessionRow {
	return []SessionRow{
		{ID: "1", StartedAt: 1000, EndedAt: 3400, Plays: 8, ListenedSecs: 2300, DistinctArtists: 3,
			OpenerArtist: "Toumani", OpenerTitle: "Kaira", CloserArtist: "AWV", CloserTitle: "Steep"},
		{ID: "2", StartedAt: 10000, EndedAt: 10300, Plays: 1, ListenedSecs: 300, DistinctArtists: 1,
			OpenerArtist: "Toumani", OpenerTitle: "Kaira", CloserArtist: "Toumani", CloserTitle: "Kaira"},
		{ID: "3", StartedAt: 20000, EndedAt: 26000, Plays: 12, ListenedSecs: 5400, DistinctArtists: 1,
			OpenerArtist: "AWV", OpenerTitle: "Atomos", CloserArtist: "AWV", CloserTitle: "Atomos"},
		{ID: "4", StartedAt: 30000, EndedAt: 31200, Plays: 4, ListenedSecs: 1100, DistinctArtists: 2,
			OpenerArtist: "Toumani", OpenerTitle: "Kaira", CloserArtist: "Bach", CloserTitle: "Aria"},
	}
}

func TestSessionsSummary(t *testing.T) {
	s := computeSessions(sessionRows())

	if s.Count != 4 {
		t.Fatalf("Count = %d, want 4", s.Count)
	}
	if want := []int{300, 1200, 2400, 6000}; !reflect.DeepEqual(s.Durations, want) {
		t.Errorf("Durations = %v, want %v ascending", s.Durations, want)
	}
	if s.MedianSecs != 1800 {
		t.Errorf("MedianSecs = %d, want 1800 (mean of the two middle sessions)", s.MedianSecs)
	}
	if s.LongestSecs != 6000 {
		t.Errorf("LongestSecs = %d, want 6000", s.LongestSecs)
	}
	if s.TotalSecs != 9100 {
		t.Errorf("TotalSecs = %d, want 9100", s.TotalSecs)
	}
	if s.SinglePlay != 1 {
		t.Errorf("SinglePlay = %d, want 1", s.SinglePlay)
	}
	if s.SingleArtistCount != 2 || s.SingleArtistShare != 0.5 {
		t.Errorf("single-artist = %d/%v, want 2/0.5", s.SingleArtistCount, s.SingleArtistShare)
	}
	if s.AvgPlays != 6.25 {
		t.Errorf("AvgPlays = %v, want 6.25", s.AvgPlays)
	}
}

func TestSessionsOpenersAndClosers(t *testing.T) {
	s := computeSessions(sessionRows())

	if len(s.TopOpeners) != 2 || s.TopOpeners[0].Name != "Toumani" || s.TopOpeners[0].Plays != 3 {
		t.Errorf("TopOpeners = %+v, want Toumani opening 3 sessions first", s.TopOpeners)
	}
	if s.TopOpeners[0].Rank != 1 {
		t.Errorf("TopOpeners[0].Rank = %d, want 1", s.TopOpeners[0].Rank)
	}
	if len(s.TopClosers) != 3 || s.TopClosers[0].Name != "AWV" || s.TopClosers[0].Plays != 2 {
		t.Errorf("TopClosers = %+v, want AWV closing 2 sessions", s.TopClosers)
	}
}

func TestSessionsClampsBackwardsTimestamps(t *testing.T) {
	s := computeSessions([]SessionRow{
		{ID: "1", StartedAt: 5000, EndedAt: 1000, Plays: 2, ListenedSecs: -50, DistinctArtists: 1},
	})

	if s.Durations[0] != 0 || s.LongestSecs != 0 {
		t.Errorf("durations = %v / longest %d, want 0 for an end before its start", s.Durations, s.LongestSecs)
	}
	if s.TotalSecs != 0 {
		t.Errorf("TotalSecs = %d, want 0 for negative listened seconds", s.TotalSecs)
	}
}

func TestSessionsEmptyIsZero(t *testing.T) {
	if got := computeSessions(nil); !reflect.DeepEqual(got, SessionStats{}) {
		t.Errorf("computeSessions(nil) = %+v, want zero SessionStats", got)
	}
}
