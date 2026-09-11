package library

import (
	"sort"
	"strings"
)

// genreDelimiters folds the alternate delimiters onto ";" for uniform splitting.
// It is built once because a Replacer is immutable and safe for concurrent use,
// and because SplitGenres runs per track: the DJ planner calls it for every
// candidate on every batch, where building the replacer was the second-largest
// allocation source in BenchmarkPartition.
var genreDelimiters = strings.NewReplacer("/", ";", ",", ";")

// SplitGenres splits a raw genre string into individual genre names.
// It splits on ";", "/", and "," as delimiters, trims whitespace from each
// part, deduplicates, and returns a sorted slice of unique non-empty genres.
// Returns nil when raw is empty or contains only delimiters/whitespace.
func SplitGenres(raw string) []string {
	if raw == "" {
		return nil
	}

	s := genreDelimiters.Replace(raw)
	parts := strings.Split(s, ";")

	seen := make(map[string]struct{}, len(parts))
	var genres []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			genres = append(genres, p)
		}
	}

	if len(genres) == 0 {
		return nil
	}

	sort.Strings(genres)
	return genres
}
