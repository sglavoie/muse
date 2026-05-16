package buckets

import (
	"strings"
	"testing"
)

func makeRows(n int) []string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = "row"
	}
	return rows
}

func TestChunkDividerEvery10(t *testing.T) {
	rows := makeRows(25)
	labels := make([]string, 25)
	out := Chunk(rows, labels, ChunkOptions{
		MaxRows:      100,
		DividerEvery: 10,
		MakeDivider:  func() string { return "---" },
	})
	gotDividers := 0
	for _, line := range out {
		if line == "---" {
			gotDividers++
		}
	}
	if gotDividers != 2 {
		t.Errorf("expected 2 dividers in 25-row output, got %d (lines: %d)", gotDividers, len(out))
	}
}

func TestChunkInlineBucketHeaders(t *testing.T) {
	labels := []string{"A", "A", "A", "B", "B", "C"}
	rows := makeRows(6)
	out := Chunk(rows, labels, ChunkOptions{
		MaxRows:          100,
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	want := []string{"[A]", "row", "row", "row", "[B]", "row", "row", "[C]", "row"}
	if strings.Join(out, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestChunkStickyBucketSuppressesDuplicate(t *testing.T) {
	labels := []string{"A", "A", "B"}
	rows := makeRows(3)
	out := Chunk(rows, labels, ChunkOptions{
		StickyBucket:     "A",
		MaxRows:          100,
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	want := []string{"[A]", "row", "row", "[B]", "row"}
	if strings.Join(out, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestChunkMaxRowsCap(t *testing.T) {
	rows := makeRows(50)
	labels := make([]string, 50)
	out := Chunk(rows, labels, ChunkOptions{
		MaxRows:      10,
		DividerEvery: 10,
		MakeDivider:  func() string { return "---" },
	})
	if len(out) > 10 {
		t.Errorf("expected at most 10 lines, got %d", len(out))
	}
}

func TestChunkMismatchedLabelsFallback(t *testing.T) {
	rows := makeRows(3)
	out := Chunk(rows, []string{"A"}, ChunkOptions{MaxRows: 10})
	if len(out) != 3 {
		t.Errorf("expected fallback to data-only rows, got %v", out)
	}
}
