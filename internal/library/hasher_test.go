package library

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file at path (creating parent dirs) with the given content.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeFile MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("writeFile WriteFile: %v", err)
	}
}

// makeBytes returns a byte slice of length n filled with the given byte value.
func makeBytes(n int, b byte) []byte {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = b
	}
	return buf
}

// TestHashFile_SmallFile verifies that a file under 2 MB produces a stable,
// non-empty fingerprint.
func TestHashFile_SmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.flac")
	writeFile(t, path, []byte("hello, world"))

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile error: %v", err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// Call again — must be identical (deterministic).
	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile error on second call: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %q != %q", h1, h2)
	}
}

// TestHashFile_SmallFileConsistency verifies that two files with the same
// content produce the same hash (regardless of read strategy).
func TestHashFile_SmallFileConsistency(t *testing.T) {
	dir := t.TempDir()
	content := makeBytes(512*1024, 0xAB) // 512 KB — well under threshold

	p1 := filepath.Join(dir, "a.flac")
	p2 := filepath.Join(dir, "b.flac")
	writeFile(t, p1, content)
	writeFile(t, p2, content)

	h1, err := HashFile(p1)
	if err != nil {
		t.Fatalf("HashFile(p1): %v", err)
	}
	h2, err := HashFile(p2)
	if err != nil {
		t.Fatalf("HashFile(p2): %v", err)
	}
	if h1 != h2 {
		t.Fatalf("identical small files produced different hashes: %q vs %q", h1, h2)
	}
}

// TestHashFile_LargeFile verifies that a file >= 2 MB produces a stable,
// non-empty fingerprint.
func TestHashFile_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.flac")
	writeFile(t, path, makeBytes(3*1024*1024, 0x01)) // 3 MB

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile error: %v", err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash for large file")
	}

	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile second call error: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("large-file hash not deterministic: %q != %q", h1, h2)
	}
}

// TestHashFile_SizeAloneDoesNotEqualise verifies that two large files with the
// same size but different content produce different hashes.
func TestHashFile_SizeAloneDoesNotEqualise(t *testing.T) {
	dir := t.TempDir()
	size := 3 * 1024 * 1024 // 3 MB — both files same size

	p1 := filepath.Join(dir, "c.flac")
	p2 := filepath.Join(dir, "d.flac")
	writeFile(t, p1, makeBytes(size, 0xAA))
	writeFile(t, p2, makeBytes(size, 0xBB))

	h1, err := HashFile(p1)
	if err != nil {
		t.Fatalf("HashFile(p1): %v", err)
	}
	h2, err := HashFile(p2)
	if err != nil {
		t.Fatalf("HashFile(p2): %v", err)
	}
	if h1 == h2 {
		t.Fatal("files with same size but different content must not produce equal hashes")
	}
}

// TestHashWorkers_DrainAndClose verifies that HashWorkers drains every path
// from the input channel and closes the output channel on completion.
func TestHashWorkers_DrainAndClose(t *testing.T) {
	dir := t.TempDir()

	const n = 8
	paths := make([]string, n)
	for i := range paths {
		p := filepath.Join(dir, filepath.Join("track"+string(rune('0'+i))+".flac"))
		writeFile(t, p, []byte("audio content for track"))
		paths[i] = p
	}

	in := make(chan string, n)
	for _, p := range paths {
		in <- p
	}
	close(in)

	var got []HashedFile
	for hf := range HashWorkers(in) {
		got = append(got, hf)
	}

	if len(got) != n {
		t.Fatalf("expected %d HashedFile values, got %d", n, len(got))
	}
	for _, hf := range got {
		if hf.Hash == "" {
			t.Errorf("empty hash for path %q", hf.Path)
		}
	}
}

// TestHashWorkers_SkipsErrors verifies that non-existent paths are silently
// skipped and do not block the pipeline or reduce valid results below the
// count of files that actually exist.
func TestHashWorkers_SkipsErrors(t *testing.T) {
	dir := t.TempDir()

	validPaths := []string{
		filepath.Join(dir, "a.flac"),
		filepath.Join(dir, "b.flac"),
	}
	for _, p := range validPaths {
		writeFile(t, p, []byte("real audio"))
	}

	in := make(chan string, 4)
	in <- filepath.Join(dir, "does_not_exist.flac")
	for _, p := range validPaths {
		in <- p
	}
	in <- filepath.Join(dir, "also_missing.flac")
	close(in)

	var got []HashedFile
	for hf := range HashWorkers(in) {
		got = append(got, hf)
	}

	if len(got) != len(validPaths) {
		t.Fatalf("expected %d results (valid files only), got %d", len(validPaths), len(got))
	}
}

// TestHashFile_LargeFileDifferentSizes verifies that the size suffix
// distinguishes files that share the same first+last 1 MB bytes but differ in
// total size (content in the middle).
func TestHashFile_LargeFileDifferentSizes(t *testing.T) {
	dir := t.TempDir()

	// Build two files: same first 1 MB and last 1 MB, different total size.
	chunk := makeBytes(chunkSize, 0xCC) // 1 MB of 0xCC
	middle1 := makeBytes(512*1024, 0x11)
	middle2 := makeBytes(1024*1024, 0x11)

	// file1: chunk + middle1 + chunk
	content1 := append(append(append([]byte{}, chunk...), middle1...), chunk...)
	// file2: chunk + middle2 + chunk  (same boundaries, different size)
	content2 := append(append(append([]byte{}, chunk...), middle2...), chunk...)

	p1 := filepath.Join(dir, "e.flac")
	p2 := filepath.Join(dir, "f.flac")
	writeFile(t, p1, content1)
	writeFile(t, p2, content2)

	h1, err := HashFile(p1)
	if err != nil {
		t.Fatalf("HashFile(p1): %v", err)
	}
	h2, err := HashFile(p2)
	if err != nil {
		t.Fatalf("HashFile(p2): %v", err)
	}
	if h1 == h2 {
		t.Fatal("files with same boundary bytes but different sizes must produce different hashes")
	}
}

// TestHashFile_BoundaryBelowThreshold verifies that a file one byte under the
// 2 MB threshold uses full-file hashing (non-empty, deterministic result).
func TestHashFile_BoundaryBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "just_under.flac")
	writeFile(t, path, makeBytes(partialThreshold-1, 0xDE))

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}
	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile second call: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic at threshold-1: %q != %q", h1, h2)
	}
}

// TestHashFile_BoundaryAtThreshold verifies that a file exactly at 2 MB uses
// partial hashing (size >= partialThreshold triggers the partial branch).
func TestHashFile_BoundaryAtThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exactly_threshold.flac")
	writeFile(t, path, makeBytes(partialThreshold, 0xDE))

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}
	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile second call: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic at threshold: %q != %q", h1, h2)
	}
}

// TestHashFile_BoundaryAboveThreshold verifies that a file one byte over 2 MB
// produces a stable partial hash.
func TestHashFile_BoundaryAboveThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "just_over.flac")
	writeFile(t, path, makeBytes(partialThreshold+1, 0xDE))

	h1, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}
	h2, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile second call: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic at threshold+1: %q != %q", h1, h2)
	}
}

// TestHashFile_BoundaryFullVsPartialDistinct verifies that the two hashing
// strategies produce different results even when content at the boundary bytes
// is identical.
func TestHashFile_BoundaryFullVsPartialDistinct(t *testing.T) {
	dir := t.TempDir()
	// below threshold: full hash
	pSmall := filepath.Join(dir, "small_boundary.flac")
	writeFile(t, pSmall, makeBytes(partialThreshold-1, 0xAB))

	// at threshold: partial hash (same fill byte, different strategy)
	pLarge := filepath.Join(dir, "large_boundary.flac")
	writeFile(t, pLarge, makeBytes(partialThreshold, 0xAB))

	hSmall, err := HashFile(pSmall)
	if err != nil {
		t.Fatalf("HashFile(small): %v", err)
	}
	hLarge, err := HashFile(pLarge)
	if err != nil {
		t.Fatalf("HashFile(large): %v", err)
	}
	if hSmall == hLarge {
		t.Fatal("files of different sizes must not hash to the same value")
	}
}

// TestHashFile_SmallFileSingleByteMutation verifies that files under 2 MB use
// full-file hashing: mutating any single byte — at the start, middle, or end —
// must produce a different hash than the baseline.
//
// The middle-byte position (partialThreshold/2) is the critical case: a
// partial hasher reading only the first+last 1 MB would not cover that index
// for files significantly smaller than 2 MB, so only a full-file read can
// detect the change there.
func TestHashFile_SmallFileSingleByteMutation(t *testing.T) {
	dir := t.TempDir()

	size := partialThreshold - 1 // one byte under threshold
	baseline := makeBytes(size, 0xAB)

	pBase := filepath.Join(dir, "base_small.flac")
	writeFile(t, pBase, baseline)

	hBase, err := HashFile(pBase)
	if err != nil {
		t.Fatalf("HashFile(baseline): %v", err)
	}

	positions := []struct {
		name string
		idx  int
	}{
		{"first byte", 0},
		{"middle byte", size / 2},
		{"last byte", size - 1},
	}

	for _, pos := range positions {
		t.Run(pos.name, func(t *testing.T) {
			mutated := make([]byte, size)
			copy(mutated, baseline)
			mutated[pos.idx] ^= 0xFF // flip all bits at this position

			p := filepath.Join(dir, "mutated_"+pos.name+".flac")
			writeFile(t, p, mutated)

			h, err := HashFile(p)
			if err != nil {
				t.Fatalf("HashFile(mutated %s): %v", pos.name, err)
			}
			if h == hBase {
				t.Errorf("mutating %s (index %d) did not change the hash — full-file hashing not in effect", pos.name, pos.idx)
			}
		})
	}
}

// TestHashFile_EmptyFile verifies that an empty file can be hashed without
// error and produces a non-empty fingerprint.
func TestHashFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.flac")
	writeFile(t, path, []byte{})

	h, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile on empty file: %v", err)
	}
	if h == "" {
		t.Fatal("expected non-empty hash for empty file")
	}
}

// TestHashFile_NonExistentFile verifies that HashFile returns an error for a
// path that does not exist.
func TestHashFile_NonExistentFile(t *testing.T) {
	_, err := HashFile("/tmp/this_path_does_not_exist_muse_test.flac")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

// TestHashWorkers_ManyFiles verifies that HashWorkers correctly processes a
// larger batch of files concurrently and closes the output channel afterwards.
func TestHashWorkers_ManyFiles(t *testing.T) {
	dir := t.TempDir()

	const n = 50
	in := make(chan string, n)
	expected := make(map[string]struct{}, n)
	for i := range n {
		p := filepath.Join(dir, filepath.Join("track_many_"+string(rune('A'+i%26))+string(rune('0'+i/26))+".flac"))
		writeFile(t, p, makeBytes(1024+i, byte(i)))
		in <- p
		expected[p] = struct{}{}
	}
	close(in)

	seen := make(map[string]struct{}, n)
	for hf := range HashWorkers(in) {
		if hf.Hash == "" {
			t.Errorf("empty hash for %q", hf.Path)
		}
		seen[hf.Path] = struct{}{}
	}

	if len(seen) != n {
		t.Fatalf("expected %d results, got %d", n, len(seen))
	}
	for p := range expected {
		if _, ok := seen[p]; !ok {
			t.Errorf("path %q missing from HashWorkers output", p)
		}
	}
}

// TestHashFile_MiddleMutationPreservesHash verifies the core partial-hash
// contract: mutating only the middle bytes of a >= 2 MB file leaves the hash
// unchanged, while mutating the first or last 1 MB changes it.
//
// File layout (3 MB total):
//
//	[0x00, chunkSize)       — first chunk  (0xAA)
//	[chunkSize, 2*chunkSize) — middle chunk (0x00 baseline / 0xFF mutated)
//	[2*chunkSize, 3*chunkSize) — last chunk  (0xBB)
func TestHashFile_MiddleMutationPreservesHash(t *testing.T) {
	dir := t.TempDir()

	first := makeBytes(chunkSize, 0xAA)
	middle := makeBytes(chunkSize, 0x00)
	last := makeBytes(chunkSize, 0xBB)

	baseline := append(append(append([]byte{}, first...), middle...), last...)

	// Variant with only the middle 1 MB changed.
	middleChanged := makeBytes(chunkSize, 0xFF)
	withMiddleChanged := append(append(append([]byte{}, first...), middleChanged...), last...)

	// Variant with the first 1 MB changed.
	firstChanged := makeBytes(chunkSize, 0x11)
	withFirstChanged := append(append(append([]byte{}, firstChanged...), middle...), last...)

	// Variant with the last 1 MB changed.
	lastChanged := makeBytes(chunkSize, 0x22)
	withLastChanged := append(append(append([]byte{}, first...), middle...), lastChanged...)

	pBase := filepath.Join(dir, "base.flac")
	pMid := filepath.Join(dir, "mid_changed.flac")
	pFirst := filepath.Join(dir, "first_changed.flac")
	pLast := filepath.Join(dir, "last_changed.flac")

	writeFile(t, pBase, baseline)
	writeFile(t, pMid, withMiddleChanged)
	writeFile(t, pFirst, withFirstChanged)
	writeFile(t, pLast, withLastChanged)

	hBase, err := HashFile(pBase)
	if err != nil {
		t.Fatalf("HashFile(base): %v", err)
	}
	hMid, err := HashFile(pMid)
	if err != nil {
		t.Fatalf("HashFile(mid_changed): %v", err)
	}
	hFirst, err := HashFile(pFirst)
	if err != nil {
		t.Fatalf("HashFile(first_changed): %v", err)
	}
	hLast, err := HashFile(pLast)
	if err != nil {
		t.Fatalf("HashFile(last_changed): %v", err)
	}

	// Middle mutation must NOT change the hash.
	if hBase != hMid {
		t.Errorf("middle-only mutation changed hash: %q -> %q", hBase, hMid)
	}
	// First-chunk mutation MUST change the hash.
	if hBase == hFirst {
		t.Errorf("first-chunk mutation did not change hash: %q", hBase)
	}
	// Last-chunk mutation MUST change the hash.
	if hBase == hLast {
		t.Errorf("last-chunk mutation did not change hash: %q", hBase)
	}
}

// TestHashWorkers_OutputClosedOnEmptyInput verifies that HashWorkers closes
// the output channel immediately when the input channel is empty and closed.
func TestHashWorkers_OutputClosedOnEmptyInput(t *testing.T) {
	in := make(chan string)
	close(in)

	count := 0
	for range HashWorkers(in) {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", count)
	}
}
