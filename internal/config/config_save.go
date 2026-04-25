package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Save writes cfg to path as TOML, replacing any existing file.
// It is used to persist settings changes made via the in-app settings overlay.
func Save(path string, cfg Config) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	encErr := toml.NewEncoder(f).Encode(cfg)
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	return closeErr
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
