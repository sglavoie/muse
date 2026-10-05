package queue

import (
	"slices"
	"testing"
)

func TestGenreFilterPreservesQueueIdentityAndOrder(t *testing.T) {
	v := makeQueue(100, []Entry{
		{ID: 1, Hash: "same", Artist: "Miles", Title: "Live", Genre: "Jazz"},
		{ID: 2, Hash: "other", Artist: "Miles", Title: "Live", Genre: "Rock"},
		{ID: 3, Hash: "same", Artist: "Miles", Title: "Live", Genre: "Jazz"},
		{ID: 4, Hash: "blank", Title: "Jazz", Genre: ""},
	})
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"g:jAzZ", []int{0, 2}},
		{"live genre:Jazz a:Miles", []int{0, 2}},
		{"g:Jazz a:Other", nil},
		{"g:missing", nil},
		{"g:", []int{0, 1, 2, 3}},
	} {
		v.Filter.Text = tc.query
		if got := entryOrigIdxs(v.FilteredEntries()); !slices.Equal(got, tc.want) {
			t.Fatalf("%q: indices %v, want %v", tc.query, got, tc.want)
		}
	}
	v.Filter.Text = "g:Jazz"
	v.Traversal = shuffledTimeline(1, 2, 1, 0, 3)
	if got := entryOrigIdxs(v.FilteredEntries()); !slices.Equal(got, []int{2, 0}) {
		t.Fatalf("shuffled genre filter reordered or collapsed duplicates: %v", got)
	}
	v.TagFilterHashes = map[string]struct{}{"other": {}}
	if len(v.FilteredEntries()) != 0 {
		t.Fatal("genre and tag filters should intersect")
	}
}
