package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load reads the TOML config file at configDir/config.toml.
// If the file does not exist it is created with default values.
// Invalid field values are replaced with their defaults and surfaced as warnings.
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
	md, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return LoadResult{Config: cfg}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return LoadResult{Config: cfg}, fmt.Errorf("unknown config keys in %q: %s", path, joinTOMLKeys(undecoded))
	}

	cfg, warnings := validate(raw)
	return LoadResult{Config: cfg, Warnings: warnings}, nil
}

func joinTOMLKeys(keys []toml.Key) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key.String()
	}
	return strings.Join(parts, ", ")
}
