package config

import (
	"log/slog"
	"path/filepath"
)

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
		MusicDirectories:           []string{musicDir},
		ExcludedPaths:              nil,
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
		EQEnabled:                  defaultEQEnabled,
		EQPreset:                   defaultEQPreset,
		EQBands:                    [10]int{},
		EQCustomPresets:            nil,
		StereoWideEnabled:          defaultStereoWideEnabled,
		StereoWideAmount:           defaultStereoWideAmount,
	}
}

// ConfigPath returns the path to the config file within configDir.
// Use this instead of constructing the path manually so that all callers
// stay in sync if the filename ever changes.
func ConfigPath(configDir string) string {
	return filepath.Join(configDir, configFileName)
}
