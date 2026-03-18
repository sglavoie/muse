package library

import "testing"

func TestParseFilterQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  FilterQuery
	}{
		{
			name:  "PlainText",
			input: "rock music",
			want:  FilterQuery{General: "rock music"},
		},
		{
			name:  "ShortArtistPrefix",
			input: "a:beatles",
			want:  FilterQuery{Artist: "beatles"},
		},
		{
			name:  "LongArtistPrefix",
			input: "artist:beatles",
			want:  FilterQuery{Artist: "beatles"},
		},
		{
			name:  "ShortTitlePrefix",
			input: "t:yesterday",
			want:  FilterQuery{Title: "yesterday"},
		},
		{
			name:  "LongTitlePrefix",
			input: "title:yesterday",
			want:  FilterQuery{Title: "yesterday"},
		},
		{
			name:  "ShortAlbumPrefix",
			input: "l:abbey road",
			want:  FilterQuery{Album: "abbey road"},
		},
		{
			name:  "LongAlbumPrefix",
			input: "album:abbey road",
			want:  FilterQuery{Album: "abbey road"},
		},
		{
			name:  "MultiplePrefixes",
			input: "a:beatles t:yesterday",
			want:  FilterQuery{Artist: "beatles", Title: "yesterday"},
		},
		{
			name:  "GeneralPlusPrefix",
			input: "rock a:beatles",
			want:  FilterQuery{General: "rock", Artist: "beatles"},
		},
		{
			name:  "AllPrefixes",
			input: "a:beatles t:yesterday l:help",
			want:  FilterQuery{Artist: "beatles", Title: "yesterday", Album: "help"},
		},
		{
			name:  "EmptyPrefix",
			input: "a:",
			want:  FilterQuery{Artist: ""},
		},
		{
			name:  "DuplicatePrefixLastWins",
			input: "a:foo a:bar",
			want:  FilterQuery{Artist: "bar"},
		},
		{
			name:  "MidWordColonStaysInGeneral",
			input: "foo:bar",
			want:  FilterQuery{General: "foo:bar"},
		},
		{
			name:  "GeneralAndMultiplePrefixes",
			input: "hey a:beatles l:abbey",
			want:  FilterQuery{General: "hey", Artist: "beatles", Album: "abbey"},
		},
		{
			name:  "EmptyInput",
			input: "",
			want:  FilterQuery{},
		},
		{
			name:  "WhitespaceOnly",
			input: "   ",
			want:  FilterQuery{},
		},
		{
			name:  "PrefixWithExtraSpaces",
			input: "  a:  beatles  ",
			want:  FilterQuery{Artist: "beatles"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFilterQuery(tt.input)
			if got != tt.want {
				t.Errorf("ParseFilterQuery(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFilterQuery_IsEmpty(t *testing.T) {
	if !(FilterQuery{}).IsEmpty() {
		t.Error("zero FilterQuery should be empty")
	}
	if (FilterQuery{General: "x"}).IsEmpty() {
		t.Error("FilterQuery with General should not be empty")
	}
	if (FilterQuery{Artist: "x"}).IsEmpty() {
		t.Error("FilterQuery with Artist should not be empty")
	}
}

func TestFilterQuery_IsFieldSpecific(t *testing.T) {
	if (FilterQuery{General: "x"}).IsFieldSpecific() {
		t.Error("General-only query should not be field-specific")
	}
	if !(FilterQuery{Artist: "x"}).IsFieldSpecific() {
		t.Error("Artist query should be field-specific")
	}
	if !(FilterQuery{Title: "x"}).IsFieldSpecific() {
		t.Error("Title query should be field-specific")
	}
	if !(FilterQuery{Album: "x"}).IsFieldSpecific() {
		t.Error("Album query should be field-specific")
	}
}
