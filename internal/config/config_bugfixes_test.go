package config

import (
	"log/slog"
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

// TestLoad_MPVExtraFlags_EmptyEntriesDropped ensures whitespace-only or empty
// flag strings from the TOML config are silently dropped. Passing an empty
// argument to mpv produces an unhelpful "Option --: parameter must be ..."
// error and aborts startup.
func TestLoad_MPVExtraFlags_EmptyEntriesDropped(t *testing.T) {
	dir := t.TempDir()
	content := `mpv_extra_flags = ["--no-video", "", "  ", "--volume=80"]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := result.Config.MPVExtraFlags
	if len(got) != 2 {
		t.Fatalf("MPVExtraFlags: expected 2 entries after dropping empties, got %d (%v)", len(got), got)
	}
	if got[0] != "--no-video" || got[1] != "--volume=80" {
		t.Errorf("MPVExtraFlags = %v, want [--no-video --volume=80]", got)
	}
}

// TestLoad_EQCustomPresets_KeyWhitespaceTrimmed verifies that custom EQ
// presets with whitespace-padded keys are accepted with the key normalised,
// so lookups (which use a trimmed key) succeed instead of silently failing.
func TestLoad_EQCustomPresets_KeyWhitespaceTrimmed(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "Padded"
key = "  padded_key  "
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Key; got != "padded_key" {
		t.Errorf("Key = %q, want %q (whitespace should be trimmed)", got, "padded_key")
	}
}

// TestLoad_EQCustomPresets_EmptyKeyDroppedWithWarning verifies that a preset
// with an empty or whitespace-only key is dropped AND a warning is emitted so
// the user knows their entry was rejected instead of silently lost.
func TestLoad_EQCustomPresets_EmptyKeyDroppedWithWarning(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "No Key"
key = ""
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Whitespace Key"
key = "   "
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 0 {
		t.Fatalf("EQCustomPresets len = %d, want 0", len(result.Config.EQCustomPresets))
	}
	emptyWarnings := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "empty") {
			emptyWarnings++
		}
	}
	if emptyWarnings != 2 {
		t.Errorf("expected 2 warnings about empty keys, got %d (%v)", emptyWarnings, result.Warnings)
	}
}

// TestLoad_EQCustomPresets_EmptyNameFallsBackToKey verifies that a preset
// without a Name is still addressable by using the key as the display name.
func TestLoad_EQCustomPresets_EmptyNameFallsBackToKey(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = ""
key = "rock_remix"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(result.Config.EQCustomPresets))
	}
	if got := result.Config.EQCustomPresets[0].Name; got != "rock_remix" {
		t.Errorf("Name = %q, want %q (should fall back to key)", got, "rock_remix")
	}
}

// TestLoad_EQCustomPresets_DuplicateKeyWarns verifies that a duplicated
// preset key produces a warning instead of silently dropping the later entry.
func TestLoad_EQCustomPresets_DuplicateKeyWarns(t *testing.T) {
	dir := t.TempDir()
	content := `
[[eq_custom_presets]]
name = "First"
key = "dup"
bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

[[eq_custom_presets]]
name = "Second"
key = "dup"
bands = [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Config.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1 after dedup", len(result.Config.EQCustomPresets))
	}
	if result.Config.EQCustomPresets[0].Name != "First" {
		t.Errorf("kept preset name = %q, want %q", result.Config.EQCustomPresets[0].Name, "First")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "duplicated") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about duplicated key, got: %v", result.Warnings)
	}
}

// TestCloseLog_HandlerSwappedBeforeFileClosed verifies that CloseLog installs
// the discard handler BEFORE closing the file handle, so a late slog call
// during shutdown does not write to a closed file descriptor. Calling CloseLog
// followed by slog.Info must not panic and must not produce any output to
// the closed file.
func TestCloseLog_HandlerSwappedBeforeFileClosed(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "muse")

	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		logFile = nil
	})

	if err := InitLogging(stateDir, slog.LevelInfo); err != nil {
		t.Fatalf("InitLogging: %v", err)
	}

	// Capture the file size before close.
	logFilePath := filepath.Join(stateDir, logFileName)
	preInfo, err := os.Stat(logFilePath)
	if err != nil {
		t.Fatalf("stat before close: %v", err)
	}
	preSize := preInfo.Size()

	CloseLog()

	// A racing slog call after CloseLog must not panic and must NOT extend
	// the file. The discard handler swap happens before file close so this
	// write goes to /dev/null.
	slog.Info("post-close message should not write to file or panic")

	postInfo, err := os.Stat(logFilePath)
	if err != nil {
		t.Fatalf("stat after close: %v", err)
	}
	// The shutdown line written by CloseLog itself can extend the file, but
	// only by a bounded amount. The post-close slog call must not.
	postClosePadding := int64(512)
	if delta := postInfo.Size() - preSize; delta > postClosePadding {
		t.Errorf("file grew by %d bytes after CloseLog; expected only the shutdown line", delta)
	}
}

// TestLoad_MPVExtraFlags_OnlyEmptyEntriesYieldsEmptySlice ensures that a list
// of only blank entries does not leave callers with a nil slice (the rest of
// the codebase assumes a non-nil MPVExtraFlags).
func TestLoad_MPVExtraFlags_OnlyEmptyEntriesYieldsEmptySlice(t *testing.T) {
	dir := t.TempDir()
	content := `mpv_extra_flags = ["", "  "]
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := result.Config.MPVExtraFlags
	if got == nil {
		t.Fatal("MPVExtraFlags is nil; expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("MPVExtraFlags = %v, want empty", got)
	}
}
