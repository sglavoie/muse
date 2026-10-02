package config

import (
	"errors"
	"fmt"
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

	owner := NewLockfile(tmp)
	if err := owner.Acquire(); err != nil {
		t.Fatalf("owner Acquire: %v", err)
	}
	t.Cleanup(owner.Release)

	contender := NewLockfile(tmp)
	err := contender.Acquire()
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

func TestLockfile_ConcurrentAcquireAllowsSingleWinner(t *testing.T) {
	tmp := t.TempDir()
	const contenders = 8

	type result struct {
		lf  *Lockfile
		err error
	}
	results := make(chan result, contenders)
	start := make(chan struct{})

	for i := 0; i < contenders; i++ {
		lf := NewLockfile(tmp)
		go func(lf *Lockfile) {
			<-start
			results <- result{lf: lf, err: lf.Acquire()}
		}(lf)
	}

	close(start)

	successes := 0
	for i := 0; i < contenders; i++ {
		res := <-results
		if res.err == nil {
			successes++
			t.Cleanup(res.lf.Release)
			continue
		}
		if !strings.Contains(res.err.Error(), "another instance is running") {
			t.Fatalf("expected contention error, got %v", res.err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful acquire, got %d", successes)
	}
}

func TestLockfile_ReleaseKeepsPathAndAllowsReacquire(t *testing.T) {
	tmp := t.TempDir()
	lf := NewLockfile(tmp)

	if err := lf.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	lf.Release()

	lockPath := filepath.Join(tmp, lockfileName)
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lockfile should remain after Release, stat err: %v", err)
	}

	reacquired := NewLockfile(tmp)
	if err := reacquired.Acquire(); err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	reacquired.Release()
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

// TestLockfile_AlreadyRunningErrorIsTyped: a held lock surfaces as
// *AlreadyRunningError carrying the holder's PID and the lockfile path, and it
// matches ErrAlreadyRunning even after the startup path wraps it.
func TestLockfile_AlreadyRunningErrorIsTyped(t *testing.T) {
	tmp := t.TempDir()
	owner := NewLockfile(tmp)
	if err := owner.Acquire(); err != nil {
		t.Fatalf("owner Acquire: %v", err)
	}
	t.Cleanup(owner.Release)

	err := fmt.Errorf("startup: %w", NewLockfile(tmp).Acquire())
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("errors.Is(%v, ErrAlreadyRunning) = false", err)
	}
	var running *AlreadyRunningError
	if !errors.As(err, &running) {
		t.Fatalf("errors.As(%v, *AlreadyRunningError) = false", err)
	}
	if running.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", running.PID, os.Getpid())
	}
	if want := filepath.Join(tmp, lockfileName); running.Path != want {
		t.Errorf("Path = %q, want %q", running.Path, want)
	}
}

// TestAlreadyRunningError_MessageUnchanged pins the message text, with and
// without a readable PID, to what Acquire printed before the type existed.
func TestAlreadyRunningError_MessageUnchanged(t *testing.T) {
	withPID := &AlreadyRunningError{PID: 42, Path: "/run/muse.pid"}
	if got, want := withPID.Error(), "another instance is running (PID 42); quit it first or remove /run/muse.pid if the process is stale"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	noPID := &AlreadyRunningError{Path: "/run/muse.pid"}
	if got, want := noPID.Error(), "another instance is running; quit it first or remove /run/muse.pid if the process is stale"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if errors.Is(errors.New("another instance is running"), ErrAlreadyRunning) {
		t.Error("an unrelated error with the same text matched ErrAlreadyRunning")
	}
}
