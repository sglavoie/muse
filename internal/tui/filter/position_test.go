package filter

import "testing"

func TestRestorePositionAfterDeletionAndReordering(t *testing.T) {
	origin := CapturePosition([]string{"a", "b", "c"}, 2, 1)
	for _, tc := range []struct {
		name  string
		rows  []string
		focus string
		want  int
	}{
		{"focused row moved", []string{"c", "a", "b"}, "b", 2},
		{"no results", []string{"c", "a", "b"}, "", 0},
		{"original deleted", []string{"a", "b"}, "", 1},
		{"empty", nil, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cursor, scroll := origin.Restore(tc.rows, tc.focus)
			if cursor != tc.want || scroll > cursor || scroll < 0 {
				t.Fatalf("position = %d,%d, want cursor %d", cursor, scroll, tc.want)
			}
		})
	}
}
