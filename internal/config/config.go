package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const configTemplate = `# Path to your music library.
music_directory = %q

# Default playback volume (0-100).
default_volume = %d

# Log verbosity: debug, info, warn, error (case-insensitive).
log_level = %q

# Custom path for the mpv IPC socket (leave empty for default).
# mpv_socket_path = ""

# Extra flags passed to the mpv subprocess.
# mpv_extra_flags = ["--audio-device=pulse"]
`

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
// Returns the config, whether the file was newly created, any validation warnings, and any error.
func Load(configDir string) (Config, bool, []string, error) {
	path := filepath.Join(configDir, configFileName)

	cfg := defaults()

	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			return cfg, false, nil, err
		}
		if err := writeDefaults(path, cfg); err != nil {
			return cfg, false, nil, err
		}
		return cfg, true, nil, nil
	}
	if err != nil {
		return cfg, false, nil, err
	}

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		slog.Warn("failed to parse config file, using defaults", "path", path, "error", err)
		fmt.Fprintf(os.Stderr, "muse: warning: failed to parse %s: %v (using defaults)\n", path, err)
		warnings := []string{fmt.Sprintf("failed to parse config: %v (using defaults)", err)}
		return cfg, false, warnings, nil
	}

	cfg, warnings := validate(raw)
	return cfg, false, warnings, nil
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
// logging a warning for every field that is replaced. It also returns a slice
// of human-readable warning strings for display in the TUI.
func validate(raw rawConfig) (Config, []string) {
	cfg := defaults()
	var warnings []string

	if raw.MusicDirectory != "" {
		if expanded, err := expandTilde(raw.MusicDirectory); err == nil {
			cfg.MusicDirectory = expanded
		} else {
			cfg.MusicDirectory = raw.MusicDirectory
		}
	}
	// empty means not set — use the default silently

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
			warnings = append(warnings, fmt.Sprintf("default_volume %d is out of range (0-100), using %d", v, defaultVolume))
		}
	}
	// nil means omitted — keep the default silently.

	raw.LogLevel = strings.ToLower(raw.LogLevel)
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
		warnings = append(warnings, fmt.Sprintf("log_level %q is not recognized (use debug/info/warn/error), using %q", raw.LogLevel, defaultLogLevel))
	}

	return cfg, warnings
}

// writeDefaults writes a commented TOML config to path using the values in cfg.
func writeDefaults(path string, cfg Config) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, configTemplate, cfg.MusicDirectory, cfg.DefaultVolume, cfg.LogLevel)
	return err
}
