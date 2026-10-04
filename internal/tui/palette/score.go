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
		return prefixMatchScore(q), true
	}
	return subsequenceMatchScore(q, n)
}

// prefixMatchScore scores a prefix match. Prefix matches dominate — they always
// report a positive score. Rune count keeps multi-byte queries (e.g. "café")
// scoring the same per character as ASCII-only ones, and the cap keeps an
// arbitrarily long prefix from outranking a true exact match against a
// different candidate.
func prefixMatchScore(q string) int {
	bonus := scoreCharMatch * len([]rune(q))
	return scorePrefixMatch + min(bonus, maxNonExactMatchScore()-scorePrefixMatch)
}

// subsequenceMatchScore scores q as a case-insensitive subsequence of n,
// rewarding characters that land on a word start and runs of consecutive
// matches. Reports false when q is not a subsequence of n at all.
func subsequenceMatchScore(q, n string) (int, bool) {
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
		if r == qRunes[qi] {
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
	return min(score, maxNonExactMatchScore()), true
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
	return rank(items, query, id, name, nil, usage)
}

// rank optionally searches aliases. Alias-only hits form a second tier, while
// synonyms can improve an existing name hit without beating exact name hits.
func rank[T any](items []T, query string, id func(T) string, name func(T) string, aliases func(T) []string, usage *UsageStore) []T {
	q := strings.TrimSpace(query)
	out := make([]scoredItem[T], 0, len(items))
	for _, it := range items {
		if sc, ok := scoreItem(it, q, id, name, aliases, usage); ok {
			out = append(out, sc)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return lessScored(out[i], out[j], q == "") })

	result := make([]T, len(out))
	for i, s := range out {
		result[i] = s.item
	}
	return result
}

// scoredItem is one ranked candidate together with the tie-break inputs Rank
// orders on.
type scoredItem[T any] struct {
	item      T
	score     int
	count     int
	lastUsed  time.Time
	name      string
	aliasOnly bool
}

// scoreItem combines a candidate's fuzzy match score with its usage bonus.
// Reports false when a non-empty query matches neither the name nor an alias.
func scoreItem[T any](it T, q string, id func(T) string, name func(T) string, aliases func(T) []string, usage *UsageStore) (scoredItem[T], bool) {
	nm := name(it)
	matchScore, nameMatch := Match(q, nm)
	if q != "" && aliases != nil {
		aliasScore := bestAliasScore(q, aliases(it))
		if nameMatch {
			aliasScore = min(aliasScore, maxNonExactMatchScore())
		}
		matchScore = max(matchScore, aliasScore)
	}
	if !nameMatch && matchScore == 0 {
		return scoredItem[T]{}, false
	}
	var count int
	var lastUsed time.Time
	if usage != nil {
		rec := usage.Get(id(it))
		count = rec.Count
		lastUsed = rec.LastUsed
	}
	return scoredItem[T]{
		item:      it,
		score:     matchScore + usageBonus(q, count, lastUsed),
		count:     count,
		lastUsed:  lastUsed,
		name:      nm,
		aliasOnly: !nameMatch,
	}, true
}

func bestAliasScore(query string, aliases []string) int {
	best := 0
	for _, alias := range aliases {
		if score, ok := Match(query, alias); ok {
			best = max(best, score)
		}
	}
	return best
}

// lessScored orders two candidates by score, then by usage frequency, recency
// and finally name. An empty query skips the score comparison entirely so the
// ordering is pure MRU+frequency.
func lessScored[T any](a, b scoredItem[T], emptyQuery bool) bool {
	if a.aliasOnly != b.aliasOnly {
		return !a.aliasOnly
	}
	if !emptyQuery && a.score != b.score {
		return a.score > b.score
	}
	if a.count != b.count {
		return a.count > b.count
	}
	if !a.lastUsed.Equal(b.lastUsed) {
		return a.lastUsed.After(b.lastUsed)
	}
	return strings.ToLower(a.name) < strings.ToLower(b.name)
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
