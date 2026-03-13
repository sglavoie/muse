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
