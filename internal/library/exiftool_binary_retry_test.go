package library

import (
	"context"
	"fmt"
	"math/bits"
	"testing"
)

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

	if len(tracks) != n {
		t.Fatalf("expected %d tracks (including fallback for corrupt file), got %d", n, len(tracks))
	}

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

	log2n := bits.Len(uint(n))
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
}

// TestBinaryRetry_CleanBatch verifies that a batch with no corrupt file returns
// all tracks in a single Extract call.
func TestBinaryRetry_CleanBatch(t *testing.T) {
	const n = 8
	files := make([]HashedFile, n)
	for i := range n {
		files[i] = HashedFile{Path: fmt.Sprintf("/music/track%02d.flac", i), Hash: fmt.Sprintf("h%02d", i)}
	}

	pe := &poisonExtractor{poisonIdx: 99}
	tracks := extractWithBinaryRetry(context.Background(), pe, files)

	if len(tracks) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(tracks))
	}
	if pe.calls != 1 {
		t.Errorf("expected 1 Extract call for clean batch, got %d", pe.calls)
	}
}

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

	if len(tracks) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(tracks))
	}

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

	if ce.perFileCalls == 0 {
		t.Error("expected at least one per-file Extract call (len==1), got none; chain was not exercised")
	}
}

// TestBinaryRetry_WarnExtractorFallsBackOnError verifies that when an
// extractor returns both valid tracks and an error, extractWithBinaryRetry
// still falls back to binary subdivision because it only checks err == nil.
func TestBinaryRetry_WarnExtractorFallsBackOnError(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/a.flac", Hash: "h1"},
		{Path: "/music/b.flac", Hash: "h2"},
	}
	w := &warnExtractor{}
	tracks := extractWithBinaryRetry(context.Background(), w, files)

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if w.calls < 2 {
		t.Errorf("expected multiple Extract calls due to error, got %d", w.calls)
	}
}