package library

import (
	"sort"
	"strings"
)

// SplitGenres splits a raw genre string into individual genre names.
// It splits on ";", "/", and "," as delimiters, trims whitespace from each
// part, deduplicates, and returns a sorted slice of unique non-empty genres.
// Returns nil when raw is empty or contains only delimiters/whitespace.
func SplitGenres(raw string) []string {
	if raw == "" {
		return nil
	}

	// Replace all delimiters with ";" for uniform splitting.
	s := strings.NewReplacer("/", ";", ",", ";").Replace(raw)
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
