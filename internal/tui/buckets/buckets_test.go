package buckets

import (
	"testing"
	"time"
)

func TestFirstLetter(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Beatles", "B"},
		{"beatles", "B"},
		{"The Beatles", "B"},
		{"the beatles", "B"},
		{"The ", "T"},
		{"  Pixies  ", "P"},
		{"1990s mix", "#"},
		{"---", "#"},
		{"", "—"},
		{"   ", "—"},
		{"Émilie", "É"},
	}
	for _, c := range cases {
		got := FirstLetter(c.in)
		if got != c.want {
			t.Errorf("FirstLetter(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDateBucket(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ts   int64
		want string
	}{
		{0, DateBucketUnknown},
		{-1, DateBucketUnknown},
		{now.Unix(), DateBucketToday},
		{time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC).Unix(), DateBucketToday},
		{time.Date(2026, 5, 15, 23, 59, 59, 0, time.UTC).Unix(), DateBucketThisWeek},
		{time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC).Unix(), DateBucketThisWeek},
		{time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC).Unix(), DateBucketThisWeek},
		{time.Date(2026, 5, 8, 23, 59, 59, 0, time.UTC).Unix(), DateBucketThisMonth},
		{time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC).Unix(), DateBucketThisMonth},
		{time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC).Unix(), DateBucketThisMonth},
		{time.Date(2026, 4, 15, 23, 59, 59, 0, time.UTC).Unix(), DateBucketOlder},
		{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), DateBucketOlder},
	}
	for _, c := range cases {
		got := DateBucket(c.ts, now)
		if got != c.want {
			t.Errorf("DateBucket(%d) = %q, want %q", c.ts, got, c.want)
		}
	}
}
