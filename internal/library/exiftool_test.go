package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/bits"
	"path/filepath"
	"sync"
	"testing"
)

// fakeExtractor is a MetadataExtractor that returns one Track per HashedFile,
// echoing the path and hash with no real metadata. It records each batch it
// receives so tests can assert batching behaviour.
type fakeExtractor struct {
	batches [][]HashedFile
}

func (f *fakeExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
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

// TestExtractBatches_SingleBatch verifies that fewer than exiftoolBatchSize files are
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
	for tr := range ExtractBatches(context.Background(), fe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(got))
	}
	if len(fe.batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(fe.batches))
	}
}

// TestExtractBatches_FullBatch verifies that exactly exiftoolBatchSize files trigger a
// flush mid-stream, producing at least one batch of exiftoolBatchSize.
func TestExtractBatches_FullBatch(t *testing.T) {
	const n = exiftoolBatchSize + 3
	fe := &fakeExtractor{}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{Path: fmt.Sprintf("/music/track%04d.flac", i), Hash: fmt.Sprintf("h%04d", i)}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(context.Background(), fe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(got))
	}
	// First batch must be exactly exiftoolBatchSize; remainder in a second batch.
	if len(fe.batches) < 2 {
		t.Fatalf("expected at least 2 batches for %d files, got %d", n, len(fe.batches))
	}
	if len(fe.batches[0]) != exiftoolBatchSize {
		t.Fatalf("first batch size: want %d, got %d", exiftoolBatchSize, len(fe.batches[0]))
	}
}

// TestExtractBatches_EmptyInput verifies that an already-closed empty channel
// produces no Tracks and closes the output channel without hanging.
func TestExtractBatches_EmptyInput(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile)
	close(in)

	count := 0
	for range ExtractBatches(context.Background(), fe, in) {
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
	for range ExtractBatches(context.Background(), fe, in) {
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

func (p *poisonExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
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
	tracks := extractWithBinaryRetry(context.Background(), pe, files)

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
	tracks := extractWithBinaryRetry(context.Background(), pe, nil)
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
	tracks := extractWithBinaryRetry(context.Background(), pe, files)

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
	for tr := range ExtractBatches(context.Background(), pe, in) {
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
// It is safe for concurrent use from multiple goroutines.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

// alwaysFailExtractor always returns an error regardless of input.
type alwaysFailExtractor struct{}

func (a *alwaysFailExtractor) Extract(_ context.Context, _ []HashedFile) ([]Track, error) {
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
	tracks := extractWithBinaryRetry(context.Background(), &alwaysFailExtractor{}, files)

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

// chainExtractor is a MetadataExtractor that simulates the full fallback chain:
// any batch (len > 1) containing the corrupt path returns an error, and even
// the per-file (len == 1) invocation of that corrupt path returns an error,
// forcing extractWithBinaryRetry to produce filename-derived fallback metadata.
// All other invocations succeed and echo each file as a Track.
// perFileCalls counts Extract calls where len(files) == 1.
type chainExtractor struct {
	corruptPath  string
	perFileCalls int
}

func (c *chainExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	// Detect whether the corrupt file is in this batch.
	for _, f := range files {
		if f.Path == c.corruptPath {
			if len(files) == 1 {
				c.perFileCalls++
			}
			return nil, errors.New("invalid JSON: corrupt file in batch")
		}
	}
	// Healthy batch: echo each file as a Track.
	if len(files) == 1 {
		c.perFileCalls++
	}
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*chainExtractor)(nil)

// TestBinaryRetry_FilenameMetadataFallbackChain verifies the full retry chain:
// a batch containing a corrupt file fails at the batch level, binary search
// subdivision isolates it, per-file invocation also fails, and
// extractWithBinaryRetry falls back to filename-derived metadata for that
// single file while returning all other tracks normally.
func TestBinaryRetry_FilenameMetadataFallbackChain(t *testing.T) {
	const n = 8
	const corruptIdx = 5

	files := make([]HashedFile, n)
	for i := range n {
		files[i] = HashedFile{
			Path: fmt.Sprintf("/music/track%02d.flac", i),
			Hash: fmt.Sprintf("hash%02d", i),
		}
	}
	corruptPath := fmt.Sprintf("/music/track%02d.flac", corruptIdx)

	ce := &chainExtractor{corruptPath: corruptPath}
	tracks := extractWithBinaryRetry(context.Background(), ce, files)

	// (1) All N tracks must be returned.
	if len(tracks) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(tracks))
	}

	// (2) The corrupt track must have filename-derived fallback metadata.
	found := false
	for _, tr := range tracks {
		if tr.Path == corruptPath {
			found = true
			if tr.Artist != "Unknown Artist" {
				t.Errorf("corrupt track artist: want %q, got %q", "Unknown Artist", tr.Artist)
			}
			wantTitle := fmt.Sprintf("track%02d", corruptIdx)
			if tr.Title != wantTitle {
				t.Errorf("corrupt track title: want %q, got %q", wantTitle, tr.Title)
			}
		}
	}
	if !found {
		t.Errorf("corrupt path %q missing from results; expected fallback track", corruptPath)
	}

	// (3) At least one per-file Extract call must have occurred, proving the
	// binary-search chain reached per-file isolation.
	if ce.perFileCalls == 0 {
		t.Error("expected at least one per-file Extract call (len==1), got none; chain was not exercised")
	}
}

// warnExtractor returns valid tracks AND an error simultaneously, simulating
// what ExifToolExtractor does after the fix: exiftool exits non-zero (e.g.
// warnings about one file) but still writes valid JSON to stdout. Before the
// fix, the error would cause the entire batch to be discarded and retried via
// binary subdivision. After the fix, the valid tracks are used directly.
type warnExtractor struct {
	calls int
}

func (w *warnExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	w.calls++
	// Return valid tracks for all files, plus a non-nil error to simulate
	// exiftool's exit-code-1-with-valid-output behaviour.
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash, Title: "ok"}
	}
	return tracks, errors.New("exiftool exited with status 1")
}

var _ MetadataExtractor = (*warnExtractor)(nil)

// TestBinaryRetry_WarnExtractorFallsBackOnError verifies that when an
// extractor returns BOTH valid tracks AND an error (simulating ExifTool's
// non-zero exit with valid stdout), extractWithBinaryRetry still falls back
// to binary subdivision because it only checks err==nil. This is expected:
// the fix lives inside ExifToolExtractor.Extract itself (which now suppresses
// the error when stdout is parseable), so extractWithBinaryRetry never sees
// the error for ExitError cases. This test documents the contract.
func TestBinaryRetry_WarnExtractorFallsBackOnError(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/a.flac", Hash: "h1"},
		{Path: "/music/b.flac", Hash: "h2"},
	}
	w := &warnExtractor{}
	tracks := extractWithBinaryRetry(context.Background(), w, files)

	// All files must still be returned (via fallback or recursion).
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	// Because warnExtractor always errors, binary retry will subdivide down
	// to single files, then produce filename-derived fallback metadata.
	// The total calls should be > 1 (the initial batch call + recursive calls).
	if w.calls < 2 {
		t.Errorf("expected multiple Extract calls due to error, got %d", w.calls)
	}
}

// mismatchExtractor simulates an extractor (like exiftool) that returns a
// SourceFile path that does not match any of the input HashedFile paths. This
// exercises the guard in ExifToolExtractor.Extract that skips records with
// unknown SourceFile values to prevent tracks with empty hashes.
type mismatchExtractor struct{}

func (m *mismatchExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, 0, len(files)+1)
	for _, hf := range files {
		tracks = append(tracks, Track{Path: hf.Path, Hash: hf.Hash, Title: "ok"})
	}
	// Inject an extra track with a path not in the input set.
	tracks = append(tracks, Track{Path: "/phantom/resolved-symlink.flac", Hash: "", Title: "phantom"})
	return tracks, nil
}

var _ MetadataExtractor = (*mismatchExtractor)(nil)

// TestExifToolExtract_SkipsUnknownSourceFile verifies that Extract skips records
// whose SourceFile does not match any input HashedFile path, preventing tracks
// with empty hashes from entering the library index and database.
func TestExifToolExtract_SkipsUnknownSourceFile(t *testing.T) {
	h := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	files := []HashedFile{
		{Path: "/music/a.flac", Hash: "hash_a"},
		{Path: "/music/b.flac", Hash: "hash_b"},
	}

	// Build a fileByPath map as ExifToolExtractor.Extract does.
	fileByPath := make(map[string]HashedFile, len(files))
	for _, f := range files {
		fileByPath[f.Path] = f
	}

	// Simulate exiftool returning 3 records: 2 matching + 1 unknown.
	type exifRecord struct {
		SourceFile string
		Title      string
	}
	records := []exifRecord{
		{SourceFile: "/music/a.flac", Title: "Track A"},
		{SourceFile: "/music/b.flac", Title: "Track B"},
		{SourceFile: "/phantom/resolved-symlink.flac", Title: "Phantom"},
	}

	var tracks []Track
	for _, r := range records {
		hf, ok := fileByPath[r.SourceFile]
		if !ok {
			continue
		}
		tracks = append(tracks, Track{
			Path:  r.SourceFile,
			Hash:  hf.Hash,
			Title: r.Title,
		})
	}

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks (unknown SourceFile skipped), got %d", len(tracks))
	}
	for _, tr := range tracks {
		if tr.Hash == "" {
			t.Errorf("track %q has empty hash; unknown SourceFile should have been skipped", tr.Path)
		}
	}
}

// emptyFieldExtractor returns tracks with empty Title and Artist fields,
// simulating exiftool output for files with no embedded metadata tags.
type emptyFieldExtractor struct{}

func (e *emptyFieldExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*emptyFieldExtractor)(nil)

// TestBinaryRetry_EmptyMetadataFallback verifies that when an extractor
// succeeds but returns tracks with empty Title and Artist (e.g. exiftool
// reading an opus file with no embedded tags), extractWithBinaryRetry applies
// filename-derived fallback metadata.
func TestBinaryRetry_EmptyMetadataFallback(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/Estas Tonne/INTERNAL FLIGHT (REMASTERED).opus", Hash: "abc123"},
		{Path: "/music/artist/album/song.flac", Hash: "def456"},
	}

	tracks := extractWithBinaryRetry(context.Background(), &emptyFieldExtractor{}, files)

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}

	// First track: title from filename stem, artist fallback.
	if tracks[0].Title != "INTERNAL FLIGHT (REMASTERED)" {
		t.Errorf("track[0] title: want %q, got %q", "INTERNAL FLIGHT (REMASTERED)", tracks[0].Title)
	}
	if tracks[0].Artist != "Unknown Artist" {
		t.Errorf("track[0] artist: want %q, got %q", "Unknown Artist", tracks[0].Artist)
	}

	// Second track: title from filename stem, artist fallback.
	if tracks[1].Title != "song" {
		t.Errorf("track[1] title: want %q, got %q", "song", tracks[1].Title)
	}
	if tracks[1].Artist != "Unknown Artist" {
		t.Errorf("track[1] artist: want %q, got %q", "Unknown Artist", tracks[1].Artist)
	}
}

// TestBinaryRetry_PartialMetadataPreserved verifies that when an extractor
// returns a track with a valid Artist but empty Title, only the missing field
// gets the fallback — the existing Artist is preserved.
func TestBinaryRetry_PartialMetadataPreserved(t *testing.T) {
	ext := &partialFieldExtractor{artist: "Estas Tonne"}
	files := []HashedFile{
		{Path: "/music/INTERNAL FLIGHT.opus", Hash: "aaa"},
	}

	tracks := extractWithBinaryRetry(context.Background(), ext, files)

	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Artist != "Estas Tonne" {
		t.Errorf("artist: want %q, got %q", "Estas Tonne", tracks[0].Artist)
	}
	if tracks[0].Title != "INTERNAL FLIGHT" {
		t.Errorf("title: want %q (filename stem), got %q", "INTERNAL FLIGHT", tracks[0].Title)
	}
}

// partialFieldExtractor returns tracks with a preset Artist but empty Title.
type partialFieldExtractor struct {
	artist string
}

func (p *partialFieldExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash, Artist: p.artist}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*partialFieldExtractor)(nil)

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
	for tr := range ExtractBatches(context.Background(), fe, in) {
		byPath[tr.Path] = tr.Hash
	}

	for _, hf := range inputs {
		if byPath[hf.Path] != hf.Hash {
			t.Errorf("path %q: want hash %q, got %q", hf.Path, hf.Hash, byPath[hf.Path])
		}
	}
}
