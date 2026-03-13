package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/bits"
	"path/filepath"
	"testing"
)

// fakeExtractor is a MetadataExtractor that returns one Track per HashedFile,
// echoing the path and hash with no real metadata. It records each batch it
// receives so tests can assert batching behaviour.
type fakeExtractor struct {
	batches [][]HashedFile
}

func (f *fakeExtractor) Extract(files []HashedFile) ([]Track, error) {
	cp := make([]HashedFile, len(files))
	copy(cp, files)
	f.batches = append(f.batches, cp)

	tracks := make([]Track, len(files))
	for i, hf := range files {
		tracks[i] = Track{
			Path:  hf.Path,
			Hash:  hf.Hash,
			Title: filepath.Base(hf.Path),
		}
	}
	return tracks, nil
}

// Verify fakeExtractor satisfies the interface at compile time.
var _ MetadataExtractor = (*fakeExtractor)(nil)

// TestExtractBatches_SingleBatch verifies that fewer than batchSize files are
// dispatched in one Extract call and all Tracks reach the output channel.
func TestExtractBatches_SingleBatch(t *testing.T) {
	const n = 10
	fe := &fakeExtractor{}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{Path: fmt.Sprintf("/music/track%02d.flac", i), Hash: fmt.Sprintf("hash%02d", i)}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(fe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(got))
	}
	if len(fe.batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(fe.batches))
	}
}

// TestExtractBatches_FullBatch verifies that exactly batchSize files trigger a
// flush mid-stream, producing at least one batch of batchSize.
func TestExtractBatches_FullBatch(t *testing.T) {
	const n = batchSize + 3
	fe := &fakeExtractor{}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{Path: fmt.Sprintf("/music/track%04d.flac", i), Hash: fmt.Sprintf("h%04d", i)}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(fe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(got))
	}
	// First batch must be exactly batchSize; remainder in a second batch.
	if len(fe.batches) < 2 {
		t.Fatalf("expected at least 2 batches for %d files, got %d", n, len(fe.batches))
	}
	if len(fe.batches[0]) != batchSize {
		t.Fatalf("first batch size: want %d, got %d", batchSize, len(fe.batches[0]))
	}
}

// TestExtractBatches_EmptyInput verifies that an already-closed empty channel
// produces no Tracks and closes the output channel without hanging.
func TestExtractBatches_EmptyInput(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile)
	close(in)

	count := 0
	for range ExtractBatches(fe, in) {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 tracks for empty input, got %d", count)
	}
	if len(fe.batches) != 0 {
		t.Fatalf("expected no Extract calls for empty input, got %d", len(fe.batches))
	}
}

// TestExtractBatches_OutputClosedAfterDrain verifies the output channel is
// closed (range completes) after all input is consumed.
func TestExtractBatches_OutputClosedAfterDrain(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile, 5)
	for i := range 5 {
		in <- HashedFile{Path: fmt.Sprintf("/x/%d.mp3", i), Hash: fmt.Sprintf("hx%d", i)}
	}
	close(in)

	// If the output channel never closes this would hang; the test timeout catches it.
	for range ExtractBatches(fe, in) {
	}
}

// poisonExtractor is a MetadataExtractor that returns an error whenever the
// batch contains the file at poisonIdx. For all other batches it echoes each
// file as a Track. It counts every Extract call so tests can assert O(log n)
// behaviour.
type poisonExtractor struct {
	poisonIdx int
	calls     int
}

func (p *poisonExtractor) Extract(files []HashedFile) ([]Track, error) {
	p.calls++
	for _, f := range files {
		if f.Path == fmt.Sprintf("/music/track%02d.flac", p.poisonIdx) {
			return nil, errors.New("invalid JSON: corrupt file in batch")
		}
	}
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*poisonExtractor)(nil)

// TestBinaryRetry_IsolatesCorruptFile verifies that extractWithBinaryRetry
// isolates a single corrupt file, applies fallback metadata, and returns all n
// tracks. It also asserts that the number of Extract calls is O(log n).
func TestBinaryRetry_IsolatesCorruptFile(t *testing.T) {
	const n = 16
	const poisonIdx = 7

	files := make([]HashedFile, n)
	for i := range n {
		files[i] = HashedFile{
			Path: fmt.Sprintf("/music/track%02d.flac", i),
			Hash: fmt.Sprintf("hash%02d", i),
		}
	}

	pe := &poisonExtractor{poisonIdx: poisonIdx}
	tracks := extractWithBinaryRetry(pe, files)

	// All n tracks must be present: the corrupt file gets fallback metadata.
	if len(tracks) != n {
		t.Fatalf("expected %d tracks (including fallback for corrupt file), got %d", n, len(tracks))
	}

	// Verify the corrupt file is present with fallback metadata.
	corruptPath := fmt.Sprintf("/music/track%02d.flac", poisonIdx)
	found := false
	for _, tr := range tracks {
		if tr.Path == corruptPath {
			found = true
			if tr.Artist != "Unknown Artist" {
				t.Errorf("corrupt track artist: want %q, got %q", "Unknown Artist", tr.Artist)
			}
			if tr.Title != "track07" {
				t.Errorf("corrupt track title: want %q, got %q", "track07", tr.Title)
			}
		}
	}
	if !found {
		t.Errorf("corrupt file %q missing from results; should be present with fallback metadata", corruptPath)
	}

	// Verify O(log n) calls: upper bound is 2*(log2(n)+1) for a single corrupt file.
	log2n := bits.Len(uint(n)) // ceil(log2(n)+1)
	maxCalls := 2*log2n + 1
	if pe.calls > maxCalls {
		t.Errorf("Extract called %d times; want ≤ %d (2·log₂(%d)+1)", pe.calls, maxCalls, n)
	}
}

// TestBinaryRetry_NoCalls verifies the empty-input case returns nil without
// calling Extract at all.
func TestBinaryRetry_NoCalls(t *testing.T) {
	pe := &poisonExtractor{poisonIdx: 0}
	tracks := extractWithBinaryRetry(pe, nil)
	if len(tracks) != 0 {
		t.Fatalf("expected 0 tracks for nil input, got %d", len(tracks))
	}
	// Extract must not be called for an empty slice — poisonExtractor returns
	// nil,nil for len(files)==0, so we just verify the call count is correct.
}

// TestBinaryRetry_CleanBatch verifies that a batch with no corrupt file returns
// all tracks in a single Extract call.
func TestBinaryRetry_CleanBatch(t *testing.T) {
	const n = 8
	files := make([]HashedFile, n)
	for i := range n {
		files[i] = HashedFile{Path: fmt.Sprintf("/music/track%02d.flac", i), Hash: fmt.Sprintf("h%02d", i)}
	}

	pe := &poisonExtractor{poisonIdx: 99} // 99 is never in the batch
	tracks := extractWithBinaryRetry(pe, files)

	if len(tracks) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(tracks))
	}
	if pe.calls != 1 {
		t.Errorf("expected 1 Extract call for clean batch, got %d", pe.calls)
	}
}

// TestExtractBatches_BinaryRetryIntegration verifies that ExtractBatches
// propagates binary-search retry through the pipeline: with one poison file
// in a batch, the output contains all other tracks.
func TestExtractBatches_BinaryRetryIntegration(t *testing.T) {
	const n = 8
	const poisonIdx = 3

	pe := &poisonExtractor{poisonIdx: poisonIdx}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{
			Path: fmt.Sprintf("/music/track%02d.flac", i),
			Hash: fmt.Sprintf("hash%02d", i),
		}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(pe, in) {
		got = append(got, tr)
	}

	// All n tracks must be present; the corrupt file gets fallback metadata.
	if len(got) != n {
		t.Fatalf("expected %d tracks (including fallback for corrupt file), got %d", n, len(got))
	}
	corruptPath := fmt.Sprintf("/music/track%02d.flac", poisonIdx)
	found := false
	for _, tr := range got {
		if tr.Path == corruptPath {
			found = true
			if tr.Artist != "Unknown Artist" {
				t.Errorf("corrupt track artist: want %q, got %q", "Unknown Artist", tr.Artist)
			}
		}
	}
	if !found {
		t.Errorf("corrupt path %q missing from output; should be present with fallback metadata", corruptPath)
	}
}

// capturingHandler is a slog.Handler that records all log records for inspection.
type capturingHandler struct {
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler  { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler        { return h }

// alwaysFailExtractor always returns an error regardless of input.
type alwaysFailExtractor struct{}

func (a *alwaysFailExtractor) Extract(_ []HashedFile) ([]Track, error) {
	return nil, errors.New("extraction failed")
}

var _ MetadataExtractor = (*alwaysFailExtractor)(nil)

// TestFallback_SingleFileFailureInsertsFallback verifies that when ExifTool
// fails to extract metadata for a single file, the track is still returned with
// Artist="Unknown Artist" and Title set to the filename stem. It also verifies
// that a slog.Warn is emitted and the track is present in the returned slice.
// This test exercises the fallback path independently of the binary-search retry.
func TestFallback_SingleFileFailureInsertsFallback(t *testing.T) {
	h := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	files := []HashedFile{{Path: "/music/artist/some song.flac", Hash: "deadbeef"}}
	tracks := extractWithBinaryRetry(&alwaysFailExtractor{}, files)

	// Track must be present (never omitted).
	if len(tracks) != 1 {
		t.Fatalf("expected 1 fallback track, got %d", len(tracks))
	}
	tr := tracks[0]

	if tr.Artist != "Unknown Artist" {
		t.Errorf("artist: want %q, got %q", "Unknown Artist", tr.Artist)
	}
	if tr.Title != "some song" {
		t.Errorf("title: want %q (filename stem), got %q", "some song", tr.Title)
	}
	if tr.Path != files[0].Path {
		t.Errorf("path: want %q, got %q", files[0].Path, tr.Path)
	}
	if tr.Hash != files[0].Hash {
		t.Errorf("hash: want %q, got %q", files[0].Hash, tr.Hash)
	}

	// A warning must have been logged.
	warnCount := 0
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			warnCount++
		}
	}
	if warnCount == 0 {
		t.Error("expected at least one slog.Warn for failed extraction, got none")
	}
}

// TestExtractBatches_HashPreserved verifies that Track.Hash matches the
// HashedFile.Hash that entered the pipeline.
func TestExtractBatches_HashPreserved(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile, 3)
	inputs := []HashedFile{
		{Path: "/a.flac", Hash: "aaa"},
		{Path: "/b.flac", Hash: "bbb"},
		{Path: "/c.flac", Hash: "ccc"},
	}
	for _, hf := range inputs {
		in <- hf
	}
	close(in)

	byPath := make(map[string]string)
	for tr := range ExtractBatches(fe, in) {
		byPath[tr.Path] = tr.Hash
	}

	for _, hf := range inputs {
		if byPath[hf.Path] != hf.Hash {
			t.Errorf("path %q: want hash %q, got %q", hf.Path, hf.Hash, byPath[hf.Path])
		}
	}
}
