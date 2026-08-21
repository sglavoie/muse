package insights

import (
	"testing"
	"time"

	// tzdata is embedded so LoadLocation works on hosts without a system
	// zoneinfo database; the DST tests are meaningless without it.
	_ "time/tzdata"
)

// testLoc is a zone with DST transitions, which UTC cannot exercise.
func testLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

// at parses a local wall-clock time in loc and returns its Unix seconds.
func at(t *testing.T, loc *time.Location, wall string) int64 {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", wall, loc)
	if err != nil {
		t.Fatalf("ParseInLocation(%q): %v", wall, err)
	}
	return ts.Unix()
}

// play builds a row the way the DB reader does: the caller states when the
// play *ended* and how long it ran, and the start is derived.
func play(endedAt int64, listenedSecs int, artist string) PlayRow {
	return PlayRow{
		Hash:         artist + "-hash",
		StartAt:      endedAt - int64(listenedSecs),
		ListenedSecs: listenedSecs,
		Artist:       artist,
	}
}
