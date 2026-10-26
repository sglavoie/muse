package sorting

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// trackLess is the per-pair comparator SortTracks used before it switched to
// precomputed keys, kept as the oracle for TestSortTracksMatchesComparator. A
// field it does not sort on reports false, leaving the original order intact.
func trackLess(a, b library.Track, field SortField, dir SortDir) bool {
	switch field {
	case SortArtist:
		return CompareStringsUnknownLast(a.Artist, b.Artist, dir)
	case SortTitle:
		return CompareStringsUnknownLast(a.Title, b.Title, dir)
	case SortAlbum:
		return CompareStringsUnknownLast(a.Album, b.Album, dir)
	case SortDuration:
		return CompareNumericZeroLast(a.Duration, b.Duration, dir)
	case SortDateAdded:
		return timestampLess(a.CreatedAt, b.CreatedAt, dir)
	case SortLastPlayed:
		return timestampLess(a.LastPlayedAt, b.LastPlayedAt, dir)
	default:
		return false
	}
}

func timestampLess(a, b int64, dir SortDir) bool {
	if dir == SortAsc {
		return a < b
	}
	return a > b
}

// TestSortTracksMatchesComparator verifies that the key-based SortTracks orders
// every field and direction exactly as a stable sort with the per-pair
// comparators did, including case and Unicode folding, invalid UTF-8, unknown
// sentinels and zero, negative and NaN durations.
func TestSortTracksMatchesComparator(t *testing.T) {
	texts := []string{
		"", "a", "A", "b", "B", "ab", "Ab", "aB", "abc", "Étude", "étude", "ÉTUDE",
		"Ωmega", "ωmega", "Zappa", "alt-J", "unknown artist", "Unknown Artist",
		"UNKNOWN ARTIST", "Unknown artiſt", "(Unknown Album)", "(unknown genre)",
		"(unknown artist)", "\xff", "\xfe", "a\xffb", "İstanbul", "istanbul", "ß", "SS",
	}
	durations := []float64{0, -1, 1, 1, 2.5, 180, math.NaN(), 3600}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // seeded source so the fixture is reproducible
	tracks := make([]library.Track, 600)
	for i := range tracks {
		tracks[i] = library.Track{
			Hash:         fmt.Sprint(i),
			Artist:       texts[rng.IntN(len(texts))],
			Title:        texts[rng.IntN(len(texts))],
			Album:        texts[rng.IntN(len(texts))],
			Duration:     durations[rng.IntN(len(durations))],
			CreatedAt:    rng.Int64N(5),
			LastPlayedAt: rng.Int64N(5),
		}
	}
	fields := []SortField{SortArtist, SortTitle, SortAlbum, SortDuration, SortDateAdded, SortLastPlayed, SortName, SortSource}
	for _, field := range fields {
		for _, dir := range []SortDir{SortAsc, SortDesc} {
			want := make([]library.Track, len(tracks))
			copy(want, tracks)
			sort.SliceStable(want, func(i, j int) bool { return trackLess(want[i], want[j], field, dir) })
			got := SortTracks(tracks, field, dir)
			for i := range want {
				if got[i].Hash != want[i].Hash {
					t.Fatalf("%s/%d: position %d: got track %s, want %s",
						SortFieldLabel(field), dir, i, got[i].Hash, want[i].Hash)
				}
			}
		}
	}
}
