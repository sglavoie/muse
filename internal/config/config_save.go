package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Save writes cfg to path as TOML, replacing any existing file.
// It is used to persist settings changes made via the in-app settings overlay.
// The write is atomic: the encoded content is first written to a temporary
// file in the same directory and then renamed over the destination, so a
// crash or out-of-disk failure mid-write cannot leave a truncated config.
// After the rename the parent directory is fsynced so the rename itself
// survives a power loss; without this step POSIX permits a crash window in
// which the rename has been buffered but not yet committed, leaving the old
// inode visible (or no entry at all) on reboot.
func Save(path string, cfg Config) error {
	dirs := normalizeMusicDirectories(cfg.MusicDirectories)
	if len(dirs) == 0 {
		dirs = normalizeMusicDirectories([]string{cfg.MusicDirectory})
	}
	if len(dirs) > 0 {
		cfg.MusicDirectories = dirs
		cfg.MusicDirectory = dirs[0]
	}

	return writeAtomically(path, 0o600, func(f *os.File) error {
		return toml.NewEncoder(f).Encode(cfg)
	})
}

func writeAtomically(path string, perm os.FileMode, write func(*os.File) error) error {
	dir := filepath.Dir(path)
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

// writeDefaults writes a commented TOML config to path using the values in cfg.
func writeDefaults(path string, cfg Config) error {
	firstDir := cfg.MusicDirectory
	if len(cfg.MusicDirectories) > 0 {
		firstDir = cfg.MusicDirectories[0]
	}
	return writeAtomically(path, 0o600, func(f *os.File) error {
		_, err := fmt.Fprintf(f, configTemplate, firstDir, cfg.DefaultVolume, cfg.LogLevel)
		return err
	})
}
