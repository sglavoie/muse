package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A configured music directory that does not exist must not abort the walk:
// first-run users boot into an empty but usable library instead of an error.
func TestWalkAudioPaths_MissingRootIsSkipped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")

	var visited []string
	var onErrCalls int
	err := walkAudioPaths(context.Background(), missing, nil,
		func(path string) error { visited = append(visited, path); return nil },
		func(error) { onErrCalls++ },
	)
	if err == nil || !errors.Is(err, errRootSkipped) {
		t.Fatalf("walkAudioPaths err = %v, want an errRootSkipped wrapper", err)
	}
	if len(visited) != 0 {
		t.Errorf("visited = %v, want none", visited)
	}
	// onErr still fires so the TUI keeps surfacing the reason to the user.
	if onErrCalls != 1 {
		t.Errorf("onErr calls = %d, want 1", onErrCalls)
	}
}

// The deliberate half of the old root guard: a failure below the root stays
// non-fatal, reaches onErr, and does not stop the rest of the traversal.
func TestWalkAudioPaths_NonRootFailureDoesNotAbortWalk(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "keep.mp3"))
	locked := filepath.Join(root, "locked")
	mkfile(t, filepath.Join(locked, "hidden.mp3"))
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) //nolint:gosec // G302: restoring perms in test cleanup

	var visited []string
	var onErrCalls int
	err := walkAudioPaths(context.Background(), root, nil,
		func(path string) error { visited = append(visited, filepath.Base(path)); return nil },
		func(error) { onErrCalls++ },
	)
	if err != nil {
		t.Fatalf("walkAudioPaths err = %v, want nil for a non-root failure", err)
	}
	if errors.Is(err, errRootSkipped) {
		t.Fatal("non-root failure must not be reported as a skipped root")
	}
	if onErrCalls == 0 {
		t.Error("onErr never fired for the unreadable subdirectory")
	}
	if !slices.Contains(visited, "keep.mp3") {
		t.Errorf("visited = %v, want the sibling file to still be walked", visited)
	}
}

func TestWalk_MissingRootStillWalksRemainingRoots(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	good := t.TempDir()
	mkfile(t, filepath.Join(good, "a.mp3"))

	got := collectNames(Walk(context.Background(), []string{missing, good}, nil))
	if !slices.Equal(got, []string{"a.mp3"}) {
		t.Errorf("Walk = %v, want [a.mp3]", got)
	}
}

func TestCountAudioFiles_MissingRootCountsRemainingRoots(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	good := t.TempDir()
	mkfile(t, filepath.Join(good, "a.mp3"))

	if got := CountAudioFiles(context.Background(), []string{missing, good}, nil); got != 1 {
		t.Errorf("CountAudioFiles = %d, want 1", got)
	}
}

func TestFilteredWalk_MissingRootYieldsEmptyResultWithoutError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")

	var onErrCalls int
	changed, deleted, err := FilteredWalk(context.Background(), []string{missing}, nil, nil,
		func(error) { onErrCalls++ })
	if err != nil {
		t.Fatalf("FilteredWalk err = %v, want nil", err)
	}
	if len(changed) != 0 {
		t.Errorf("changed = %v, want none", changed)
	}
	if len(deleted) != 0 {
		t.Errorf("deleted = %v, want none", deleted)
	}
	if onErrCalls != 1 {
		t.Errorf("onErr calls = %d, want 1", onErrCalls)
	}
}

// One unreadable root must not suppress discovery in the others, but it still
// suppresses deletions for the whole walk (the view of disk is incomplete).
// A missing root means the cached files were never observed, not that they were
// deleted; reporting them would mark the whole library missing the first time a
// music directory is unmounted or renamed.
func TestFilteredWalk_MissingRootStillWalksRemainingRoots(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	good := t.TempDir()
	mkfile(t, filepath.Join(good, "a.mp3"))
	gone := filepath.Join(good, "gone.mp3")

	changed, deleted, err := FilteredWalk(context.Background(), []string{missing, good}, nil,
		map[string]KnownFile{gone: {ModTime: 1, SizeBytes: 2}}, nil)
	if err != nil {
		t.Fatalf("FilteredWalk err = %v, want nil", err)
	}
	if len(changed) != 1 || filepath.Base(changed[0]) != "a.mp3" {
		t.Errorf("changed = %v, want [a.mp3]", changed)
	}
	if deleted != nil {
		t.Errorf("deleted = %v, want nil while any root was skipped", deleted)
	}
}

func TestFilteredWalk_AllRootsReadableStillReportsDeletions(t *testing.T) {
	good := t.TempDir()
	mkfile(t, filepath.Join(good, "a.mp3"))
	gone := filepath.Join(good, "gone.mp3")

	_, deleted, err := FilteredWalk(context.Background(), []string{good}, nil,
		map[string]KnownFile{gone: {ModTime: 1, SizeBytes: 2}}, nil)
	if err != nil {
		t.Fatalf("FilteredWalk err = %v, want nil", err)
	}
	if !slices.Equal(deleted, []string{gone}) {
		t.Errorf("deleted = %v, want [%s]", deleted, gone)
	}
}
