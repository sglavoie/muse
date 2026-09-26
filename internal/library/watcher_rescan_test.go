package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// statPreservingExtractor echoes HashedFile fields into Track, preserving
// ModTime and SizeBytes.
type statPreservingExtractor struct{}

func (s *statPreservingExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, hf := range files {
		tracks[i] = Track{
			Path:      hf.Path,
			Hash:      hf.Hash,
			Title:     filepath.Base(hf.Path),
			ModTime:   hf.ModTime,
			SizeBytes: hf.SizeBytes,
		}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*statPreservingExtractor)(nil)

// TestRescan_PopulatesModTimeAndSizeBytes verifies that the rescan function
// populates ModTime and SizeBytes on the HashedFile passed to Extract.
func TestRescan_PopulatesModTimeAndSizeBytes(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "track.mp3")
	if err := os.WriteFile(p, []byte("ID3fake"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	ext := &statPreservingExtractor{}
	ctx := context.Background()
	out := make(chan WatchEvent, 16)

	pending := map[string]bool{p: false}
	rescan(ctx, pending, nil, ext, out)
	close(out)

	var tracks []Track
	for e := range out {
		if !e.Deleted {
			tracks = append(tracks, e.Track)
		}
	}

	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.ModTime != info.ModTime().Unix() {
		t.Errorf("ModTime: got %d, want %d", tr.ModTime, info.ModTime().Unix())
	}
	if tr.SizeBytes != info.Size() {
		t.Errorf("SizeBytes: got %d, want %d", tr.SizeBytes, info.Size())
	}
}

// TestRescan_ChunksExtractionByBatchSize verifies that a large debounce window
// (e.g. a folder of thousands of files dropped into the library) is extracted
// in exiftoolBatchSize chunks, as the full scan does, rather than as a single
// ExifTool command line racing one extract timeout.
func TestRescan_ChunksExtractionByBatchSize(t *testing.T) {
	root := t.TempDir()
	const n = 2*exiftoolBatchSize + 7
	pending := make(map[string]bool, n)
	for i := range n {
		p := filepath.Join(root, fmt.Sprintf("track%04d.mp3", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("ID3fake%d", i)), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		pending[p] = false
	}

	ext := &fakeExtractor{}
	out := make(chan WatchEvent, n)
	rescan(context.Background(), pending, nil, ext, out)
	close(out)

	emitted := 0
	for range out {
		emitted++
	}
	if emitted != n {
		t.Errorf("emitted %d tracks, want %d", emitted, n)
	}

	var sizes []int
	for _, b := range ext.batches {
		sizes = append(sizes, len(b))
	}
	want := []int{exiftoolBatchSize, exiftoolBatchSize, 7}
	if !slices.Equal(sizes, want) {
		t.Errorf("Extract batch sizes = %v, want %v", sizes, want)
	}
}

// TestRescan_NoZeroStatLeak verifies that rescan never appends a HashedFile
// with zero ModTime/SizeBytes to the Extract batch. Files whose os.Stat fails
// after a successful hash must be skipped entirely, mirroring the protection
// already present in hashWorkersN. Without this fix, a track emitted with
// zero stat fields would cause DiffScan to flag the path as changed on every
// subsequent rescan.
func TestRescan_NoZeroStatLeak(t *testing.T) {
	root := t.TempDir()
	healthy := filepath.Join(root, "ok.mp3")
	if err := os.WriteFile(healthy, []byte("ID3fake"), 0o644); err != nil {
		t.Fatalf("write healthy: %v", err)
	}
	// A non-existent path: HashFile fails fast, the file is dropped from
	// rescan output before any zero-stat HashedFile is ever constructed.
	missing := filepath.Join(root, "vanished.mp3")

	ext := &statPreservingExtractor{}
	ctx := context.Background()
	out := make(chan WatchEvent, 16)
	pending := map[string]bool{healthy: false, missing: false}
	rescan(ctx, pending, nil, ext, out)
	close(out)

	emittedHealthy := false
	for e := range out {
		if e.Deleted {
			continue
		}
		if e.Track.Path == missing {
			t.Errorf("vanished file %q must not be emitted", missing)
		}
		if e.Track.ModTime == 0 || e.Track.SizeBytes == 0 {
			t.Errorf("emitted track %q has zero ModTime/SizeBytes (stat-fail path leaked through)", e.Track.Path)
		}
		if e.Track.Path == healthy {
			emittedHealthy = true
		}
	}
	if !emittedHealthy {
		t.Errorf("healthy file %q was not emitted alongside the vanished one", healthy)
	}
}
