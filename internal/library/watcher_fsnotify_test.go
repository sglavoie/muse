package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TestHandleFsEvent_PendingSetBehavior tests handleFsEvent directly using
// synthetic fsnotify.Event values to verify the pending-set logic in isolation.
func TestHandleFsEvent_PendingSetBehavior(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "song.mp3")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	t.Run("Create audio file adds to pending", func(t *testing.T) {
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Create}, nil, nil, pending, deleted, w)
		isDir, ok := pending[audioPath]
		if !ok {
			t.Errorf("expected %q in pending after Create event", audioPath)
		}
		if isDir {
			t.Errorf("audio file should have isDir=false in pending")
		}
	})

	t.Run("Write audio file adds to pending", func(t *testing.T) {
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Write}, nil, nil, pending, deleted, w)
		if _, ok := pending[audioPath]; !ok {
			t.Errorf("expected %q in pending after Write event", audioPath)
		}
	})

	t.Run("Rename removes from pending and adds to deleted", func(t *testing.T) {
		pending := map[string]bool{audioPath: false}
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Rename}, nil, nil, pending, deleted, w)
		if _, ok := pending[audioPath]; ok {
			t.Errorf("expected %q removed from pending after Rename event", audioPath)
		}
		if _, ok := deleted[audioPath]; !ok {
			t.Errorf("expected %q in deleted after Rename event", audioPath)
		}
	})

	t.Run("Remove removes from pending and adds to deleted", func(t *testing.T) {
		pending := map[string]bool{audioPath: false}
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Remove}, nil, nil, pending, deleted, w)
		if _, ok := pending[audioPath]; ok {
			t.Errorf("expected %q removed from pending after Remove event", audioPath)
		}
		if _, ok := deleted[audioPath]; !ok {
			t.Errorf("expected %q in deleted after Remove event", audioPath)
		}
	})

	// A moved or trashed directory yields one event for the directory alone;
	// it must still be recorded so the tracks below it can be evicted.
	t.Run("Directory Rename and Remove add to deleted", func(t *testing.T) {
		dirPath := filepath.Join(root, "Album")
		for _, op := range []fsnotify.Op{fsnotify.Rename, fsnotify.Remove} {
			pending := map[string]bool{dirPath: true}
			deleted := make(map[string]struct{})
			handleFsEvent(context.Background(), fsnotify.Event{Name: dirPath, Op: op}, nil, nil, pending, deleted, w)
			if _, ok := deleted[dirPath]; !ok {
				t.Errorf("%s: expected %q in deleted", op, dirPath)
			}
			if _, ok := pending[dirPath]; ok {
				t.Errorf("%s: expected %q removed from pending", op, dirPath)
			}
		}
	})

	t.Run("Non-audio Create is ignored", func(t *testing.T) {
		txtPath := filepath.Join(root, "readme.txt")
		mkfile(t, txtPath)
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: txtPath, Op: fsnotify.Create}, nil, nil, pending, deleted, w)
		if _, ok := pending[txtPath]; ok {
			t.Errorf("non-audio file should not be added to pending")
		}
	})

	t.Run("Create directory adds as dir and registers watch", func(t *testing.T) {
		dirPath := filepath.Join(root, "newdir")
		if err := os.Mkdir(dirPath, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: dirPath, Op: fsnotify.Create}, nil, nil, pending, deleted, w)
		isDir, ok := pending[dirPath]
		if !ok {
			t.Errorf("expected %q in pending after directory Create", dirPath)
		}
		if !isDir {
			t.Errorf("directory pending entry should have isDir=true")
		}
	})
}

// TestHandleFsEvent_SkipsHiddenFiles verifies that handleFsEvent ignores
// events for hidden files and directories.
func TestHandleFsEvent_SkipsHiddenFiles(t *testing.T) {
	root := t.TempDir()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	t.Run("hidden audio file Create is ignored", func(t *testing.T) {
		hiddenAudio := filepath.Join(root, ".hidden.mp3")
		mkfile(t, hiddenAudio)
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: hiddenAudio, Op: fsnotify.Create}, nil, nil, pending, deleted, w)
		if _, ok := pending[hiddenAudio]; ok {
			t.Errorf("hidden audio file should not be added to pending")
		}
	})

	t.Run("hidden directory Create is ignored", func(t *testing.T) {
		hiddenDir := filepath.Join(root, ".hidden_dir")
		if err := os.Mkdir(hiddenDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: hiddenDir, Op: fsnotify.Create}, nil, nil, pending, deleted, w)
		if _, ok := pending[hiddenDir]; ok {
			t.Errorf("hidden directory should not be added to pending")
		}
	})

	t.Run("hidden file Write is ignored", func(t *testing.T) {
		hiddenAudio := filepath.Join(root, ".hidden.flac")
		mkfile(t, hiddenAudio)
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: hiddenAudio, Op: fsnotify.Write}, nil, nil, pending, deleted, w)
		if _, ok := pending[hiddenAudio]; ok {
			t.Errorf("hidden file Write should not be added to pending")
		}
	})

	t.Run("hidden file Remove is ignored", func(t *testing.T) {
		hiddenAudio := filepath.Join(root, ".hidden.mp3")
		pending := make(map[string]bool)
		deleted := make(map[string]struct{})
		handleFsEvent(context.Background(), fsnotify.Event{Name: hiddenAudio, Op: fsnotify.Remove}, nil, nil, pending, deleted, w)
		if _, ok := deleted[hiddenAudio]; ok {
			t.Errorf("hidden file Remove should not be added to deleted")
		}
	})
}

// TestHandleFsEvent_CreateAfterRemoveClearsDeleted verifies that a Create
// event for a path that was previously Removed/Renamed within the same
// debounce window clears the path from the deleted set. Without this
// behaviour, the TUI would briefly receive a Deleted=true event for a file
// that still exists (e.g. atomic-save dance, rapid rename-back), causing a
// visible flicker until the rescan re-adds the track.
func TestHandleFsEvent_CreateAfterRemoveClearsDeleted(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "song.mp3")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	pending := make(map[string]bool)
	deleted := make(map[string]struct{})

	// Simulate: file removed, then immediately re-created in the same window.
	handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Remove}, nil, nil, pending, deleted, w)
	if _, ok := deleted[audioPath]; !ok {
		t.Fatalf("precondition: expected %q in deleted after Remove", audioPath)
	}
	handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Create}, nil, nil, pending, deleted, w)

	if _, ok := deleted[audioPath]; ok {
		t.Errorf("Create after Remove must clear path from deleted; still present: %v", deleted)
	}
	if _, ok := pending[audioPath]; !ok {
		t.Errorf("Create after Remove must add path to pending")
	}
}

// TestHandleFsEvent_WriteAfterRemoveClearsDeleted is the Write variant of the
// same race: some editors emit Remove followed by Write+Chmod (e.g. via
// rewrite-in-place). The file still exists; we must not propagate a
// deletion message.
func TestHandleFsEvent_WriteAfterRemoveClearsDeleted(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "song.flac")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	pending := make(map[string]bool)
	deleted := make(map[string]struct{})

	handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Rename}, nil, nil, pending, deleted, w)
	handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Write}, nil, nil, pending, deleted, w)

	if _, ok := deleted[audioPath]; ok {
		t.Errorf("Write after Rename must clear path from deleted; still present: %v", deleted)
	}
	if _, ok := pending[audioPath]; !ok {
		t.Errorf("Write after Rename must add path to pending")
	}
}

// TestHandleFsEvent_ChmodAfterActualRemoveKeepsDeleted verifies that a
// trailing metadata event for a path that is already gone does not erase the
// deletion marker. macOS-style event streams can include Chmod after Remove;
// clearing deleted in that case leaves the stale track in the in-memory index.
func TestHandleFsEvent_ChmodAfterActualRemoveKeepsDeleted(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "song.mp3")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	pending := make(map[string]bool)
	deleted := make(map[string]struct{})

	if err := os.Remove(audioPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Remove}, nil, nil, pending, deleted, w)
	handleFsEvent(context.Background(), fsnotify.Event{Name: audioPath, Op: fsnotify.Chmod}, nil, nil, pending, deleted, w)

	if _, ok := deleted[audioPath]; !ok {
		t.Errorf("Chmod after actual Remove must keep path in deleted")
	}
	if _, ok := pending[audioPath]; ok {
		t.Errorf("Chmod after actual Remove must not add missing path to pending")
	}
}

// TestAddWatchesRecursive_SkipsHiddenDirs verifies that addWatchesRecursive
// does not register watches on hidden directories.
func TestAddWatchesRecursive_SkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "music")
	hidden := filepath.Join(root, ".git")
	if err := os.Mkdir(visible, 0o755); err != nil {
		t.Fatalf("mkdir visible: %v", err)
	}
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatalf("mkdir hidden: %v", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := addWatchesRecursive(context.Background(), w, root, nil); err != nil {
		t.Fatalf("addWatchesRecursive: %v", err)
	}

	watched := w.WatchList()
	watchedSet := make(map[string]bool, len(watched))
	for _, p := range watched {
		watchedSet[p] = true
	}

	if !watchedSet[root] {
		t.Errorf("expected root %q to be watched", root)
	}
	if !watchedSet[visible] {
		t.Errorf("expected visible dir %q to be watched", visible)
	}
	if watchedSet[hidden] {
		t.Errorf("hidden dir %q should NOT be watched", hidden)
	}
}

func TestAddWatchesRecursive_MissingRootReturnsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := addWatchesRecursive(context.Background(), w, root, nil); err == nil {
		t.Fatalf("addWatchesRecursive(%q): expected error for missing root", root)
	}
}

// With no configured roots there is nothing to register, so the watcher starts
// normally rather than degrading; the TUI reports it as available and a later
// config edit plus manual rescan is all a first-run user needs.
func TestWatch_NoRootsStartsUndegraded(t *testing.T) {
	ch, degraded, err := Watch(t.Context(), nil, nil, &fakeExtractor{}, DefaultDebounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if degraded {
		t.Fatal("Watch with no roots: expected degraded=false")
	}
	if ch == nil {
		t.Fatal("Watch with no roots: expected a live event channel")
	}
}

func TestWatch_MissingRootDegradesToClosedChannel(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	ch, degraded, err := Watch(t.Context(), []string{root}, nil, &fakeExtractor{}, DefaultDebounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if !degraded {
		t.Fatal("Watch with missing root: expected degraded=true")
	}
	if _, ok := <-ch; ok {
		t.Fatal("Watch with missing root: expected pre-closed event channel")
	}
}

// An unreachable root (e.g. an unplugged external drive) must not disable live
// watching for the roots that are still there: the missing root is skipped and
// changes under the good root keep producing events.
func TestWatch_MissingRootDoesNotDisableOtherRoots(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "unplugged")
	good := t.TempDir()
	const debounce = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	ch, degraded, err := Watch(ctx, []string{missing, good}, nil, &fakeExtractor{}, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if degraded {
		t.Fatal("Watch with one missing and one good root: expected degraded=false")
	}

	p := filepath.Join(good, "new.mp3")
	mkfile(t, p)

	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("event channel closed before the good root reported a change")
			}
			if !e.Deleted && e.Track.Path == p {
				return
			}
		case <-ctx.Done():
			t.Fatalf("no event for %q under the good root", p)
		}
	}
}
