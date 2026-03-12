package config

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	defaultMusicDirectory  = "~/Music"
	defaultMPVSocketPath   = "/tmp/muse-mpv.sock"
	defaultMPVExtraFlags   = ""
	defaultVolume          = 100
	configFileName         = "config.toml"
)

// Config holds all application configuration values.
type Config struct {
	MusicDirectory string   `toml:"music_directory"`
	MPVSocketPath  string   `toml:"mpv_socket_path"`
	MPVExtraFlags  []string `toml:"mpv_extra_flags"`
	DefaultVolume  int      `toml:"default_volume"`
}

// defaults returns a Config populated with built-in default values.
func defaults() Config {
	return Config{
		MusicDirectory: defaultMusicDirectory,
		MPVSocketPath:  defaultMPVSocketPath,
		MPVExtraFlags:  []string{},
		DefaultVolume:  defaultVolume,
	}
}

// Load reads the TOML config file at configDir/config.toml.
// If the file does not exist it is created with default values.
// Invalid field values are replaced with their defaults and a warning is logged.
// Returns the config and whether the file was newly created.
func Load(configDir string) (Config, bool, error) {
	path := filepath.Join(configDir, configFileName)

	cfg := defaults()

	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			return cfg, false, err
		}
		if err := writeDefaults(path, cfg); err != nil {
			return cfg, false, err
		}
		return cfg, true, nil
	}
	if err != nil {
		return cfg, false, err
	}

	var raw Config
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		slog.Warn("failed to parse config file, using defaults", "path", path, "error", err)
		return cfg, false, nil
	}

	cfg = validate(raw)
	return cfg, false, nil
}

// validate checks each field and falls back to defaults for invalid values,
// logging a warning for every field that is replaced.
func validate(raw Config) Config {
	cfg := defaults()

	if raw.MusicDirectory != "" {
		cfg.MusicDirectory = raw.MusicDirectory
	} else {
		slog.Warn("invalid config value, using default",
			"field", "music_directory",
			"default", defaultMusicDirectory,
		)
	}

	if raw.MPVSocketPath != "" {
		cfg.MPVSocketPath = raw.MPVSocketPath
	} else {
		slog.Warn("invalid config value, using default",
			"field", "mpv_socket_path",
			"default", defaultMPVSocketPath,
		)
	}

	cfg.MPVExtraFlags = raw.MPVExtraFlags
	if cfg.MPVExtraFlags == nil {
		cfg.MPVExtraFlags = []string{}
	}

	if raw.DefaultVolume >= 0 && raw.DefaultVolume <= 100 {
		cfg.DefaultVolume = raw.DefaultVolume
	} else {
		slog.Warn("invalid config value, using default",
			"field", "default_volume",
			"value", raw.DefaultVolume,
			"default", defaultVolume,
		)
	}

	return cfg
}

// writeDefaults serialises the default config to path as TOML.
func writeDefaults(path string, cfg Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
