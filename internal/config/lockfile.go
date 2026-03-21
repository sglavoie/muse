package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockfileName = "muse.pid"

// Lockfile manages a PID lockfile to prevent multiple instances from running.
type Lockfile struct {
	path string
}

// NewLockfile creates a Lockfile that will be placed in runtimeDir.
func NewLockfile(runtimeDir string) *Lockfile {
	return &Lockfile{path: filepath.Join(runtimeDir, lockfileName)}
}

// Acquire creates the lockfile with the current PID. If a lockfile already
// exists, it checks whether the recorded PID is still alive:
//   - If alive: returns an error with "another instance is running (PID N)".
//   - If dead (stale): removes the old lockfile and creates a fresh one.
func (l *Lockfile) Acquire() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("lockfile: could not create runtime directory %q: %w", filepath.Dir(l.path), err)
	}

	existing, err := os.ReadFile(l.path)
	if err == nil {
		// Lockfile exists — check whether the recorded PID is alive.
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(existing)))
		if parseErr == nil && isProcessAlive(pid) {
			return fmt.Errorf("another instance is running (PID %d); quit it first or remove %s if the process is stale", pid, l.path)
		}
		// PID is dead or unreadable — reclaim the stale lockfile.
		_ = os.Remove(l.path)
	}

	return os.WriteFile(l.path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
}

// Release removes the lockfile. It is a no-op if the file is already gone.
func (l *Lockfile) Release() {
	_ = os.Remove(l.path)
}

// isProcessAlive returns true if the given PID is a live process, using
// signal 0 (which performs permission and existence checks without sending
// an actual signal).
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
