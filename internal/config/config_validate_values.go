package config

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// validate checks each field and falls back to defaults for invalid values,
// logging a warning for every field that is replaced. It also returns a slice
// of human-readable warning strings for display in the TUI.
func validate(raw rawConfig) (Config, []string) {
	cfg := defaults()
	var warnings []string

	// Resolve music directories: prefer music_directories, fall back to
	// legacy music_directory, then use the built-in default.
	var rawDirs []string
	switch {
	case len(raw.MusicDirectories) > 0:
		rawDirs = raw.MusicDirectories
	case raw.MusicDirectory != "":
		rawDirs = []string{raw.MusicDirectory}
	}
	if len(rawDirs) > 0 {
		seen := make(map[string]bool, len(rawDirs))
		var resolved []string
		for _, d := range rawDirs {
			trimmed := strings.TrimSpace(d)
			if trimmed == "" {
				continue
			}
			expanded, err := expandTilde(trimmed)
			if err != nil {
				expanded = trimmed
			}
			clean := filepath.Clean(expanded)
			if !seen[clean] {
				seen[clean] = true
				resolved = append(resolved, clean)
			}
		}
		if len(resolved) > 0 {
			cfg.MusicDirectories = resolved
			cfg.MusicDirectory = resolved[0]
		}
	}
	// empty means not set - use the default silently

	// Dedup, trim, expand, and clean excluded paths.
	if len(raw.ExcludedPaths) > 0 {
		seenExcl := make(map[string]bool, len(raw.ExcludedPaths))
		var resolved []string
		for _, p := range raw.ExcludedPaths {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			if expanded, err := expandTilde(trimmed); err == nil {
				trimmed = expanded
			}
			if strings.Contains(trimmed, string(filepath.Separator)) {
				trimmed = filepath.Clean(trimmed)
			}
			if !seenExcl[trimmed] {
				seenExcl[trimmed] = true
				resolved = append(resolved, trimmed)
			}
		}
		cfg.ExcludedPaths = resolved
	}

	if trimmedSocket := strings.TrimSpace(raw.MPVSocketPath); trimmedSocket != "" {
		if expanded, err := expandTilde(trimmedSocket); err == nil {
			cfg.MPVSocketPath = expanded
		} else {
			cfg.MPVSocketPath = trimmedSocket
		}
	}

	cfg.MPVExtraFlags = []string{}
	for _, f := range raw.MPVExtraFlags {
		if strings.TrimSpace(f) == "" {
			continue
		}
		cfg.MPVExtraFlags = append(cfg.MPVExtraFlags, f)
	}

	validateIntRange(raw.DefaultVolume, 0, 100, "default_volume", defaultVolume, &cfg.DefaultVolume, &warnings)
	validateIntRange(raw.VolumeIncrement, 1, 25, "volume_increment", defaultVolumeIncrement, &cfg.VolumeIncrement, &warnings)
	validateFloatRange(raw.SeekOffsetShort, 1, 60, "seek_offset_short", defaultSeekOffsetShort, &cfg.SeekOffsetShort, &warnings)
	validateFloatRange(raw.SeekOffsetLong, 5, 300, "seek_offset_long", defaultSeekOffsetLong, &cfg.SeekOffsetLong, &warnings)
	validateIntRange(raw.TransientDurationSecs, 1, 30, "transient_duration_secs", defaultTransientDurationSecs, &cfg.TransientDurationSecs, &warnings)
	validateIntRange(raw.VolumeFeedbackDurationSecs, 1, 10, "volume_feedback_duration_secs", defaultVolumeFeedbackDurationSecs, &cfg.VolumeFeedbackDurationSecs, &warnings)

	if raw.NormalizeVolume != nil {
		cfg.NormalizeVolume = *raw.NormalizeVolume
	}

	if raw.GaplessAudio != nil {
		cfg.GaplessAudio = *raw.GaplessAudio
	}

	raw.LogLevel = strings.ToLower(strings.TrimSpace(raw.LogLevel))
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

	if raw.EQEnabled != nil {
		cfg.EQEnabled = *raw.EQEnabled
	}

	if raw.EQBands != nil {
		for i, v := range raw.EQBands {
			switch {
			case v < -12:
				cfg.EQBands[i] = -12
			case v > 12:
				cfg.EQBands[i] = 12
			default:
				cfg.EQBands[i] = v
			}
		}
	}

	// Validate and deduplicate custom presets, clamping their bands.
	seen := make(map[string]bool)
	for _, cp := range raw.EQCustomPresets {
		if cp.Key == "" || seen[cp.Key] {
			continue
		}
		seen[cp.Key] = true
		clamped := cp.Bands
		for i, v := range clamped {
			if v < -12 {
				clamped[i] = -12
			} else if v > 12 {
				clamped[i] = 12
			}
		}
		cfg.EQCustomPresets = append(cfg.EQCustomPresets, EQCustomPreset{
			Name:  cp.Name,
			Key:   cp.Key,
			Bands: clamped,
		})
	}

	// Validate preset key: must match a known built-in or custom key.
	if raw.EQPreset != "" {
		allPresets := make([]EQPresetDef, 0, len(BuiltinEQPresets)+len(cfg.EQCustomPresets))
		allPresets = append(allPresets, BuiltinEQPresets...)
		for _, cp := range cfg.EQCustomPresets {
			allPresets = append(allPresets, EQPresetDef{Key: cp.Key})
		}
		if EQPresetIndex(raw.EQPreset, allPresets) >= 0 {
			cfg.EQPreset = raw.EQPreset
		} else {
			slog.Warn("invalid config value, using default",
				"field", "eq_preset",
				"value", raw.EQPreset,
				"default", defaultEQPreset,
			)
			warnings = append(warnings, fmt.Sprintf("eq_preset %q is not a known preset key, using %q", raw.EQPreset, defaultEQPreset))
		}
	}

	if raw.StereoWideEnabled != nil {
		cfg.StereoWideEnabled = *raw.StereoWideEnabled
	}
	validateIntRange(raw.StereoWideAmount, 0, 40, "stereo_wide_amount", defaultStereoWideAmount, &cfg.StereoWideAmount, &warnings)

	if raw.ShowFooterHints != nil {
		cfg.ShowFooterHints = *raw.ShowFooterHints
	}

	if raw.QueueFollowNowPlaying != nil {
		cfg.QueueFollowNowPlaying = *raw.QueueFollowNowPlaying
	}

	rm := raw.Mixes
	validateIntRange(rm.RediscoverMinPlays, 1, 1000, "mixes.rediscover_min_plays", defaultMixRediscoverMinPlays, &cfg.Mixes.RediscoverMinPlays, &warnings)
	validateIntRange(rm.RediscoverAgeDays, 7, 365, "mixes.rediscover_age_days", defaultMixRediscoverAgeDays, &cfg.Mixes.RediscoverAgeDays, &warnings)
	validateIntRange(rm.HiddenGemsMaxPlays, 0, 50, "mixes.hidden_gems_max_plays", defaultMixHiddenGemsMaxPlays, &cfg.Mixes.HiddenGemsMaxPlays, &warnings)
	validateIntRange(rm.HiddenGemsTopArtists, 1, 100, "mixes.hidden_gems_top_artists", defaultMixHiddenGemsTopArtists, &cfg.Mixes.HiddenGemsTopArtists, &warnings)
	validateIntRange(rm.HiddenGemsTopGenres, 1, 100, "mixes.hidden_gems_top_genres", defaultMixHiddenGemsTopGenres, &cfg.Mixes.HiddenGemsTopGenres, &warnings)
	validateIntRange(rm.OnRepeatMinPlays, 1, 100, "mixes.on_repeat_min_plays", defaultMixOnRepeatMinPlays, &cfg.Mixes.OnRepeatMinPlays, &warnings)
	validateIntRange(rm.OnRepeatWindowDays, 1, 90, "mixes.on_repeat_window_days", defaultMixOnRepeatWindowDays, &cfg.Mixes.OnRepeatWindowDays, &warnings)
	validateIntRange(rm.DeepCutsMinPlays, 1, 100, "mixes.deep_cuts_min_plays", defaultMixDeepCutsMinPlays, &cfg.Mixes.DeepCutsMinPlays, &warnings)
	validateIntRange(rm.DeepCutsMaxPlays, 1, 1000, "mixes.deep_cuts_max_plays", defaultMixDeepCutsMaxPlays, &cfg.Mixes.DeepCutsMaxPlays, &warnings)
	validateFloatRange(rm.DeepCutsMinCompletion, 0.0, 1.0, "mixes.deep_cuts_min_completion", defaultMixDeepCutsMinCompletion, &cfg.Mixes.DeepCutsMinCompletion, &warnings)
	validateIntRange(rm.SufficientHistoryCount, 1, 10000, "mixes.sufficient_history_count", defaultMixSufficientHistory, &cfg.Mixes.SufficientHistoryCount, &warnings)

	return cfg, warnings
}

// validateIntRange checks an optional int pointer against [lo, hi].
// When the value is in range it is written to *dst; when out of range a
// warning is logged and appended to *warnings; when ptr is nil the default
// is kept silently.
func validateIntRange(ptr *int, lo, hi int, field string, dflt int, dst *int, warnings *[]string) {
	if ptr == nil {
		return
	}
	v := *ptr
	if v >= lo && v <= hi {
		*dst = v
		return
	}
	slog.Warn("invalid config value, using default", "field", field, "value", v, "default", dflt)
	*warnings = append(*warnings, fmt.Sprintf("%s %d is out of range (%d-%d), using %d", field, v, lo, hi, dflt))
}

// validateFloatRange is the float64 analogue of validateIntRange.
func validateFloatRange(ptr *float64, lo, hi float64, field string, dflt float64, dst *float64, warnings *[]string) {
	if ptr == nil {
		return
	}
	v := *ptr
	if v >= lo && v <= hi {
		*dst = v
		return
	}
	slog.Warn("invalid config value, using default", "field", field, "value", v, "default", dflt)
	*warnings = append(*warnings, fmt.Sprintf("%s %.1f is out of range (%.0f-%.0f), using %.1f", field, v, lo, hi, dflt))
}
