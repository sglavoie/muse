package player

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// resetSoundOnceState resets the package-level once and soundPaths so the
// caller can exercise ensureSoundFiles in isolation. It restores a fresh
// once and the prior paths map when the returned cleanup runs.
func resetSoundOnceState(t *testing.T) func() {
	t.Helper()
	prevPaths := soundPaths
	soundOnce = sync.Once{}
	soundPaths = nil
	return func() {
		soundOnce = sync.Once{}
		soundPaths = prevPaths
	}
}

// setUserCacheDir points os.UserCacheDir at dir: $XDG_CACHE_HOME on Linux and
// other Unixes, $HOME/Library/Caches on macOS.
func setUserCacheDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
}

func TestEnsureSoundFiles_SkipsPathsOnWriteFailure(t *testing.T) {
	defer resetSoundOnceState(t)()

	// Point the user cache dir at a file so MkdirAll fails and subsequent
	// WriteFile calls also fail. This mirrors a degraded filesystem state.
	tmpRoot := t.TempDir()
	blocker := filepath.Join(tmpRoot, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	setUserCacheDir(t, blocker)

	ensureSoundFiles()

	if soundPaths == nil {
		t.Fatalf("soundPaths should be initialized")
	}
	for s, p := range soundPaths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("soundPaths contains entry for sound %d pointing to missing file %q (err: %v); writes that fail must not populate the map", s, p, err)
		}
	}
}

func TestEnsureSoundFiles_PopulatesAllOnSuccess(t *testing.T) {
	defer resetSoundOnceState(t)()

	setUserCacheDir(t, t.TempDir())

	ensureSoundFiles()

	if got, want := len(soundPaths), len(soundData); got != want {
		t.Fatalf("soundPaths len = %d, want %d", got, want)
	}
	for s, p := range soundPaths {
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("sound %d path %q not written: %v", s, p, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("sound %d path %q is empty", s, p)
		}
	}
}

// TestEnsureSoundFiles_WritesToPrivateUserDir guards against a fixed, shared
// directory under os.TempDir(), which another local user could pre-create or
// fill with symlinks: the sounds must land in a 0700 directory inside the
// user's cache directory.
func TestEnsureSoundFiles_WritesToPrivateUserDir(t *testing.T) {
	defer resetSoundOnceState(t)()

	setUserCacheDir(t, t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	wantDir := filepath.Join(cache, "muse", "sounds")

	ensureSoundFiles()

	if got, want := len(soundPaths), len(soundData); got != want {
		t.Fatalf("soundPaths len = %d, want %d", got, want)
	}
	for s, p := range soundPaths {
		if got := filepath.Dir(p); got != wantDir {
			t.Errorf("sound %d written to %q, want directory %q", s, p, wantDir)
		}
	}
	info, err := os.Stat(wantDir)
	if err != nil {
		t.Fatalf("stat sound dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("sound dir mode = %o, want 700", perm)
	}
}

// TestEnsureSoundFiles_FallsBackToRuntimeDir covers a host with neither $HOME
// nor $XDG_CACHE_HOME: the sounds go to the per-user runtime directory rather
// than a shared temp path.
func TestEnsureSoundFiles_FallsBackToRuntimeDir(t *testing.T) {
	defer resetSoundOnceState(t)()

	setUserCacheDir(t, "")
	runtimeRoot := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeRoot)
	if _, err := os.UserCacheDir(); err == nil {
		t.Skip("os.UserCacheDir resolves without $HOME on this platform")
	}

	ensureSoundFiles()

	wantDir := filepath.Join(runtimeRoot, "muse", "sounds")
	if got, want := len(soundPaths), len(soundData); got != want {
		t.Fatalf("soundPaths len = %d, want %d", got, want)
	}
	for s, p := range soundPaths {
		if got := filepath.Dir(p); got != wantDir {
			t.Errorf("sound %d written to %q, want directory %q", s, p, wantDir)
		}
	}
}

func TestPlaySound_MutedShortCircuits(t *testing.T) {
	// Muted is set true in TestMain; verify the contract.
	if !Muted {
		t.Fatalf("expected Muted=true in tests")
	}
	// Should not spawn anything, should not panic on unknown Sound values.
	PlaySound(SoundError)
	PlaySound(Sound(-1))
	PlaySound(Sound(999))
}
