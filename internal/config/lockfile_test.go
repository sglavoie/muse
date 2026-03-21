package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLockfile_CreatedAtRuntimeDir(t *testing.T) {
	tmp := t.TempDir()
	lf := NewLockfile(tmp)

	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(lf.Release)

	expected := filepath.Join(tmp, lockfileName)
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("lockfile not created at %q: %v", expected, err)
	}
}

func TestLockfile_LivePIDBlocksLaunch(t *testing.T) {
	tmp := t.TempDir()

	// Write the current process's PID as if a prior instance created the lockfile.
	lockPath := filepath.Join(tmp, lockfileName)
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	lf := NewLockfile(tmp)
	err := lf.Acquire()
	if err == nil {
		t.Fatal("expected error when live PID exists, got nil")
	}
	if !strings.Contains(err.Error(), "another instance is running") {
		t.Errorf("error should mention 'another instance is running', got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), strconv.Itoa(os.Getpid())) {
		t.Errorf("error should include the PID, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), lockfileName) {
		t.Errorf("error should include the lockfile path, got: %s", err.Error())
	}
}

func TestLockfile_StaleLockfileIsReclaimed(t *testing.T) {
	tmp := t.TempDir()

	// Write a PID that is guaranteed to be dead.
	// PID 0 is never a valid user process; signal-0 to it will fail.
	lockPath := filepath.Join(tmp, lockfileName)
	if err := os.WriteFile(lockPath, []byte("99999999\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	lf := NewLockfile(tmp)
	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire should succeed after reclaiming stale lockfile: %v", err)
	}
	t.Cleanup(lf.Release)

	// Confirm the lockfile now contains our PID.
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("reading lockfile after reclaim: %v", err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parsing PID from lockfile: %v", err)
	}
	if got != os.Getpid() {
		t.Errorf("lockfile PID = %d, want %d", got, os.Getpid())
	}
}

func TestLockfile_ReleasedOnCleanup(t *testing.T) {
	tmp := t.TempDir()
	lf := NewLockfile(tmp)

	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	lf.Release()

	lockPath := filepath.Join(tmp, lockfileName)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("lockfile should be removed after Release, but still exists at %q", lockPath)
	}
}

func TestLockfile_RuntimeDirCreatedIfMissing(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "subdir", "muse")

	lf := NewLockfile(nested)
	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire should create missing runtime dir: %v", err)
	}
	t.Cleanup(lf.Release)

	if _, err := os.Stat(nested); err != nil {
		t.Errorf("runtime dir not created: %v", err)
	}
}
