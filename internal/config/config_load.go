package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
)

// Load reads the TOML config file at configDir/config.toml.
// If the file does not exist it is created with default values.
// Invalid field values are replaced with their defaults and a warning is logged.
func Load(configDir string) (LoadResult, error) {
	path := ConfigPath(configDir)

	cfg := defaults()

	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			return LoadResult{Config: cfg}, err
		}
		if err := writeDefaults(path, cfg); err != nil {
			return LoadResult{Config: cfg}, err
		}
		return LoadResult{Config: cfg, Created: true}, nil
	}
	if err != nil {
		return LoadResult{Config: cfg}, err
	}

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		slog.Warn("failed to parse config file, using defaults", "path", path, "error", err)
		fmt.Fprintf(os.Stderr, "muse: warning: failed to parse %s: %v (using defaults)\n", path, err)
		warnings := []string{fmt.Sprintf("failed to parse config: %v (using defaults)", err)}
		return LoadResult{Config: cfg, Warnings: warnings}, nil
	}

	cfg, warnings := validate(raw)
	return LoadResult{Config: cfg, Warnings: warnings}, nil
}
