package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/sglavoie/muse/internal/fsutil"
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
	return fsutil.WriteAtomically(path, perm, write)
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
