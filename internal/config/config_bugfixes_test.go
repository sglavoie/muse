package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_MusicDirectories_TrailingSlashCleaned ensures that a configured
// music directory keeps its canonical form (no trailing slash) so that string
// comparisons such as slices.Equal between the saved roots and a freshly
// loaded value are stable.
func TestLoad_MusicDirectories_TrailingSlashCleaned(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["/home/user/Music/"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry, got %v", result.Config.MusicDirectories)
	}
	if result.Config.MusicDirectories[0] != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want %q (trailing slash should be cleaned)",
			result.Config.MusicDirectories[0], "/home/user/Music")
	}
	if result.Config.MusicDirectory != "/home/user/Music" {
		t.Errorf("MusicDirectory = %q, want %q (legacy mirror should also be cleaned)",
			result.Config.MusicDirectory, "/home/user/Music")
	}
}

// TestLoad_MusicDirectories_WhitespaceTrimmed ensures that user typos with
// surrounding whitespace in TOML entries don't produce roots that fail to
// match the actual on-disk path.
func TestLoad_MusicDirectories_WhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["  /home/user/Music  "]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry, got %v", result.Config.MusicDirectories)
	}
	got := result.Config.MusicDirectories[0]
	if strings.ContainsAny(got, " \t") {
		t.Errorf("MusicDirectories[0] = %q contains whitespace; expected trimmed", got)
	}
	if got != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want %q", got, "/home/user/Music")
	}
}

// TestLoad_MusicDirectories_EmptyEntryDropped checks that whitespace-only or
// empty TOML entries are silently dropped instead of being treated as the
// current working directory (which filepath.Clean would turn "" into ".").
func TestLoad_MusicDirectories_EmptyEntryDropped(t *testing.T) {
	dir := t.TempDir()
	content := `music_directories = ["/home/user/Music", "   ", ""]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Config.MusicDirectories) != 1 {
		t.Fatalf("MusicDirectories: expected 1 entry after dropping empties, got %v", result.Config.MusicDirectories)
	}
	if result.Config.MusicDirectories[0] != "/home/user/Music" {
		t.Errorf("MusicDirectories[0] = %q, want /home/user/Music", result.Config.MusicDirectories[0])
	}
}

// TestLoad_MPVSocketPath_WhitespaceTrimmed ensures the mpv IPC socket path
// gets whitespace stripped: an untrimmed path could not be opened by mpv.
func TestLoad_MPVSocketPath_WhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	content := `mpv_socket_path = "  /tmp/muse.sock  "
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.Config.MPVSocketPath != "/tmp/muse.sock" {
		t.Errorf("MPVSocketPath = %q, want %q (whitespace should be trimmed)",
			result.Config.MPVSocketPath, "/tmp/muse.sock")
	}
}

// TestSave_PreservesPreviousFileOnDirectoryRemoval verifies the atomicity
// property by holding an open handle to the original config and checking
// that the data accessible via that handle is unchanged after a subsequent
// Save. With a non-atomic in-place truncate, an open reader would see the
// file shrink to zero bytes; with rename semantics the original inode is
// untouched and the open handle keeps the original content.
func TestSave_PreservesPreviousFileOnDirectoryRemoval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)

	cfg := defaults()
	cfg.DefaultVolume = 42
	if err := Save(path, cfg); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	// Open the original file before the second Save.
	pre, err := os.Open(path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer pre.Close()
	preInfo, err := pre.Stat()
	if err != nil {
		t.Fatalf("stat original: %v", err)
	}
	originalSize := preInfo.Size()
	if originalSize == 0 {
		t.Fatalf("setup: original file is empty")
	}

	cfg.DefaultVolume = 7
	if err := Save(path, cfg); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	// The descriptor we held open before must still report the original
	// size. With an in-place truncate the size would now be 0 (or the size
	// of the new content via the same inode).
	post, err := pre.Stat()
	if err != nil {
		t.Fatalf("stat via open fd after second save: %v", err)
	}
	if post.Size() != originalSize {
		t.Errorf("open fd size = %d, want unchanged %d (Save should not truncate the original inode)",
			post.Size(), originalSize)
	}

	// No stray *.tmp-* files should be left behind after a successful Save.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after successful save: %q", e.Name())
		}
	}
}

// TestSave_FsyncsParentDirectory exercises the post-rename directory fsync
// path. The behaviour is best-effort, so the test verifies Save still
// succeeds and the rename is observable; it cannot directly assert that the
// fsync system call was issued without OS-level hooks. Combined with the
// roundtrip and atomicity tests this guards against the durability fix
// regressing into a code path that errors out on common filesystems.
func TestSave_FsyncsParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)

	cfg := defaults()
	cfg.DefaultVolume = 11
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must exist after rename.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat after Save: %v", err)
	}

	// No stray temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after Save: %q", e.Name())
		}
	}

	// Round-trip the value to confirm the rename completed cleanly even when
	// the directory fsync ran.
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result.Config.DefaultVolume != 11 {
		t.Errorf("DefaultVolume = %d, want 11", result.Config.DefaultVolume)
	}
}

// TestSave_AtomicRoundTrip is a sanity check that the atomic Save still
// produces a file the loader can read back.
func TestSave_AtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := defaults()
	cfg.DefaultVolume = 33
	cfg.LogLevel = "warn"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// No leftover temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after successful save: %q", e.Name())
		}
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result.Config.DefaultVolume != 33 {
		t.Errorf("DefaultVolume = %d, want 33", result.Config.DefaultVolume)
	}
	if result.Config.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want warn", result.Config.LogLevel)
	}
}
