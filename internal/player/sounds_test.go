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

func TestEnsureSoundFiles_SkipsPathsOnWriteFailure(t *testing.T) {
	defer resetSoundOnceState(t)()

	// Point TMPDIR at a file so MkdirAll fails and subsequent WriteFile calls
	// also fail. This mirrors a degraded filesystem state.
	tmpRoot := t.TempDir()
	blocker := filepath.Join(tmpRoot, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	t.Setenv("TMPDIR", blocker)

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

	t.Setenv("TMPDIR", t.TempDir())

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
