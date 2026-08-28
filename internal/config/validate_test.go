package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertActionable checks the shared guidance block every music-directory
// validation error must carry: the config file path, the TOML key to change,
// and a concrete example value.
func assertActionable(t *testing.T, err error, configPath string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, configPath) {
		t.Errorf("error should contain config path %q, got: %s", configPath, msg)
	}
	if !strings.Contains(msg, "music_directories") {
		t.Errorf("error should name the music_directories TOML key, got: %s", msg)
	}
	if !strings.Contains(msg, `music_directories = ["`+exampleMusicDirectory+`"]`) {
		t.Errorf("error should show a concrete music_directories example, got: %s", msg)
	}
}

func TestValidateMusicDir_MissingDirectory(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	missingDir := filepath.Join(tmp, "no-such-dir")

	err := ValidateMusicDirs([]string{missingDir}, configPath)
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %s", err.Error())
	}
}

func TestValidateMusicDirs_NoDirectoriesConfigured(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	err := ValidateMusicDirs(nil, configPath)
	if err == nil {
		t.Fatal("expected error for missing music directories, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "no music directories configured") {
		t.Errorf("error should mention missing configured directories, got: %s", err.Error())
	}
}

func TestValidateMusicDirs_BlankEntriesIgnored(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "song.mp3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMusicDirs([]string{"  ", musicDir, "\t"}, configPath); err != nil {
		t.Fatalf("expected blank entries to be ignored when a valid directory exists, got: %v", err)
	}
}

func TestValidateMusicDirs_OnlyBlankEntriesAreUnconfigured(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	err := ValidateMusicDirs([]string{"", "  ", "\t"}, configPath)
	if err == nil {
		t.Fatal("expected error for only blank music directories, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "no music directories configured") {
		t.Errorf("error should mention missing configured directories, got: %s", err.Error())
	}
}

func TestValidateMusicDir_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test as root")
	}
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	restrictedDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(restrictedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(restrictedDir, 0o755) }) //nolint:gosec // G302: restoring perms in test cleanup

	err := ValidateMusicDirs([]string{restrictedDir}, configPath)
	if err == nil {
		t.Fatal("expected error for permission-denied directory, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention 'permission denied', got: %s", err.Error())
	}
}

func TestValidateMusicDir_OnlyNonAudioFiles(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "cover.jpg"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDirs([]string{musicDir}, configPath)
	if err == nil {
		t.Fatal("expected error for directory with only non-audio files, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "no supported audio files") {
		t.Errorf("error should mention 'no supported audio files', got: %s", err.Error())
	}
}

func TestValidateMusicDir_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	emptyDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDirs([]string{emptyDir}, configPath)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty directory, got: %s", err.Error())
	}
}

func TestValidateMusicDir_ValidDirectory(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "song.mp3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMusicDirs([]string{musicDir}, configPath); err != nil {
		t.Errorf("expected no error for valid directory, got: %s", err.Error())
	}
}

func TestValidateMusicDir_ValidDirectoryWithSubdirs(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create subdirectories that would contain audio files (typical library layout).
	if err := os.Mkdir(filepath.Join(musicDir, "Artist"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMusicDirs([]string{musicDir}, configPath); err != nil {
		t.Errorf("expected no error for directory with subdirectories, got: %s", err.Error())
	}
}

func TestValidateMusicDir_OnlyHiddenDirs(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(musicDir, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDirs([]string{musicDir}, configPath)
	if err == nil {
		t.Fatal("expected error for directory with only hidden subdirectories, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "no supported audio files") {
		t.Errorf("error should mention 'no supported audio files', got: %s", err.Error())
	}
}

func TestValidateMusicDir_OnlyHiddenAudioFiles(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	musicDir := filepath.Join(tmp, "music")
	if err := os.Mkdir(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, ".hidden.mp3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDirs([]string{musicDir}, configPath)
	if err == nil {
		t.Fatal("expected error for directory with only hidden audio files, got nil")
	}
	assertActionable(t, err, configPath)
	if !strings.Contains(err.Error(), "no supported audio files") {
		t.Errorf("error should mention 'no supported audio files', got: %s", err.Error())
	}
}

func TestValidateMusicDir_TildeExpansion(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	// Using a tilde path that resolves to a non-existent directory ensures
	// tilde expansion runs without depending on real home dir contents.
	err := ValidateMusicDirs([]string{"~/no-such-music-dir-xyz"}, configPath)
	if err == nil {
		t.Fatal("expected error for missing tilde-expanded directory, got nil")
	}
	assertActionable(t, err, configPath)
}

// TestValidateMusicDirs_MultipleDirectoriesSomeInvalid verifies that when
// multiple directories are validated and some are invalid, the error message
// lists all the bad ones.
func TestValidateMusicDirs_MultipleDirectoriesSomeInvalid(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	// Create one valid dir with an audio file.
	validDir := filepath.Join(tmp, "valid")
	if err := os.Mkdir(validDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "song.mp3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	// Two invalid dirs: missing and empty.
	missingDir := filepath.Join(tmp, "no-such-dir")
	emptyDir := filepath.Join(tmp, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidateMusicDirs([]string{validDir, missingDir, emptyDir}, configPath)
	if err == nil {
		t.Fatal("expected error for dirs with some invalid entries, got nil")
	}
	assertActionable(t, err, configPath)
	// Error should mention the missing dir.
	if !strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "no-such-dir") {
		t.Errorf("error should mention missing dir, got: %s", err.Error())
	}
	// Error should mention the empty dir.
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty dir, got: %s", err.Error())
	}
}

// A mix of valid and invalid directories is degradation, not a dead
// configuration: the usable roots are reported so callers can keep scanning
// them instead of telling the user nothing was found.
func TestValidateMusicDirsDetailed_PartialValidity(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	validDir := filepath.Join(tmp, "valid")
	if err := os.Mkdir(validDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "song.mp3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	missingDir := filepath.Join(tmp, "no-such-dir")

	res := ValidateMusicDirsDetailed([]string{validDir, missingDir}, configPath)

	if res.Err == nil {
		t.Fatal("Err = nil, want the missing directory reported")
	}
	if !res.Degraded() {
		t.Error("Degraded() = false, want true with one usable directory")
	}
	if len(res.Usable) != 1 || res.Usable[0] != validDir {
		t.Errorf("Usable = %v, want [%s]", res.Usable, validDir)
	}
}

func TestValidateMusicDirsDetailed_NoUsableDirsIsNotDegraded(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")

	for name, dirs := range map[string][]string{
		"all invalid":  {filepath.Join(tmp, "no-such-dir")},
		"none set":     nil,
		"blank only":   {"  ", "\t"},
		"empty string": {""},
	} {
		res := ValidateMusicDirsDetailed(dirs, configPath)
		if res.Err == nil {
			t.Errorf("%s: Err = nil, want an error", name)
		}
		if res.Degraded() {
			t.Errorf("%s: Degraded() = true, want false with no usable directory", name)
		}
		if len(res.Usable) != 0 {
			t.Errorf("%s: Usable = %v, want none", name, res.Usable)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standalone tilde", "~", home},
		{"tilde slash", "~/Music", filepath.Join(home, "Music")},
		{"tilde slash nested", "~/a/b/c", filepath.Join(home, "a/b/c")},
		{"absolute path unchanged", "/usr/local", "/usr/local"},
		{"relative path unchanged", "relative/path", "relative/path"},
		{"tilde user unchanged", "~otheruser/foo", "~otheruser/foo"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandTilde(tt.input)
			if err != nil {
				t.Fatalf("expandTilde(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
