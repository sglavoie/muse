package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Save writes cfg to path as TOML, replacing any existing file.
// It is used to persist settings changes made via the in-app settings overlay.
// The write is atomic: the encoded content is first written to a temporary
// file in the same directory and then renamed over the destination, so a
// crash or out-of-disk failure mid-write cannot leave a truncated config.
func Save(path string, cfg Config) error {
	if len(cfg.MusicDirectories) == 0 {
		if trimmed := strings.TrimSpace(cfg.MusicDirectory); trimmed != "" {
			cfg.MusicDirectories = []string{trimmed}
		}
	}

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

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
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
	return nil
}

// writeDefaults writes a commented TOML config to path using the values in cfg.
func writeDefaults(path string, cfg Config) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	firstDir := cfg.MusicDirectory
	if len(cfg.MusicDirectories) > 0 {
		firstDir = cfg.MusicDirectories[0]
	}
	_, writeErr := fmt.Fprintf(f, configTemplate, firstDir, cfg.DefaultVolume, cfg.LogLevel)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
