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

// TestBinaryRetry_WhitespaceMetadataTreatedAsEmpty verifies that an extractor
// returning whitespace-only Title/Artist values triggers the same
// filename-derived fallback as empty values. Otherwise the library shows
// blank-looking rows that still claim to have metadata.
func TestBinaryRetry_WhitespaceMetadataTreatedAsEmpty(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/Estas Tonne/INTERNAL FLIGHT.opus", Hash: "abc"},
	}
	tracks := extractWithBinaryRetry(context.Background(), &whitespaceFieldExtractor{}, files)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.Title != "INTERNAL FLIGHT" {
		t.Errorf("title: want %q (filename stem fallback), got %q", "INTERNAL FLIGHT", tr.Title)
	}
	if tr.Artist != "Unknown Artist" {
		t.Errorf("artist: want %q, got %q", "Unknown Artist", tr.Artist)
	}
	if tr.Album != "" {
		t.Errorf("album: want %q (whitespace trimmed), got %q", "", tr.Album)
	}
	if tr.Genre != "" {
		t.Errorf("genre: want %q (whitespace trimmed), got %q", "", tr.Genre)
	}
}

// TestExtractWithBinaryRetry_EmptyInputReturnsNil guards against an infinite
// recursion if extractWithBinaryRetry is ever invoked with an empty batch
// (len(files)/2 == 0 would otherwise call itself with the same slice).
func TestExtractWithBinaryRetry_EmptyInputReturnsNil(t *testing.T) {
	tracks := extractWithBinaryRetry(context.Background(), &alwaysFailExtractor{}, nil)
	if tracks != nil {
		t.Errorf("expected nil tracks for empty input, got %v", tracks)
	}
}

// TestExtractWithBinaryRetry_CancelledContextSkipsFallback verifies that when
// the context is cancelled, extractWithBinaryRetry does not generate
// filename-derived fallback tracks. Generating fallbacks on cancellation
// causes the cancelled scan to keep producing garbage tracks for every file
// after the failure point.
func TestExtractWithBinaryRetry_CancelledContextSkipsFallback(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/a.flac", Hash: "h1"},
		{Path: "/music/b.flac", Hash: "h2"},
		{Path: "/music/c.flac", Hash: "h3"},
		{Path: "/music/d.flac", Hash: "h4"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tracks := extractWithBinaryRetry(ctx, &alwaysFailExtractor{}, files)
	if tracks != nil {
		t.Errorf("expected nil tracks under cancelled ctx, got %d: %+v", len(tracks), tracks)
	}
}

// TestFilenameStem_DotPrefixedFile verifies that a filename consisting only
// of the extension (e.g. ".mp3") does not produce an empty Title via the
// filename fallback. An empty Title slips past downstream `if Title == ""`
// guards and produces blank library rows.
func TestFilenameStem_DotPrefixedFile(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/music/.mp3", ".mp3"},
		{"/music/.flac", ".flac"},
		{"/music/track.mp3", "track"},
		{"/music/no_ext", "no_ext"},
		{"/music/album/song.opus", "song"},
		{"/music/a.b.c.mp3", "a.b.c"},
	}
	for _, c := range cases {
		got := filenameStem(c.path)
		if got != c.want {
			t.Errorf("filenameStem(%q): got %q, want %q", c.path, got, c.want)
		}
	}
}

// TestExtractWithBinaryRetry_DotPrefixedFilenameNonEmptyTitle verifies the
// filename fallback path keeps a non-empty Title when the filename is
// extension-only (".mp3"). Previously this produced Title == "".
func TestExtractWithBinaryRetry_DotPrefixedFilenameNonEmptyTitle(t *testing.T) {
	files := []HashedFile{{Path: "/music/.mp3", Hash: "h"}}
	tracks := extractWithBinaryRetry(context.Background(), &emptyFieldExtractor{}, files)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Title == "" {
		t.Errorf("Title must not be empty for dot-prefixed filename, got %q", tracks[0].Title)
	}
}

// TestExtractWithBinaryRetry_SingleFileFailureDotPrefixed verifies the same
// guard in the single-file failure branch of extractWithBinaryRetry.
func TestExtractWithBinaryRetry_SingleFileFailureDotPrefixed(t *testing.T) {
	files := []HashedFile{{Path: "/music/.flac", Hash: "h"}}
	tracks := extractWithBinaryRetry(context.Background(), &alwaysFailExtractor{}, files)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 fallback track, got %d", len(tracks))
	}
	if tracks[0].Title == "" {
		t.Errorf("Title must not be empty in single-file fallback for %q", files[0].Path)
	}
}
