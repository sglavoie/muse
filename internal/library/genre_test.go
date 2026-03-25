package library

import (
	"reflect"
	"testing"
)

func TestSplitGenres(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "semicolon separator",
			raw:  "Rock; Jazz",
			want: []string{"Jazz", "Rock"},
		},
		{
			name: "slash separator",
			raw:  "Electronic/Ambient",
			want: []string{"Ambient", "Electronic"},
		},
		{
			name: "comma separator",
			raw:  "Pop, Folk",
			want: []string{"Folk", "Pop"},
		},
		{
			name: "deduplication",
			raw:  "Rock, Rock",
			want: []string{"Rock"},
		},
		{
			name: "empty string",
			raw:  "",
			want: nil,
		},
		{
			name: "only delimiters and whitespace",
			raw:  "  ;  /  , ",
			want: nil,
		},
		{
			name: "single genre",
			raw:  "Single Genre",
			want: []string{"Single Genre"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitGenres(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitGenres(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
