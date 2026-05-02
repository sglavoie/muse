package library

import (
	"context"
	"os"
	"testing"
)

func TestDiffScan_EmptyKnownIsFullScan(t *testing.T) {
	const n = 10
	dir := t.TempDir()
	makeAudioFiles(t, dir, n)

	fe := &fakeExtractor{}
	trackCh, deleted, err := DiffScan(context.Background(), []string{dir}, nil, fe, nil, nil)
	if err != nil {
		t.Fatalf("DiffScan: %v", err)
	}

	var count int
	for range trackCh {
		count++
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty on first run", deleted)
	}
	if count != n {
		t.Errorf("tracks: got %d, want %d", count, n)
	}
}

func TestDiffScan_UnchangedFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 5)

	known := make(map[string]KnownFile, len(paths))
	for _, p := range paths {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	fe := &fakeExtractor{}
	trackCh, deleted, err := DiffScan(context.Background(), []string{dir}, nil, fe, known, nil)
	if err != nil {
		t.Fatalf("DiffScan: %v", err)
	}

	var count int
	for range trackCh {
		count++
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if count != 0 {
		t.Errorf("tracks: got %d, want 0 (all unchanged)", count)
	}
}

func TestDiffScan_DeletedFilesReported(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 3)

	known := make(map[string]KnownFile, len(paths))
	for _, p := range paths {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}
	if err := os.Remove(paths[2]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	fe := &fakeExtractor{}
	trackCh, deleted, err := DiffScan(context.Background(), []string{dir}, nil, fe, known, nil)
	if err != nil {
		t.Fatalf("DiffScan: %v", err)
	}
	for range trackCh {
	}

	if len(deleted) != 1 || deleted[0] != paths[2] {
		t.Errorf("deleted: got %v, want [%s]", deleted, paths[2])
	}
}
