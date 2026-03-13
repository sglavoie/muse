package library

import (
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
)

func TestSortTracks(t *testing.T) {
	tests := []struct {
		name   string
		input  []musiclib.Track
		wantOrder []string // expected Artist+"|"+Title sequence
	}{
		{
			name:      "empty slice",
			input:     []musiclib.Track{},
			wantOrder: []string{},
		},
		{
			name: "already sorted",
			input: []musiclib.Track{
				{Artist: "Alpha", Title: "One"},
				{Artist: "Beta", Title: "Two"},
			},
			wantOrder: []string{"Alpha|One", "Beta|Two"},
		},
		{
			name: "reverse order sorted correctly",
			input: []musiclib.Track{
				{Artist: "Zeta", Title: "Song"},
				{Artist: "Alpha", Title: "Song"},
			},
			wantOrder: []string{"Alpha|Song", "Zeta|Song"},
		},
		{
			name: "unknown artist goes to bottom",
			input: []musiclib.Track{
				{Artist: "", Title: "Mystery"},
				{Artist: "Alpha", Title: "Song"},
			},
			wantOrder: []string{"Alpha|Song", "|Mystery"},
		},
		{
			name: "multiple unknown artists sorted at bottom by title",
			input: []musiclib.Track{
				{Artist: "", Title: "Zebra"},
				{Artist: "Alpha", Title: "Song"},
				{Artist: "", Title: "Apple"},
			},
			wantOrder: []string{"Alpha|Song", "|Apple", "|Zebra"},
		},
		{
			name: "ties on artist sorted by title",
			input: []musiclib.Track{
				{Artist: "Alpha", Title: "C Song"},
				{Artist: "Alpha", Title: "A Song"},
				{Artist: "Alpha", Title: "B Song"},
			},
			wantOrder: []string{"Alpha|A Song", "Alpha|B Song", "Alpha|C Song"},
		},
		{
			name: "case-insensitive artist sort",
			input: []musiclib.Track{
				{Artist: "beta", Title: "Song"},
				{Artist: "Alpha", Title: "Song"},
				{Artist: "GAMMA", Title: "Song"},
			},
			wantOrder: []string{"Alpha|Song", "beta|Song", "GAMMA|Song"},
		},
		{
			name: "case-insensitive title sort",
			input: []musiclib.Track{
				{Artist: "Alpha", Title: "b Track"},
				{Artist: "Alpha", Title: "A Track"},
			},
			wantOrder: []string{"Alpha|A Track", "Alpha|b Track"},
		},
		{
			name: "known artists before all unknown",
			input: []musiclib.Track{
				{Artist: "", Title: "No Name 1"},
				{Artist: "Zeta", Title: "Last Known"},
				{Artist: "", Title: "No Name 2"},
				{Artist: "Alpha", Title: "First Known"},
			},
			wantOrder: []string{
				"Alpha|First Known",
				"Zeta|Last Known",
				"|No Name 1",
				"|No Name 2",
			},
		},
		{
			name: "original slice is not mutated",
			input: []musiclib.Track{
				{Artist: "Zeta", Title: "Song"},
				{Artist: "Alpha", Title: "Song"},
			},
			wantOrder: []string{"Alpha|Song", "Zeta|Song"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keep a copy to verify original is not mutated.
			original := make([]musiclib.Track, len(tt.input))
			copy(original, tt.input)

			got := SortTracks(tt.input)

			// Verify length.
			if len(got) != len(tt.wantOrder) {
				t.Fatalf("SortTracks() len = %d, want %d", len(got), len(tt.wantOrder))
			}

			// Verify order.
			for i, want := range tt.wantOrder {
				key := got[i].Artist + "|" + got[i].Title
				if key != want {
					t.Errorf("SortTracks()[%d] = %q, want %q", i, key, want)
				}
			}

			// Verify original slice is not mutated.
			for i, orig := range original {
				if tt.input[i] != orig {
					t.Errorf("input slice mutated at index %d", i)
				}
			}
		})
	}
}
