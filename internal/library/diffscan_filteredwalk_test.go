package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilteredWalk_AllNewFiles(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 5)

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, nil, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if len(changed) != len(paths) {
		t.Errorf("changed: got %d, want %d", len(changed), len(paths))
	}
}

func TestFilteredWalk_UnchangedFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 3)

	known := make(map[string]KnownFile, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if len(changed) != 0 {
		t.Errorf("changed: got %v, want empty (all files unchanged)", changed)
	}
}

func TestFilteredWalk_ModifiedFileIncluded(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 2)

	info, err := os.Stat(paths[0])
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Files can be created on opposite sides of a second boundary. Snapshot
	// the unchanged file itself rather than borrowing the first file's time.
	unchanged, err := os.Stat(paths[1])
	if err != nil {
		t.Fatalf("stat unchanged file: %v", err)
	}
	known := map[string]KnownFile{
		paths[0]: {ModTime: info.ModTime().Unix() - 1, SizeBytes: info.Size()},
		paths[1]: {ModTime: unchanged.ModTime().Unix(), SizeBytes: unchanged.Size()},
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if len(changed) != 1 || changed[0] != paths[0] {
		t.Errorf("changed: got %v, want [%s]", changed, paths[0])
	}
}

func TestFilteredWalk_DeletedFileReported(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 2)

	known := make(map[string]KnownFile, 2)
	for _, p := range paths {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}
	if err := os.Remove(paths[1]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(changed) != 0 {
		t.Errorf("changed: got %v, want empty", changed)
	}
	if len(deleted) != 1 || deleted[0] != paths[1] {
		t.Errorf("deleted: got %v, want [%s]", deleted, paths[1])
	}
}

func TestFilteredWalk_HiddenFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	visible := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(visible, []byte("ID3"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	hidden := filepath.Join(dir, ".hidden.mp3")
	if err := os.WriteFile(hidden, []byte("ID3"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, _, _ := FilteredWalk(context.Background(), []string{dir}, nil, nil, nil)

	if len(changed) != 1 || changed[0] != visible {
		t.Errorf("changed: got %v, want [%s]", changed, visible)
	}
}

func TestFilteredWalk_NonAudioFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "track.flac")
	if err := os.WriteFile(audio, []byte("fLaC"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	nonAudio := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(nonAudio, []byte("text"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, _, _ := FilteredWalk(context.Background(), []string{dir}, nil, nil, nil)

	if len(changed) != 1 || changed[0] != audio {
		t.Errorf("changed: got %v, want [%s]", changed, audio)
	}
}

func TestFilteredWalk_SizeMismatchTriggersRescan(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(p, []byte("ID3"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, _ := os.Stat(p)
	known := map[string]KnownFile{
		p: {ModTime: info.ModTime().Unix(), SizeBytes: info.Size() + 1},
	}

	changed, _, _ := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)

	if len(changed) != 1 {
		t.Errorf("changed: got %v, want 1 file (size mismatch)", changed)
	}
}

func TestFilteredWalk_CancelledContextNilDeletions(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	makeAudioFiles(t, dir1, 3)
	paths2 := makeAudioFiles(t, dir2, 3)

	known := make(map[string]KnownFile)
	for _, p := range paths2 {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, deleted, err := FilteredWalk(ctx, []string{dir1, dir2}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}
	if deleted != nil {
		t.Errorf("deleted: got %v, want nil (context cancelled)", deleted)
	}
}

func TestFilteredWalk_MultipleRootsDeletedFileReported(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	paths1 := makeAudioFiles(t, dir1, 2)
	paths2 := makeAudioFiles(t, dir2, 2)

	known := make(map[string]KnownFile)
	for _, p := range append(paths1, paths2...) {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	if err := os.Remove(paths1[1]); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Remove(paths2[0]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, deleted, err := FilteredWalk(context.Background(), []string{dir1, dir2}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 2 {
		t.Errorf("deleted: got %d paths, want 2: %v", len(deleted), deleted)
	}
	deletedSet := make(map[string]bool, len(deleted))
	for _, p := range deleted {
		deletedSet[p] = true
	}
	if !deletedSet[paths1[1]] {
		t.Errorf("expected %q in deleted, got %v", paths1[1], deleted)
	}
	if !deletedSet[paths2[0]] {
		t.Errorf("expected %q in deleted, got %v", paths2[0], deleted)
	}
}

// A missing root is skipped rather than failing the walk, so a first-run user
// with no music directory still boots. Deletions stay suppressed: the known
// files were never observed, which is not the same as having been deleted.
func TestFilteredWalk_MissingRootSkippedAndNoDeletions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	knownPath := filepath.Join(root, "track.mp3")
	known := map[string]KnownFile{
		knownPath: {ModTime: 1, SizeBytes: 2},
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{root}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk with missing root: got err %v, want nil", err)
	}
	if changed != nil {
		t.Errorf("changed: got %v, want nil for a skipped root", changed)
	}
	if deleted != nil {
		t.Errorf("deleted: got %v, want nil for a skipped root", deleted)
	}
}

// TestFilteredWalk_ExcludedKnownPathNotReportedAsDeleted verifies that a path
// previously in the DB (present in known) is NOT reported as deleted when its
// directory is excluded. Before the fix, excluded directories were silently
// skipped by the walker so their files were never added to seen, causing them
// to appear in the deleted slice and be marked missing on every incremental scan.
func TestFilteredWalk_ExcludedKnownPathNotReportedAsDeleted(t *testing.T) {
	root := t.TempDir()
	excluded := filepath.Join(root, "excluded")
	if err := os.MkdirAll(excluded, 0o755); err != nil {
		t.Fatalf("mkdir excluded: %v", err)
	}
	excludedTrack := filepath.Join(excluded, "track.mp3")
	mkfile(t, excludedTrack)

	included := filepath.Join(root, "included")
	if err := os.MkdirAll(included, 0o755); err != nil {
		t.Fatalf("mkdir included: %v", err)
	}
	includedTrack := filepath.Join(included, "track.mp3")
	mkfile(t, includedTrack)

	includedInfo, _ := os.Stat(includedTrack)
	excludedInfo, _ := os.Stat(excludedTrack)
	known := map[string]KnownFile{
		includedTrack: {ModTime: includedInfo.ModTime().Unix(), SizeBytes: includedInfo.Size()},
		excludedTrack: {ModTime: excludedInfo.ModTime().Unix(), SizeBytes: excludedInfo.Size()},
	}

	_, deleted, err := FilteredWalk(context.Background(), []string{root}, []string{excluded}, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}
	for _, d := range deleted {
		if d == excludedTrack {
			t.Errorf("excluded path %q must not appear in deleted", excludedTrack)
		}
	}
}

func TestFilteredWalk_NestedRootsDeduplicateChangedFiles(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "albums")
	audio := filepath.Join(shared, "track.mp3")
	mkfile(t, audio)

	changed, deleted, err := FilteredWalk(context.Background(), []string{root, shared}, nil, nil, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}
	if len(changed) != 1 || changed[0] != audio {
		t.Errorf("changed: got %v, want [%s]", changed, audio)
	}
	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
}
