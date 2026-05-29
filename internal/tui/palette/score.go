// Package palette implements scoring and usage tracking for the command palette.
package palette

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

// Scoring constants. Higher = better.
const (
	scoreExactMatch     = 1000
	scorePrefixMatch    = 400
	scoreWordStartBonus = 60
	scoreConsecutive    = 30
	scoreCharMatch      = 10

	// Usage bonuses (added when a query is present).
	scoreUsageMaxBonus = 80 // cap on count-derived bonus
	scoreUsageRecent   = 40 // bonus if used in the last 24h

	maxUsageBonus = scoreUsageMaxBonus + scoreUsageRecent
)

// Match scores how well query matches name as a case-insensitive subsequence.
// Returns (score, true) on a match and (0, false) otherwise. An empty query
// always matches with score 0.
func Match(query, name string) (int, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, true
	}
	q := strings.ToLower(query)
	n := strings.ToLower(name)

	if q == n {
		return scoreExactMatch, true
	}
	if strings.HasPrefix(n, q) {
		// Prefix matches dominate — still report a positive score and ok.
		// Use rune count so multi-byte queries (e.g. "café") score the same
		// per character as ASCII-only ones. Cap below scoreExactMatch so an
		// arbitrarily long prefix never outranks a true exact match against a
		// different candidate.
		bonus := scoreCharMatch * len([]rune(q))
		if max := maxNonExactMatchScore() - scorePrefixMatch; bonus > max {
			bonus = max
		}
		return scorePrefixMatch + bonus, true
	}

	qRunes := []rune(q)
	score := 0
	qi := 0
	prevMatched := false
	prevRune := rune(' ')
	for _, r := range n {
		if qi >= len(qRunes) {
			prevMatched = false
			prevRune = r
			continue
		}
		qr := qRunes[qi]
		if r == qr {
			score += scoreCharMatch
			if isWordStart(prevRune) {
				score += scoreWordStartBonus
			}
			if prevMatched {
				score += scoreConsecutive
			}
			qi++
			prevMatched = true
		} else {
			prevMatched = false
		}
		prevRune = r
	}
	if qi < len(qRunes) {
		return 0, false
	}
	if max := maxNonExactMatchScore(); score > max {
		score = max
	}
	return score, true
}

func maxNonExactMatchScore() int {
	return scoreExactMatch - maxUsageBonus - 1
}

// isWordStart reports whether prev marks a boundary that makes the next rune
// a "word start" — useful for ranking matches against initials.
func isWordStart(prev rune) bool {
	return unicode.IsSpace(prev) || prev == '-' || prev == '_' || prev == '/' ||
		prev == '.' || prev == ':' || prev == '(' || prev == '[' || prev == ' '
}

// Rank sorts items by combined score: fuzzy match against name (when query is
// non-empty) plus a usage bonus, breaking ties by usage recency then name.
// With an empty query, items are ordered by usage (most-used first), then by
// name as a stable tie-break. Items whose fuzzy match fails are dropped from
// the result.
func Rank[T any](items []T, query string, id func(T) string, name func(T) string, usage *UsageStore) []T {
	type scored struct {
		item     T
		score    int
		count    int
		lastUsed time.Time
		name     string
	}

	q := strings.TrimSpace(query)
	out := make([]scored, 0, len(items))
	for _, it := range items {
		nm := name(it)
		var (
			matchScore int
			ok         bool
		)
		if q == "" {
			matchScore, ok = 0, true
		} else {
			matchScore, ok = Match(q, nm)
			if !ok {
				continue
			}
		}
		var count int
		var lastUsed time.Time
		if usage != nil {
			rec := usage.Get(id(it))
			count = rec.Count
			lastUsed = rec.LastUsed
		}
		total := matchScore + usageBonus(q, count, lastUsed)
		out = append(out, scored{
			item:     it,
			score:    total,
			count:    count,
			lastUsed: lastUsed,
			name:     nm,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if q == "" {
			// MRU+frequency for the empty-query case.
			if a.count != b.count {
				return a.count > b.count
			}
			if !a.lastUsed.Equal(b.lastUsed) {
				return a.lastUsed.After(b.lastUsed)
			}
			return strings.ToLower(a.name) < strings.ToLower(b.name)
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if a.count != b.count {
			return a.count > b.count
		}
		if !a.lastUsed.Equal(b.lastUsed) {
			return a.lastUsed.After(b.lastUsed)
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})

	result := make([]T, len(out))
	for i, s := range out {
		result[i] = s.item
	}
	return result
}

// usageBonus returns the score bonus contributed by past usage. When the
// query is empty, callers rely on (count, lastUsed) for ordering directly and
// no bonus is added.
func usageBonus(query string, count int, lastUsed time.Time) int {
	if query == "" || count <= 0 {
		return 0
	}
	bonus := scoreUsageMaxBonus
	if count < scoreUsageMaxBonus/8 {
		bonus = count * 8
	}
	if age := time.Since(lastUsed); !lastUsed.IsZero() && age >= 0 && age < 24*time.Hour {
		bonus += scoreUsageRecent
	}
	return bonus
}
