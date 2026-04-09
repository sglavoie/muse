package library

import (
	"context"
	"os"
	"path/filepath"
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