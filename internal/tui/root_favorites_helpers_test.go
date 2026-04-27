package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestLoadFavoriteSet verifies loadFavoriteSet returns the correct set for
// different favorite types.
func TestLoadFavoriteSet(t *testing.T) {
	t.Run("returns set for type", func(t *testing.T) {
		r := &stubFavoriteReader{favorites: map[string][]string{
			"track":  {"h1", "h2"},
			"artist": {"Bach"},
		}}
		got := loadFavoriteSet(r, "track")
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		if _, ok := got["h1"]; !ok {
			t.Error("expected h1 in set")
		}
		if _, ok := got["h2"]; !ok {
			t.Error("expected h2 in set")
		}
	})

	t.Run("nil reader returns nil", func(t *testing.T) {
		got := loadFavoriteSet(nil, "track")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty type returns empty set", func(t *testing.T) {
		r := &stubFavoriteReader{favorites: map[string][]string{
			"track": {"h1"},
		}}
		got := loadFavoriteSet(r, "artist")
		if len(got) != 0 {
			t.Fatalf("expected empty set, got %d entries", len(got))
		}
	})
}

// TestUpdateFavoriteCache verifies the cache update helper for single and bulk
// operations.
func TestUpdateFavoriteCache(t *testing.T) {
	t.Run("single add to nil cache", func(t *testing.T) {
		var cache map[string]struct{}
		msg := favoriteToggledMsg{value: "h1", added: true, count: 1}
		updateFavoriteCache(&cache, nil, "track", msg)
		if _, ok := cache["h1"]; !ok {
			t.Fatal("expected h1 in cache after add")
		}
	})

	t.Run("single remove from cache", func(t *testing.T) {
		cache := map[string]struct{}{"h1": {}, "h2": {}}
		msg := favoriteToggledMsg{value: "h1", added: false, count: 1}
		updateFavoriteCache(&cache, nil, "track", msg)
		if _, ok := cache["h1"]; ok {
			t.Fatal("expected h1 removed from cache")
		}
		if _, ok := cache["h2"]; !ok {
			t.Fatal("expected h2 still in cache")
		}
	})

	t.Run("bulk reloads from reader", func(t *testing.T) {
		cache := map[string]struct{}{"old": {}}
		r := &stubFavoriteReader{favorites: map[string][]string{
			"artist": {"Bach", "Davis"},
		}}
		msg := favoriteToggledMsg{count: 2, added: true}
		updateFavoriteCache(&cache, r, "artist", msg)
		if len(cache) != 2 {
			t.Fatalf("expected 2 entries after bulk reload, got %d", len(cache))
		}
		if _, ok := cache["Bach"]; !ok {
			t.Error("expected Bach in cache")
		}
	})

	t.Run("empty value is no-op for single", func(t *testing.T) {
		cache := map[string]struct{}{"h1": {}}
		msg := favoriteToggledMsg{value: "", added: true, count: 1}
		updateFavoriteCache(&cache, nil, "track", msg)
		if len(cache) != 1 {
			t.Fatalf("expected cache unchanged, got %d entries", len(cache))
		}
	})
}

// TestIndexOfString verifies indexOfString returns the correct raw position.
func TestIndexOfString(t *testing.T) {
	items := []string{"Alpha", "Beta", "Charlie", "Delta"}
	if got := indexOfString(items, "Charlie"); got != 2 {
		t.Errorf("indexOfString(Charlie): expected 2, got %d", got)
	}
	if got := indexOfString(items, "Alpha"); got != 0 {
		t.Errorf("indexOfString(Alpha): expected 0, got %d", got)
	}
	if got := indexOfString(items, "missing"); got != 0 {
		t.Errorf("indexOfString(missing): expected 0, got %d", got)
	}
}

// TestIndexOfTrackByHash verifies indexOfTrackByHash returns the correct raw position.
func TestIndexOfTrackByHash(t *testing.T) {
	tracks := []library.Track{
		{Hash: "h1"}, {Hash: "h2"}, {Hash: "h3"},
	}
	if got := indexOfTrackByHash(tracks, "h3"); got != 2 {
		t.Errorf("indexOfTrackByHash(h3): expected 2, got %d", got)
	}
	if got := indexOfTrackByHash(tracks, "h1"); got != 0 {
		t.Errorf("indexOfTrackByHash(h1): expected 0, got %d", got)
	}
	if got := indexOfTrackByHash(tracks, "missing"); got != 0 {
		t.Errorf("indexOfTrackByHash(missing): expected 0, got %d", got)
	}
}

// TestInsertStringAt verifies insertStringAt for various insertion positions.
func TestInsertStringAt(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		idx   int
		s     string
		want  []string
	}{
		{"insert at start", []string{"b", "c"}, 0, "a", []string{"a", "b", "c"}},
		{"insert in middle", []string{"a", "c"}, 1, "b", []string{"a", "b", "c"}},
		{"insert at end", []string{"a", "b"}, 2, "c", []string{"a", "b", "c"}},
		{"clamp negative idx", []string{"a", "b"}, -1, "z", []string{"z", "a", "b"}},
		{"clamp idx beyond len", []string{"a", "b"}, 10, "z", []string{"a", "b", "z"}},
		{"empty slice", nil, 0, "x", []string{"x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertStringAt(tc.items, tc.idx, tc.s)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestInsertTrackAt verifies insertTrackAt for various insertion positions.
func TestInsertTrackAt(t *testing.T) {
	a := library.Track{Hash: "aaa", Title: "A"}
	b := library.Track{Hash: "bbb", Title: "B"}
	c := library.Track{Hash: "ccc", Title: "C"}

	tests := []struct {
		name   string
		tracks []library.Track
		idx    int
		t      library.Track
		want   []library.Track
	}{
		{"insert at start", []library.Track{b, c}, 0, a, []library.Track{a, b, c}},
		{"insert in middle", []library.Track{a, c}, 1, b, []library.Track{a, b, c}},
		{"insert at end", []library.Track{a, b}, 2, c, []library.Track{a, b, c}},
		{"clamp negative idx", []library.Track{a, b}, -1, c, []library.Track{c, a, b}},
		{"clamp idx beyond len", []library.Track{a, b}, 10, c, []library.Track{a, b, c}},
		{"empty slice", nil, 0, a, []library.Track{a}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertTrackAt(tc.tracks, tc.idx, tc.t)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i].Hash != tc.want[i].Hash {
					t.Fatalf("index %d: got hash %q, want %q", i, got[i].Hash, tc.want[i].Hash)
				}
			}
		})
	}
}

// TestSortBulkRemovalByIndex verifies that sortBulkRemovalByIndex sorts
// parallel arrays by the index values in ascending order.
func TestSortBulkRemovalByIndex(t *testing.T) {
	t.Run("sorts values and indices", func(t *testing.T) {
		values := []string{"c", "a", "b"}
		indices := []int{5, 1, 3}
		sortBulkRemovalByIndex(values, nil, indices)
		wantValues := []string{"a", "b", "c"}
		wantIndices := []int{1, 3, 5}
		for i := range values {
			if values[i] != wantValues[i] {
				t.Errorf("values[%d] = %q, want %q", i, values[i], wantValues[i])
			}
			if indices[i] != wantIndices[i] {
				t.Errorf("indices[%d] = %d, want %d", i, indices[i], wantIndices[i])
			}
		}
	})

	t.Run("sorts tracks in parallel", func(t *testing.T) {
		values := []string{"h3", "h1", "h2"}
		tracks := []library.Track{
			{Hash: "h3", Title: "C"},
			{Hash: "h1", Title: "A"},
			{Hash: "h2", Title: "B"},
		}
		indices := []int{5, 1, 3}
		sortBulkRemovalByIndex(values, tracks, indices)
		wantHashes := []string{"h1", "h2", "h3"}
		for i := range tracks {
			if tracks[i].Hash != wantHashes[i] {
				t.Errorf("tracks[%d].Hash = %q, want %q", i, tracks[i].Hash, wantHashes[i])
			}
		}
	})

	t.Run("single element is a no-op", func(t *testing.T) {
		values := []string{"a"}
		indices := []int{5}
		sortBulkRemovalByIndex(values, nil, indices)
		if values[0] != "a" || indices[0] != 5 {
			t.Errorf("unexpected mutation of single-element slices")
		}
	})

	t.Run("already sorted", func(t *testing.T) {
		values := []string{"a", "b", "c"}
		indices := []int{1, 3, 5}
		sortBulkRemovalByIndex(values, nil, indices)
		for i, want := range []string{"a", "b", "c"} {
			if values[i] != want {
				t.Errorf("values[%d] = %q, want %q", i, values[i], want)
			}
		}
	})
}
