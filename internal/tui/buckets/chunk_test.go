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

// TestChunkBucketHeaderSuppressesDivider guards a visual bug where a row
// that triggers both a new bucket header and the periodic divider
// rendered as: "[B]" + "─" + row, putting a redundant divider between
// the header and its first row. The bucket header itself separates the
// previous group from the new one, so the divider must be suppressed for
// that row (and the divider counter reset so the next divider fires
// DividerEvery rows after the header, not immediately).
func TestChunkBucketHeaderSuppressesDivider(t *testing.T) {
	rows := makeRows(15)
	labels := make([]string, 15)
	for i := range labels {
		if i < 10 {
			labels[i] = "A"
		} else {
			labels[i] = "B"
		}
	}
	out := Chunk(rows, labels, ChunkOptions{
		MaxRows:          100,
		DividerEvery:     10,
		MakeDivider:      func() string { return "---" },
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	// Find the [B] header position and verify the line immediately after
	// is a data row, not a divider.
	headerIdx := -1
	for i, line := range out {
		if line == "[B]" {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		t.Fatalf("expected [B] header in output: %v", out)
	}
	if headerIdx+1 >= len(out) {
		t.Fatalf("no line after [B] header: %v", out)
	}
	if out[headerIdx+1] == "---" {
		t.Errorf("divider must not be emitted between bucket header and first row of new bucket: %v", out)
	}
	// The divider counter should reset on bucket header, so no divider
	// should appear within the first DividerEvery-1 rows of bucket B.
	for i := headerIdx + 1; i < headerIdx+10 && i < len(out); i++ {
		if out[i] == "---" {
			t.Errorf("divider at index %d should have been suppressed after bucket header reset: %v", i, out)
		}
	}
}

// TestChunkBucketHeaderResetsDividerCounter verifies that after a bucket
// header reset, the next divider fires DividerEvery rows into the new
// bucket (not immediately on the next row from the prior offset).
func TestChunkBucketHeaderResetsDividerCounter(t *testing.T) {
	// 17 rows: first 5 in bucket A, next 12 in bucket B. Divider every 10
	// rows. Without the reset, dataSinceDivider would still be 5 at the
	// [B] header and a divider would fire 5 rows into bucket B. With the
	// reset, the divider should fire 10 rows into bucket B instead.
	rows := makeRows(17)
	labels := make([]string, 17)
	for i := range labels {
		if i < 5 {
			labels[i] = "A"
		} else {
			labels[i] = "B"
		}
	}
	out := Chunk(rows, labels, ChunkOptions{
		MaxRows:          100,
		DividerEvery:     10,
		MakeDivider:      func() string { return "---" },
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	// Locate the [B] header and the divider; check the divider sits at
	// least 10 lines after the header (header + 10 data rows minimum).
	headerIdx, dividerIdx := -1, -1
	for i, line := range out {
		switch line {
		case "[B]":
			headerIdx = i
		case "---":
			dividerIdx = i
		}
	}
	if headerIdx < 0 || dividerIdx < 0 {
		t.Fatalf("expected one [B] header and one divider, got out=%v", out)
	}
	// Header is followed by data rows; the divider should be exactly 10
	// data rows after the header (so dividerIdx - headerIdx == 11).
	if dividerIdx-headerIdx != 11 {
		t.Errorf("divider should fire 10 rows after [B] header (idx diff = 11), got diff = %d in %v",
			dividerIdx-headerIdx, out)
	}
}

func TestChunkMismatchedLabelsFallback(t *testing.T) {
	rows := makeRows(3)
	out := Chunk(rows, []string{"A"}, ChunkOptions{MaxRows: 10})
	if len(out) != 3 {
		t.Errorf("expected fallback to data-only rows, got %v", out)
	}
}

// TestChunkMismatchedLabelsFallbackRespectsMaxRows verifies that the
// defensive mismatch fallback honours MaxRows.
func TestChunkMismatchedLabelsFallbackRespectsMaxRows(t *testing.T) {
	rows := makeRows(20)
	out := Chunk(rows, []string{"A"}, ChunkOptions{MaxRows: 5})
	if len(out) != 5 {
		t.Errorf("mismatched fallback: want 5 rows (MaxRows), got %d", len(out))
	}
}

// TestChunkStickyBucketMaxRows1 verifies that when MaxRows=1 and there are
// data rows, the sticky header is suppressed so the single allowed output
// line is used for a data row rather than the header.
func TestChunkStickyBucketMaxRows1(t *testing.T) {
	rows := makeRows(3)
	labels := []string{"A", "A", "A"}
	out := Chunk(rows, labels, ChunkOptions{
		StickyBucket:     "A",
		MaxRows:          1,
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	if len(out) != 1 {
		t.Fatalf("MaxRows=1: want 1 output line, got %d: %v", len(out), out)
	}
	if out[0] == "[A]" {
		t.Errorf("MaxRows=1: sticky header should be suppressed when it would leave no room for data, got %v", out)
	}
}

// TestChunkStickyBucketMaxRows2 verifies that when MaxRows=2, the sticky
// header is emitted followed by exactly one data row.
func TestChunkStickyBucketMaxRows2(t *testing.T) {
	rows := makeRows(5)
	labels := []string{"A", "A", "A", "A", "A"}
	out := Chunk(rows, labels, ChunkOptions{
		StickyBucket:     "A",
		MaxRows:          2,
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	if len(out) != 2 {
		t.Fatalf("MaxRows=2: want 2 output lines, got %d: %v", len(out), out)
	}
	if out[0] != "[A]" {
		t.Errorf("MaxRows=2: first line should be the sticky header, got %q", out[0])
	}
	if out[1] != "row" {
		t.Errorf("MaxRows=2: second line should be a data row, got %q", out[1])
	}
}

func TestChunkUncappedStickyBucketWithNoRows(t *testing.T) {
	out := Chunk(nil, nil, ChunkOptions{
		StickyBucket:     "A",
		MaxRows:          0,
		MakeBucketHeader: func(s string) string { return "[" + s + "]" },
	})
	want := []string{"[A]"}
	if strings.Join(out, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", out, want)
	}
}
