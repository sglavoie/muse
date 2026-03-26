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

# Volume step when pressing volume up/down (1-25%%).
# volume_increment = 5

# Short seek offset in seconds (1-60).
# seek_offset_short = 5.0

# Long seek offset in seconds (5-300).
# seek_offset_long = 30.0

# Duration in seconds for status bar notifications (1-30).
# transient_duration_secs = 5

# Duration in seconds for volume feedback notifications (1-10).
# volume_feedback_duration_secs = 2

# Enable gapless playback between tracks (true/false).
# gapless_audio = true

# Enable real-time EBU R128 volume normalization (true/false).
# normalize_volume = false
`

const (
	defaultMusicDirectory             = "~/Music"
	defaultVolume                     = 100
	defaultLogLevel                   = "info"
	configFileName                    = "config.toml"
	defaultVolumeIncrement            = 5
	defaultSeekOffsetShort            = 5.0
	defaultSeekOffsetLong             = 30.0
	defaultTransientDurationSecs      = 5
	defaultVolumeFeedbackDurationSecs = 2
	defaultNormalizeVolume             = false
	defaultGaplessAudio                = true
)

// Config holds all application configuration values.
type Config struct {
	MusicDirectory             string   `toml:"music_directory"`
	MPVSocketPath              string   `toml:"mpv_socket_path"`
	MPVExtraFlags              []string `toml:"mpv_extra_flags"`
	DefaultVolume              int      `toml:"default_volume"`
	LogLevel                   string   `toml:"log_level"`
	VolumeIncrement            int      `toml:"volume_increment"`
	SeekOffsetShort            float64  `toml:"seek_offset_short"`
	SeekOffsetLong             float64  `toml:"seek_offset_long"`
	TransientDurationSecs      int      `toml:"transient_duration_secs"`
	VolumeFeedbackDurationSecs int      `toml:"volume_feedback_duration_secs"`
	NormalizeVolume            bool     `toml:"normalize_volume"`
	GaplessAudio               bool     `toml:"gapless_audio"`
}

// Defaults returns a Config populated with built-in default values.
// The MusicDirectory is returned with the tilde expanded to an absolute path.
// It is exported so that callers that need a zero-dependency Config (e.g. tests
// that do not exercise config file loading) can obtain sensible defaults.
func Defaults() Config { return defaults() }

// defaults returns a Config populated with built-in default values.
// The MusicDirectory is returned with the tilde expanded to an absolute path.
func defaults() Config {
	musicDir := defaultMusicDirectory
	if expanded, err := expandTilde(musicDir); err == nil {
		musicDir = expanded
	} else {
		slog.Warn("failed to expand ~ in default music directory", "err", err)
	}
	return Config{
		MusicDirectory:             musicDir,
		MPVExtraFlags:              []string{},
		DefaultVolume:              defaultVolume,
		LogLevel:                   defaultLogLevel,
		VolumeIncrement:            defaultVolumeIncrement,
		SeekOffsetShort:            defaultSeekOffsetShort,
		SeekOffsetLong:             defaultSeekOffsetLong,
		TransientDurationSecs:      defaultTransientDurationSecs,
		VolumeFeedbackDurationSecs: defaultVolumeFeedbackDurationSecs,
		NormalizeVolume:            defaultNormalizeVolume,
		GaplessAudio:               defaultGaplessAudio,
	}
}

// ConfigPath returns the path to the config file within configDir.
// Use this instead of constructing the path manually so that all callers
// stay in sync if the filename ever changes.
func ConfigPath(configDir string) string {
	return filepath.Join(configDir, configFileName)
}

// LoadResult holds the output of a successful Load call.
type LoadResult struct {
	Config   Config
	Created  bool
	Warnings []string
}

// Load reads the TOML config file at configDir/config.toml.
// If the file does not exist it is created with default values.
// Invalid field values are replaced with their defaults and a warning is logged.
func Load(configDir string) (LoadResult, error) {
	path := filepath.Join(configDir, configFileName)

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

// rawConfig mirrors Config but uses pointer types for optional numeric fields
// so that omitted TOML keys can be distinguished from explicit zero values.
type rawConfig struct {
	MusicDirectory             string   `toml:"music_directory"`
	MPVSocketPath              string   `toml:"mpv_socket_path"`
	MPVExtraFlags              []string `toml:"mpv_extra_flags"`
	DefaultVolume              *int     `toml:"default_volume"`
	LogLevel                   string   `toml:"log_level"`
	VolumeIncrement            *int     `toml:"volume_increment"`
	SeekOffsetShort            *float64 `toml:"seek_offset_short"`
	SeekOffsetLong             *float64 `toml:"seek_offset_long"`
	TransientDurationSecs      *int     `toml:"transient_duration_secs"`
	VolumeFeedbackDurationSecs *int     `toml:"volume_feedback_duration_secs"`
	NormalizeVolume            *bool    `toml:"normalize_volume"`
	GaplessAudio               *bool    `toml:"gapless_audio"`
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

	if raw.MPVSocketPath != "" {
		if expanded, err := expandTilde(raw.MPVSocketPath); err == nil {
			cfg.MPVSocketPath = expanded
		} else {
			cfg.MPVSocketPath = raw.MPVSocketPath
		}
	}

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

	if raw.VolumeIncrement != nil {
		v := *raw.VolumeIncrement
		if v >= 1 && v <= 25 {
			cfg.VolumeIncrement = v
		} else {
			slog.Warn("invalid config value, using default",
				"field", "volume_increment",
				"value", v,
				"default", defaultVolumeIncrement,
			)
			warnings = append(warnings, fmt.Sprintf("volume_increment %d is out of range (1-25), using %d", v, defaultVolumeIncrement))
		}
	}

	if raw.SeekOffsetShort != nil {
		v := *raw.SeekOffsetShort
		if v >= 1.0 && v <= 60.0 {
			cfg.SeekOffsetShort = v
		} else {
			slog.Warn("invalid config value, using default",
				"field", "seek_offset_short",
				"value", v,
				"default", defaultSeekOffsetShort,
			)
			warnings = append(warnings, fmt.Sprintf("seek_offset_short %.1f is out of range (1-60), using %.1f", v, defaultSeekOffsetShort))
		}
	}

	if raw.SeekOffsetLong != nil {
		v := *raw.SeekOffsetLong
		if v >= 5.0 && v <= 300.0 {
			cfg.SeekOffsetLong = v
		} else {
			slog.Warn("invalid config value, using default",
				"field", "seek_offset_long",
				"value", v,
				"default", defaultSeekOffsetLong,
			)
			warnings = append(warnings, fmt.Sprintf("seek_offset_long %.1f is out of range (5-300), using %.1f", v, defaultSeekOffsetLong))
		}
	}

	if raw.TransientDurationSecs != nil {
		v := *raw.TransientDurationSecs
		if v >= 1 && v <= 30 {
			cfg.TransientDurationSecs = v
		} else {
			slog.Warn("invalid config value, using default",
				"field", "transient_duration_secs",
				"value", v,
				"default", defaultTransientDurationSecs,
			)
			warnings = append(warnings, fmt.Sprintf("transient_duration_secs %d is out of range (1-30), using %d", v, defaultTransientDurationSecs))
		}
	}

	if raw.VolumeFeedbackDurationSecs != nil {
		v := *raw.VolumeFeedbackDurationSecs
		if v >= 1 && v <= 10 {
			cfg.VolumeFeedbackDurationSecs = v
		} else {
			slog.Warn("invalid config value, using default",
				"field", "volume_feedback_duration_secs",
				"value", v,
				"default", defaultVolumeFeedbackDurationSecs,
			)
			warnings = append(warnings, fmt.Sprintf("volume_feedback_duration_secs %d is out of range (1-10), using %d", v, defaultVolumeFeedbackDurationSecs))
		}
	}

	if raw.NormalizeVolume != nil {
		cfg.NormalizeVolume = *raw.NormalizeVolume
	}

	if raw.GaplessAudio != nil {
		cfg.GaplessAudio = *raw.GaplessAudio
	}

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

// Save writes cfg to path as TOML, replacing any existing file.
// It is used to persist settings changes made via the in-app settings overlay.
func Save(path string, cfg Config) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
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
