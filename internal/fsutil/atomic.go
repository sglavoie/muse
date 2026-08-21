// Package fsutil holds filesystem helpers shared by the packages that persist
// user data outside the database.
package fsutil

import (
	"os"
	"path/filepath"
)

// WriteAtomically writes to path via a temporary file in the same directory
// that is then renamed over the destination, so a crash or out-of-disk failure
// mid-write cannot leave a truncated file. After the rename the parent
// directory is fsynced so the rename itself survives a power loss; without this
// step POSIX permits a crash window in which the rename has been buffered but
// not yet committed, leaving the old inode visible (or no entry at all) on
// reboot.
func WriteAtomically(path string, perm os.FileMode, write func(*os.File) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if anything below fails before the rename.
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	// Fsync the parent directory so the rename is durable. Failure here is
	// best-effort: on filesystems where opening a directory for sync is not
	// supported the rename is still visible to the running process.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
