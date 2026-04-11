package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDebounce_CoalescesFastEvents verifies that many events arriving within
// the debounce window produce at most one rescan batch (deduplicated pending
// set), not one Extract call per raw event.
func TestDebounce_CoalescesFastEvents(t *testing.T) {
	root := t.TempDir()
	const debounce = 200 * time.Millisecond

	paths := []string{
		filepath.Join(root, "a.mp3"),
		filepath.Join(root, "b.flac"),
		filepath.Join(root, "c.ogg"),
	}
	for _, p := range paths {
		mkfile(t, p)
	}

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	for i := range 10 {
		p := paths[i%len(paths)]
		if err := os.WriteFile(p, []byte("ID3upd"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+500*time.Millisecond)
	defer drainCancel()
	collectTracks(drainCtx, ch)
	cancel()

	totalCalls := 0
	for _, batch := range fe.batches {
		totalCalls += len(batch)
	}
	if totalCalls > len(paths) {
		t.Errorf("Extract called for %d file-invocations; want ≤ %d (rapid events coalesced)",
			totalCalls, len(paths))
	}
}

// TestDebounce_RescansAfterWindow verifies that the rescan fires after the
// debounce window has elapsed without further events.
func TestDebounce_RescansAfterWindow(t *testing.T) {
	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	p := filepath.Join(root, "track.mp3")
	mkfile(t, p)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := os.WriteFile(p, []byte("ID3v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.After(debounce + 500*time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if ok {
				return
			}
		case <-deadline:
			t.Fatal("no event received within debounce window + margin")
		}
	}
}

// TestDebounce_NoRescanBeforeWindowElapses verifies that no rescan fires while
// events are still arriving within the debounce window.
func TestDebounce_NoRescanBeforeWindowElapses(t *testing.T) {
	root := t.TempDir()
	const debounce = 300 * time.Millisecond

	p := filepath.Join(root, "track.mp3")
	mkfile(t, p)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := os.WriteFile(p, []byte("ID3v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	earlyDeadline := time.After(debounce / 2)
	select {
	case e, ok := <-ch:
		if ok && !e.Deleted {
			t.Error("rescan fired before debounce window elapsed")
		}
	case <-earlyDeadline:
	}
}

// TestRename_TreatedAsDeletePlusCreate verifies that renaming a file causes the
// old path to be dropped from pending while the new path drives the rescan.
func TestRename_TreatedAsDeletePlusCreate(t *testing.T) {
	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	oldPath := filepath.Join(root, "old.mp3")
	newPath := filepath.Join(root, "new.mp3")
	mkfile(t, oldPath)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+500*time.Millisecond)
	defer drainCancel()
	collectTracks(drainCtx, ch)
	cancel()

	for _, batch := range fe.batches {
		for _, hf := range batch {
			if hf.Path == oldPath {
				t.Errorf("Extract called for deleted path %q; rename should treat old path as delete", oldPath)
			}
		}
	}
}

// TestDirectoryEvent_TriggersSubtreeRescan verifies that creating a new
// directory with audio files causes Walk to traverse the subtree.
func TestDirectoryEvent_TriggersSubtreeRescan(t *testing.T) {
	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	subdir := filepath.Join(root, "newalbum")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mkfile(t, filepath.Join(subdir, "track01.flac"))
	mkfile(t, filepath.Join(subdir, "track02.flac"))

	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+600*time.Millisecond)
	defer drainCancel()
	tracks := collectTracks(drainCtx, ch)
	cancel()

	if len(tracks) < 2 {
		t.Errorf("directory event: expected ≥ 2 tracks from subtree rescan, got %d", len(tracks))
	}
}
