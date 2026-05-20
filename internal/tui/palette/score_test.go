package palette

import (
	"testing"
	"time"
)

func TestMatchEmptyQueryMatches(t *testing.T) {
	score, ok := Match("", "anything")
	if !ok {
		t.Fatalf("empty query should match anything")
	}
	if score != 0 {
		t.Errorf("empty query score: want 0, got %d", score)
	}
}

func TestMatchExactBeatsPrefixBeatsSubsequence(t *testing.T) {
	exact, okE := Match("reveal in finder", "Reveal in Finder")
	prefix, okP := Match("reveal", "Reveal in Finder")
	subseq, okS := Match("rfn", "Reveal in Finder")
	if !okE || !okP || !okS {
		t.Fatalf("expected all three to match: %v %v %v", okE, okP, okS)
	}
	if !(exact > prefix && prefix > subseq) {
		t.Errorf("expected exact (%d) > prefix (%d) > subsequence (%d)", exact, prefix, subseq)
	}
}

func TestMatchSubsequenceMatches(t *testing.T) {
	score, ok := Match("rfn", "Reveal in Finder")
	if !ok {
		t.Fatalf("rfn should match 'Reveal in Finder' as subsequence")
	}
	if score <= 0 {
		t.Errorf("expected positive score, got %d", score)
	}
}

func TestMatchNonSubsequenceFails(t *testing.T) {
	if _, ok := Match("xyz", "Reveal in Finder"); ok {
		t.Errorf("xyz should not match 'Reveal in Finder'")
	}
}

func TestMatchCaseInsensitive(t *testing.T) {
	a, _ := Match("REVEAL", "Reveal in Finder")
	b, _ := Match("reveal", "Reveal in Finder")
	if a != b {
		t.Errorf("case-insensitive: got %d vs %d", a, b)
	}
}

func TestMatchWordStartBonus(t *testing.T) {
	// Subsequence matches that land on word starts should outscore equally
	// long subsequences that match mid-word.
	wordStart, _ := Match("rif", "Reveal in Finder") // all three are word-starts
	midWord, _ := Match("eel", "Reveal in Finder")   // mid-word run, no word-starts
	if wordStart <= midWord {
		t.Errorf("word-start match (%d) should outscore mid-word match (%d)", wordStart, midWord)
	}
}

func TestMatchMultiByteQueryMatches(t *testing.T) {
	// Queries containing multi-byte runes (e.g. accented characters or
	// non-Latin scripts) must be compared rune-by-rune, not byte-by-byte.
	// Previously, byte-indexing the query against a rune-ranged name caused
	// queries like "é" to never match "café".
	cases := []struct {
		query, name string
	}{
		{"é", "café"},
		{"caf", "café"},
		{"café", "café"},
		{"naïve", "naïve approach"},
		{"日", "日本語"},
	}
	for _, c := range cases {
		if _, ok := Match(c.query, c.name); !ok {
			t.Errorf("Match(%q, %q) should succeed", c.query, c.name)
		}
	}
}

func TestMatchPrefixScoreUsesRuneCount(t *testing.T) {
	// Prefix score should scale with the rune count of the query, not its
	// byte length. A 3-rune query "caf" and a 4-rune query "café" must
	// produce different scores against "café…" so that callers can rank
	// longer prefix matches above shorter ones consistently across ASCII
	// and multi-byte queries.
	shorter, okA := Match("caf", "café latte")
	longer, okB := Match("café", "café latte")
	if !okA || !okB {
		t.Fatalf("both queries should match: %v %v", okA, okB)
	}
	if longer <= shorter {
		t.Errorf("longer multi-byte prefix score (%d) should exceed shorter (%d)", longer, shorter)
	}
}

type stub struct{ id, name string }

func names(items []stub) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return out
}

func TestRankEmptyQueryAlphabeticalWhenNoUsage(t *testing.T) {
	items := []stub{
		{id: "z", name: "Zebra"},
		{id: "a", name: "Alpha"},
		{id: "m", name: "Middle"},
	}
	out := Rank(items, "", func(s stub) string { return s.id }, func(s stub) string { return s.name }, nil)
	got := names(out)
	want := []string{"Alpha", "Middle", "Zebra"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("position %d: want %q, got %q (full: %v)", i, w, got[i], got)
		}
	}
}

func TestRankEmptyQueryMRUFirstWithUsage(t *testing.T) {
	items := []stub{
		{id: "a", name: "Alpha"},
		{id: "m", name: "Middle"},
		{id: "z", name: "Zebra"},
	}
	usage := NewUsageStore("")
	usage.Bump("m")
	usage.Bump("m")
	usage.Bump("z")

	out := Rank(items, "", func(s stub) string { return s.id }, func(s stub) string { return s.name }, usage)
	got := names(out)
	if got[0] != "Middle" {
		t.Errorf("expected most-used first; got %v", got)
	}
	if got[1] != "Zebra" {
		t.Errorf("expected second-most-used second; got %v", got)
	}
	if got[2] != "Alpha" {
		t.Errorf("expected unused last; got %v", got)
	}
}

func TestRankSubsequenceMatchRfn(t *testing.T) {
	items := []stub{
		{id: "rev", name: "Reveal in Finder"},
		{id: "copy", name: "Copy track info"},
		{id: "ren", name: "Remove queue duplicates"},
	}
	out := Rank(items, "rfn", func(s stub) string { return s.id }, func(s stub) string { return s.name }, nil)
	if len(out) == 0 {
		t.Fatalf("expected at least one match")
	}
	if out[0].name != "Reveal in Finder" {
		t.Errorf("expected 'Reveal in Finder' first; got %v", names(out))
	}
}

func TestRankDropsNonMatches(t *testing.T) {
	items := []stub{
		{id: "a", name: "Alpha"},
		{id: "b", name: "Beta"},
	}
	out := Rank(items, "xyz", func(s stub) string { return s.id }, func(s stub) string { return s.name }, nil)
	if len(out) != 0 {
		t.Errorf("expected empty result for non-matching query; got %v", names(out))
	}
}

func TestRankUsageBonusBreaksTies(t *testing.T) {
	// Two commands have equivalent fuzzy quality for "foo"; usage should push
	// the more-used one higher.
	items := []stub{
		{id: "a", name: "Food"},
		{id: "b", name: "Food"}, // duplicate name on purpose
	}
	usage := NewUsageStore("")
	usage.Bump("b")
	out := Rank(items, "foo", func(s stub) string { return s.id }, func(s stub) string { return s.name }, usage)
	if len(out) != 2 {
		t.Fatalf("want 2 results, got %d", len(out))
	}
	if out[0].id != "b" {
		t.Errorf("expected used item first; got %s then %s", out[0].id, out[1].id)
	}
}

func TestRankPreservesStableTieBreakByName(t *testing.T) {
	items := []stub{
		{id: "c", name: "Charlie"},
		{id: "a", name: "Alpha"},
		{id: "b", name: "Bravo"},
	}
	out := Rank(items, "", func(s stub) string { return s.id }, func(s stub) string { return s.name }, nil)
	got := names(out)
	if got[0] != "Alpha" || got[1] != "Bravo" || got[2] != "Charlie" {
		t.Errorf("expected alphabetical tie-break; got %v", got)
	}
}

func TestUsageBonusZeroWhenEmptyQuery(t *testing.T) {
	if b := usageBonus("", 10, time.Now()); b != 0 {
		t.Errorf("usageBonus with empty query should be 0; got %d", b)
	}
}

func TestUsageBonusRecentAdded(t *testing.T) {
	old := usageBonus("foo", 1, time.Now().Add(-48*time.Hour))
	recent := usageBonus("foo", 1, time.Now())
	if recent <= old {
		t.Errorf("recent (%d) should exceed old (%d)", recent, old)
	}
}
