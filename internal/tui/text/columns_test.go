package text

import "testing"

func TestTrackColumnsKeepIdentityAsWindowsGrow(t *testing.T) {
	for _, position := range []bool{false, true} {
		for _, detail := range [][2]int{{0, 0}, {0, 7}, {0, 12}, {10, 10}, {10, 16}} {
			previous := TrackColumns(59, position, detail[0], detail[1])
			for width := 60; width <= 300; width++ {
				c := TrackColumns(width, position, detail[0], detail[1])
				if c.Title < previous.Title || c.Artist < previous.Artist {
					t.Fatalf("identity shrank at %d: %+v -> %+v", width, previous, c)
				}
				if c.Title < 20 || c.Artist < 10 || (c.Album > 0 && c.Album < 8) || (c.Detail != detail[0] && c.Detail != detail[1]) {
					t.Fatalf("unreadable columns at %d: %+v", width, c)
				}
				total := 2 + c.Title + c.Artist + c.Album + c.Duration + c.Detail + 2
				if position {
					total += c.Position + 1
				}
				if c.Album > 0 {
					total++
				}
				if c.Detail > 0 {
					total++
				}
				if total != width {
					t.Fatalf("width %d: allocation %d (%+v)", width, total, c)
				}
				previous = c
			}
		}
	}
}
