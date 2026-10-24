package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// symlinkedRoot creates a real music directory holding two tracks (one in a
// subdirectory) and returns a symlink to it, as a configured music directory.
func symlinkedRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	mkfile(t, filepath.Join(real, "a.mp3"))
	mkfile(t, filepath.Join(real, "album", "b.flac"))
	link := filepath.Join(dir, "Music")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	return link
}

// A music directory configured as a symlink is walked, and every path is
// reported under the configured spelling so it matches the stored tracks.
func TestWalk_SymlinkedRootIsFollowed(t *testing.T) {
	link := symlinkedRoot(t)

	var got []string
	for p := range Walk(context.Background(), []string{link}, nil) {
		got = append(got, p)
	}
	slices.Sort(got)
	want := []string{filepath.Join(link, "a.mp3"), filepath.Join(link, "album", "b.flac")}
	if !slices.Equal(got, want) {
		t.Fatalf("Walk(symlinked root) = %v, want %v", got, want)
	}
	if n := CountAudioFiles(context.Background(), []string{link}, nil); n != 2 {
		t.Errorf("CountAudioFiles(symlinked root) = %d, want 2", n)
	}
}

// Before the fix the symlinked root yielded nothing, so every known track
// under it was reported deleted on each launch.
func TestFilteredWalk_SymlinkedRootReportsNoDeletions(t *testing.T) {
	link := symlinkedRoot(t)
	known := make(map[string]KnownFile)
	for _, p := range []string{filepath.Join(link, "a.mp3"), filepath.Join(link, "album", "b.flac")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{link}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}
	if len(changed) != 0 || len(deleted) != 0 {
		t.Fatalf("changed=%v deleted=%v, want both empty", changed, deleted)
	}
}

// Exclusion patterns written against the configured root keep applying.
func TestWalk_SymlinkedRootHonoursExclusionsUnderConfiguredPath(t *testing.T) {
	link := symlinkedRoot(t)
	got := collectNames(Walk(context.Background(), []string{link}, []string{filepath.Join(link, "album")}))
	if !slices.Equal(got, []string{"a.mp3"}) {
		t.Fatalf("Walk with exclusion = %v, want [a.mp3]", got)
	}
}

// A dangling root symlink is an unreadable root, not an empty one, so its
// known tracks are not reported deleted.
func TestFilteredWalk_DanglingRootSymlinkIsSkipped(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "Music")
	if err := os.Symlink(filepath.Join(dir, "unmounted"), link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	err := walkAudioPaths(context.Background(), link, nil, func(string) error { return nil }, nil)
	if !errors.Is(err, errRootSkipped) {
		t.Fatalf("walkAudioPaths err = %v, want errRootSkipped", err)
	}
	known := map[string]KnownFile{filepath.Join(link, "a.mp3"): {ModTime: 1, SizeBytes: 1}}
	_, deleted, err := FilteredWalk(context.Background(), []string{link}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted = %v, want none for a dangling root", deleted)
	}
}

func TestAddWatchesRecursive_SymlinkedRootRegistersSubdirs(t *testing.T) {
	link := symlinkedRoot(t)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := addWatchesRecursive(context.Background(), w, link, nil); err != nil {
		t.Fatalf("addWatchesRecursive: %v", err)
	}
	if watched := w.WatchList(); !slices.Contains(watched, filepath.Join(link, "album")) {
		t.Fatalf("WatchList = %v, want the subdirectory under the configured root", watched)
	}
}

// Changes inside a symlinked root reach the watcher under the configured path.
func TestWatch_SymlinkedRootReportsConfiguredPaths(t *testing.T) {
	link := symlinkedRoot(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	ch, degraded, err := Watch(ctx, []string{link}, nil, &fakeExtractor{}, 100*time.Millisecond)
	if err != nil || degraded {
		t.Fatalf("Watch: degraded=%v err=%v", degraded, err)
	}
	want := map[string]bool{
		filepath.Join(link, "top.mp3"):          true,
		filepath.Join(link, "album", "new.mp3"): true,
	}
	for p := range want {
		mkfile(t, p)
	}
	for len(want) > 0 {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("event channel closed before the changes were reported")
			}
			if !e.Deleted {
				delete(want, e.Track.Path)
			}
		case <-ctx.Done():
			t.Fatalf("no event under the symlinked root for %v", want)
		}
	}
}
