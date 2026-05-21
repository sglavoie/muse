package library

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/sglavoie/muse/internal/audio"
)

// mkfile creates a file (and any parent directories) with empty content.
func mkfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// collect drains the channel and returns a sorted slice of base names.
func collectNames(ch <-chan string) []string {
	var names []string
	for p := range ch {
		names = append(names, filepath.Base(p))
	}
	sort.Strings(names)
	return names
}

func TestWalk_CaseInsensitiveExtensions(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "track.MP3"))
	mkfile(t, filepath.Join(dir, "album.Flac"))
	mkfile(t, filepath.Join(dir, "song.OPUS"))
	mkfile(t, filepath.Join(dir, "short.AIF"))
	mkfile(t, filepath.Join(dir, "cover.jpg")) // not audio

	got := collectNames(Walk(context.Background(), []string{dir}, nil))
	want := []string{"album.Flac", "short.AIF", "song.OPUS", "track.MP3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWalk_SkipsHiddenFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "visible.mp3"))
	mkfile(t, filepath.Join(dir, ".hidden.mp3"))
	mkfile(t, filepath.Join(dir, ".hiddendir", "inside.mp3"))

	got := collectNames(Walk(context.Background(), []string{dir}, nil))
	want := []string{"visible.mp3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWalk_NonAudioFilesDropped(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "track.mp3"))
	mkfile(t, filepath.Join(dir, "cover.jpg"))
	mkfile(t, filepath.Join(dir, "notes.txt"))

	got := collectNames(Walk(context.Background(), []string{dir}, nil))
	want := []string{"track.mp3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWalk_FileSymlinksFollowed(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.mp3")
	mkfile(t, real)
	link := filepath.Join(dir, "link.mp3")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	got := collectNames(Walk(context.Background(), []string{dir}, nil))
	// Both the real file and the symlink should appear.
	if !slices.Contains(got, "real.mp3") {
		t.Errorf("real file missing: %v", got)
	}
	if !slices.Contains(got, "link.mp3") {
		t.Errorf("file symlink not followed: %v", got)
	}
}

func TestWalk_DirSymlinksNotFollowed(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "music")
	mkfile(t, filepath.Join(subdir, "track.mp3"))

	link := filepath.Join(dir, "linked")
	if err := os.Symlink(subdir, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	got := collectNames(Walk(context.Background(), []string{dir}, nil))
	// track.mp3 appears once (via subdir), not twice.
	count := 0
	for _, n := range got {
		if n == "track.mp3" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected track.mp3 exactly once, got %d times in %v", count, got)
	}
}

// --- isAudioFile ---

func TestIsAudioFile_SupportedExtensions(t *testing.T) {
	for ext := range audio.Extensions {
		name := "track" + ext
		if !isAudioFile(name) {
			t.Errorf("isAudioFile(%q) = false, want true", name)
		}
	}
}

func TestIsAudioFile_CaseInsensitive(t *testing.T) {
	cases := []string{"track.MP3", "song.Flac", "file.OGG", "audio.OPUS"}
	for _, name := range cases {
		if !isAudioFile(name) {
			t.Errorf("isAudioFile(%q) = false, want true (case-insensitive)", name)
		}
	}
}

func TestIsAudioFile_NonAudio(t *testing.T) {
	cases := []string{"photo.jpg", "doc.pdf", "data.txt", "script.py", "noext", ""}
	for _, name := range cases {
		if isAudioFile(name) {
			t.Errorf("isAudioFile(%q) = true, want false", name)
		}
	}
}

func TestIsAudioFile_HiddenAudioFile(t *testing.T) {
	// isAudioFile only checks the extension; hidden-file filtering is done by Walk.
	if !isAudioFile(".mp3") {
		t.Error("isAudioFile(.mp3) should be true — extension is .mp3")
	}
}

// --- CountAudioFiles ---

func TestCountAudioFiles(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "a.mp3"))
	mkfile(t, filepath.Join(dir, "b.flac"))
	mkfile(t, filepath.Join(dir, "c.txt"))
	mkfile(t, filepath.Join(dir, "sub", "d.ogg"))
	mkfile(t, filepath.Join(dir, ".hidden.mp3"))

	got := CountAudioFiles(context.Background(), []string{dir}, nil)
	// a.mp3, b.flac, sub/d.ogg = 3 (hidden file excluded by walk)
	if got != 3 {
		t.Errorf("CountAudioFiles = %d, want 3", got)
	}
}

func TestCountAudioFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got := CountAudioFiles(context.Background(), []string{dir}, nil)
	if got != 0 {
		t.Errorf("CountAudioFiles empty dir = %d, want 0", got)
	}
}

func TestWalk_ChannelClosedOnCompletion(t *testing.T) {
	dir := t.TempDir()
	ch := Walk(context.Background(), []string{dir}, nil)
	// Drain and confirm the channel is closed (range completes).
	for range ch {
	}
	// If we reach here the channel was closed cleanly.
}

// TestWalk_MultipleRootsMergesResults verifies that Walk over two directories
// returns files from both roots.
func TestWalk_MultipleRootsMergesResults(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	mkfile(t, filepath.Join(dir1, "track_a.mp3"))
	mkfile(t, filepath.Join(dir1, "track_b.flac"))
	mkfile(t, filepath.Join(dir2, "track_c.ogg"))

	got := collectNames(Walk(context.Background(), []string{dir1, dir2}, nil))
	want := []string{"track_a.mp3", "track_b.flac", "track_c.ogg"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWalk_NestedRootsDeduplicateFiles(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "albums")
	mkfile(t, filepath.Join(shared, "track.mp3"))

	got := collectNames(Walk(context.Background(), []string{root, shared}, nil))
	want := []string{"track.mp3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestCountAudioFiles_SumsAcrossMultipleRoots verifies that CountAudioFiles
// sums counts across all provided root directories.
func TestCountAudioFiles_SumsAcrossMultipleRoots(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	mkfile(t, filepath.Join(dir1, "a.mp3"))
	mkfile(t, filepath.Join(dir1, "b.flac"))
	mkfile(t, filepath.Join(dir2, "c.ogg"))
	mkfile(t, filepath.Join(dir2, "d.opus"))

	got := CountAudioFiles(context.Background(), []string{dir1, dir2}, nil)
	if got != 4 {
		t.Errorf("CountAudioFiles two dirs = %d, want 4", got)
	}
}

func TestCountAudioFiles_NestedRootsDeduplicateFiles(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "albums")
	mkfile(t, filepath.Join(shared, "a.mp3"))
	mkfile(t, filepath.Join(shared, "b.flac"))

	got := CountAudioFiles(context.Background(), []string{root, shared}, nil)
	if got != 2 {
		t.Errorf("CountAudioFiles nested roots = %d, want 2", got)
	}
}

// --- Exclusion tests ---

// TestWalk_ExcludedDirSkipsEntireSubtree verifies that Walk skips all files
// inside a directory whose name matches an excluded pattern.
func TestWalk_ExcludedDirSkipsEntireSubtree(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "Podcasts", "ep1.mp3"))
	mkfile(t, filepath.Join(dir, "Podcasts", "sub", "ep2.mp3"))
	mkfile(t, filepath.Join(dir, "Music", "track.mp3"))

	got := collectNames(Walk(context.Background(), []string{dir}, []string{"Podcasts"}))
	want := []string{"track.mp3"}
	if !slices.Equal(got, want) {
		t.Errorf("Walk with exclusion: got %v, want %v", got, want)
	}
}

// TestWalk_ExclusionLeavesNonExcludedSiblings verifies that sibling
// directories that don't match the exclusion pattern are still traversed.
func TestWalk_ExclusionLeavesNonExcludedSiblings(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "Podcasts", "ep.mp3"))
	mkfile(t, filepath.Join(dir, "Audiobooks", "chapter.flac"))
	mkfile(t, filepath.Join(dir, "Music", "rock.ogg"))
	mkfile(t, filepath.Join(dir, "Music", "jazz.flac"))

	got := collectNames(Walk(context.Background(), []string{dir}, []string{"Podcasts", "Audiobooks"}))
	want := []string{"jazz.flac", "rock.ogg"}
	if !slices.Equal(got, want) {
		t.Errorf("Walk multi-exclude: got %v, want %v", got, want)
	}
}

// TestCountAudioFiles_WithExclusion verifies that excluded directories are not
// counted.
func TestCountAudioFiles_WithExclusion(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "Podcasts", "ep1.mp3"))
	mkfile(t, filepath.Join(dir, "Podcasts", "ep2.mp3"))
	mkfile(t, filepath.Join(dir, "Music", "track.mp3"))

	got := CountAudioFiles(context.Background(), []string{dir}, []string{"Podcasts"})
	if got != 1 {
		t.Errorf("CountAudioFiles with exclusion = %d, want 1", got)
	}
}
