package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"testing"
)

// TestBuildTimeGenreBonus_TopTwo verifies that the top genre for the given hour
// receives +3 and the runner-up receives +1.
func TestBuildTimeGenreBonus_TopTwo(t *testing.T) {
	affinities := []db.HourGenreAffinity{
		{Hour: 10, Genre: "Rock", Count: 8},
		{Hour: 10, Genre: "Jazz", Count: 5},
		{Hour: 10, Genre: "Pop", Count: 3},
	}
	bonus := buildTimeGenreBonus(affinities, 10)
	if bonus["Rock"] != 3 {
		t.Errorf("Rock bonus = %v, want 3", bonus["Rock"])
	}
	if bonus["Jazz"] != 1 {
		t.Errorf("Jazz bonus = %v, want 1", bonus["Jazz"])
	}
	if _, ok := bonus["Pop"]; ok {
		t.Errorf("Pop should not have a bonus, got %v", bonus["Pop"])
	}
}

// TestBuildTimeGenreBonus_OtherHoursIgnored verifies that genres from other
// hours do not appear in the result.
func TestBuildTimeGenreBonus_OtherHoursIgnored(t *testing.T) {
	affinities := []db.HourGenreAffinity{
		{Hour: 8, Genre: "Classical", Count: 10},
		{Hour: 10, Genre: "Rock", Count: 6},
		{Hour: 10, Genre: "Jazz", Count: 4},
	}
	bonus := buildTimeGenreBonus(affinities, 10)
	if _, ok := bonus["Classical"]; ok {
		t.Errorf("Classical (hour 8) should not appear in bonus for hour 10")
	}
	if bonus["Rock"] != 3 {
		t.Errorf("Rock bonus = %v, want 3", bonus["Rock"])
	}
}

// TestBuildTimeGenreBonus_Empty verifies that an empty affinity slice returns
// an empty (non-nil) map.
func TestBuildTimeGenreBonus_Empty(t *testing.T) {
	bonus := buildTimeGenreBonus(nil, 14)
	if len(bonus) != 0 {
		t.Errorf("expected empty map for nil affinities, got %v", bonus)
	}
	bonus2 := buildTimeGenreBonus([]db.HourGenreAffinity{}, 14)
	if len(bonus2) != 0 {
		t.Errorf("expected empty map for empty affinities, got %v", bonus2)
	}
}
