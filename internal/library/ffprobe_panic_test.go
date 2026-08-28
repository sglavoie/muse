package library

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestFFProbeExtractor_WorkerPanicDoesNotEmitZeroTrack verifies that a panic
// raised while probing one file degrades only that file. Extraction writes into
// a preallocated slice indexed by input position, so a worker that died mid-loop
// used to leave zero-value Tracks behind — rows with an empty path and hash that
// the scan bridge would then upsert into the library.
func TestFFProbeExtractor_WorkerPanicDoesNotEmitZeroTrack(t *testing.T) {
	const n = 12
	const badIndex = 5

	files := make([]HashedFile, n)
	for i := range files {
		files[i] = HashedFile{
			Path:      fmt.Sprintf("/music/track_%02d.mp3", i),
			Hash:      fmt.Sprintf("hash%02d", i),
			ModTime:   int64(1700000000 + i),
			SizeBytes: int64(1024 + i),
		}
	}

	prev := probeFileFn
	t.Cleanup(func() { probeFileFn = prev })
	probeFileFn = func(_ context.Context, f HashedFile) Track {
		if f.Path == files[badIndex].Path {
			panic("ffprobe output blew up")
		}
		return Track{
			Path:      f.Path,
			Hash:      f.Hash,
			Artist:    "Real Artist",
			Title:     "Real Title",
			ModTime:   f.ModTime,
			SizeBytes: f.SizeBytes,
		}
	}

	tracks, err := (FFProbeExtractor{}).Extract(t.Context(), files)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(tracks) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(tracks))
	}

	for i, tr := range tracks {
		if tr.Path != files[i].Path {
			t.Fatalf("track %d: Path = %q, want %q (zero-value or misaligned slot)", i, tr.Path, files[i].Path)
		}
		if tr.Hash != files[i].Hash {
			t.Errorf("track %d: Hash = %q, want %q", i, tr.Hash, files[i].Hash)
		}
		if tr.ModTime != files[i].ModTime || tr.SizeBytes != files[i].SizeBytes {
			t.Errorf("track %d: stat info not carried through: %+v", i, tr)
		}
	}

	// The panicking file falls back to filename-derived metadata...
	bad := tracks[badIndex]
	if bad.Artist != "Unknown Artist" {
		t.Errorf("panicking file: Artist = %q, want %q", bad.Artist, "Unknown Artist")
	}
	if want := strings.TrimSuffix("track_05.mp3", ".mp3"); bad.Title != want {
		t.Errorf("panicking file: Title = %q, want %q", bad.Title, want)
	}

	// ...and every other file is still probed normally, proving recovery is per
	// item rather than per worker.
	for i, tr := range tracks {
		if i == badIndex {
			continue
		}
		if tr.Artist != "Real Artist" || tr.Title != "Real Title" {
			t.Errorf("track %d was abandoned by the panicking worker: %+v", i, tr)
		}
	}
}
