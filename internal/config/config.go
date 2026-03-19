package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	defaultMusicDirectory = "~/Music"
	defaultVolume         = 100
	defaultLogLevel       = "info"
	configFileName        = "config.toml"
)

// Config holds all application configuration values.
type Config struct {
	MusicDirectory string   `toml:"music_directory"`
	MPVSocketPath  string   `toml:"mpv_socket_path"`
	MPVExtraFlags  []string `toml:"mpv_extra_flags"`
	DefaultVolume  int      `toml:"default_volume"`
	LogLevel       string   `toml:"log_level"`
}

// defaults returns a Config populated with built-in default values.
// The MusicDirectory is returned with the tilde expanded to an absolute path.
func defaults() Config {
	musicDir := defaultMusicDirectory
	if expanded, err := expandTilde(musicDir); err == nil {
		musicDir = expanded
	}
	return Config{
		MusicDirectory: musicDir,
		MPVExtraFlags:  []string{},
		DefaultVolume:  defaultVolume,
		LogLevel:       defaultLogLevel,
	}
}

// ConfigPath returns the path to the config file within configDir.
// Use this instead of constructing the path manually so that all callers
// stay in sync if the filename ever changes.
func ConfigPath(configDir string) string {
	return filepath.Join(configDir, configFileName)
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

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		slog.Warn("failed to parse config file, using defaults", "path", path, "error", err)
		fmt.Fprintf(os.Stderr, "muse: warning: failed to parse %s: %v (using defaults)\n", path, err)
		return cfg, false, nil
	}

	cfg = validate(raw)
	return cfg, false, nil
}

// rawConfig mirrors Config but uses pointer types for optional numeric fields
// so that omitted TOML keys can be distinguished from explicit zero values.
type rawConfig struct {
	MusicDirectory string   `toml:"music_directory"`
	MPVSocketPath  string   `toml:"mpv_socket_path"`
	MPVExtraFlags  []string `toml:"mpv_extra_flags"`
	DefaultVolume  *int     `toml:"default_volume"`
	LogLevel       string   `toml:"log_level"`
}

// validate checks each field and falls back to defaults for invalid values,
// logging a warning for every field that is replaced.
func validate(raw rawConfig) Config {
	cfg := defaults()

	if raw.MusicDirectory != "" {
		if expanded, err := expandTilde(raw.MusicDirectory); err == nil {
			cfg.MusicDirectory = expanded
		} else {
			cfg.MusicDirectory = raw.MusicDirectory
		}
	} else {
		slog.Warn("invalid config value, using default",
			"field", "music_directory",
			"default", defaultMusicDirectory,
		)
	}

	cfg.MPVSocketPath = raw.MPVSocketPath

	cfg.MPVExtraFlags = raw.MPVExtraFlags
	if cfg.MPVExtraFlags == nil {
		cfg.MPVExtraFlags = []string{}
	}

	if raw.DefaultVolume != nil {
		v := *raw.DefaultVolume
		if v >= 0 && v <= 100 {
			cfg.DefaultVolume = v
		} else {
			slog.Warn("invalid config value, using default",
				"field", "default_volume",
				"value", v,
				"default", defaultVolume,
			)
		}
	}
	// nil means omitted — keep the default silently.

	switch raw.LogLevel {
	case "debug", "info", "warn", "error":
		cfg.LogLevel = raw.LogLevel
	case "":
		// empty means not set; keep default silently
	default:
		slog.Warn("invalid config value, using default",
			"field", "log_level",
			"value", raw.LogLevel,
			"default", defaultLogLevel,
		)
	}

	return cfg
}

// writeDefaults serializes the default config to path as TOML.
func writeDefaults(path string, cfg Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
