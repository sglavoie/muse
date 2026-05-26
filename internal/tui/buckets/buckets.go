// Package buckets provides shared bucket-label helpers used by long-list
// renderers (Library, Queue) to chunk rows by sort key. Two label families
// are supported: first-letter alphabetical buckets and coarse date buckets.
package buckets

import (
	"strings"
	"time"
	"unicode"
)

// Date bucket labels emitted by DateBucket.
const (
	DateBucketToday     = "Today"
	DateBucketThisWeek  = "This Week"
	DateBucketThisMonth = "This Month"
	DateBucketOlder     = "Older"
	DateBucketUnknown   = "—"
)

// FirstLetter returns an uppercase first-letter bucket label for s. Leading
// whitespace and punctuation are skipped, and the leading article "The " is
// skipped so that "The Beatles" buckets under "B". Digits bucket under "#".
// Returns "—" for empty/whitespace strings.
func FirstLetter(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	runes := []rune(s)
	start := firstAlphaNumIndex(runes)
	if start < 0 {
		return "#"
	}
	runes = runes[start:]
	if len(runes) >= 4 && unicode.IsSpace(runes[3]) && strings.EqualFold(string(runes[:3]), "the") {
		s = strings.TrimSpace(string(runes[4:]))
		if s == "" {
			return "T"
		}
		runes = []rune(s)
		start = firstAlphaNumIndex(runes)
		if start < 0 {
			return "T"
		}
		runes = runes[start:]
	}
	r := runes[0]
	switch {
	case unicode.IsLetter(r):
		return string(unicode.ToUpper(r))
	case unicode.IsDigit(r):
		return "#"
	default:
		return "#"
	}
}

func firstAlphaNumIndex(runes []rune) int {
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return i
		}
	}
	return -1
}

// DateBucket returns a coarse date-range label for a Unix timestamp relative
// to now. Today covers the current calendar day. This Week covers the last
// seven calendar days exclusive of today. This Month covers the last thirty
// calendar days exclusive of today and this week. Anything older buckets as
// Older. Zero or negative timestamps bucket as "—".
func DateBucket(ts int64, now time.Time) string {
	if ts <= 0 {
		return DateBucketUnknown
	}
	t := time.Unix(ts, 0).In(now.Location())
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfTomorrow := startOfToday.AddDate(0, 0, 1)
	if !t.Before(startOfToday) && t.Before(startOfTomorrow) {
		return DateBucketToday
	}
	if !t.Before(startOfTomorrow) {
		return DateBucketUnknown
	}
	weekAgo := startOfToday.AddDate(0, 0, -7)
	if !t.Before(weekAgo) {
		return DateBucketThisWeek
	}
	monthAgo := startOfToday.AddDate(0, 0, -30)
	if !t.Before(monthAgo) {
		return DateBucketThisMonth
	}
	return DateBucketOlder
}
