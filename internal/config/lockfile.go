package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockfileName = "muse.pid"

// Lockfile manages a PID lockfile to prevent multiple instances from running.
// The file handle stays open for the lifetime of the process so the advisory
// lock remains held until Release closes it.
type Lockfile struct {
	path string
	file *os.File
}

// NewLockfile creates a Lockfile that will be placed in runtimeDir.
func NewLockfile(runtimeDir string) *Lockfile {
	return &Lockfile{path: filepath.Join(runtimeDir, lockfileName)}
}

// Acquire opens the PID file, takes an exclusive non-blocking advisory lock,
// and writes the current PID. If another instance already holds the lock, it
// returns an error that includes the recorded PID when available. Stale
// lockfiles are reclaimed automatically because no process still holds the
// advisory lock.
func (l *Lockfile) Acquire() error {
	if l.file != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("lockfile: could not create runtime directory %q: %w", filepath.Dir(l.path), err)
	}

	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("lockfile: could not open %q: %w", l.path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		pid, pidErr := readLockfilePID(f)
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			if pidErr == nil {
				return fmt.Errorf("another instance is running (PID %d); quit it first or remove %s if the process is stale", pid, l.path)
			}
			return fmt.Errorf("another instance is running; quit it first or remove %s if the process is stale", l.path)
		}
		return fmt.Errorf("lockfile: could not acquire %q: %w", l.path, err)
	}
	if err := writeLockPID(f); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		_ = os.Remove(l.path)
		return err
	}
	l.file = f
	return nil
}

// Release closes the held advisory lock and removes the lockfile. It is a
// no-op when this Lockfile never acquired the lock or was already released.
func (l *Lockfile) Release() {
	if l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
	_ = os.Remove(l.path)
}

func writeLockPID(f *os.File) error {
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("lockfile: could not truncate %q: %w", f.Name(), err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("lockfile: could not rewind %q: %w", f.Name(), err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		return fmt.Errorf("lockfile: could not write PID to %q: %w", f.Name(), err)
	}
	return nil
}

func readLockfilePID(f *os.File) (int, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
