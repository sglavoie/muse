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
	if len(raw.MusicDirectories) > 0 {
		resolved := normalizeMusicDirectories(raw.MusicDirectories)
		if len(resolved) > 0 {
			cfg.MusicDirectories = resolved
			cfg.MusicDirectory = resolved[0]
		} else if raw.MusicDirectory != "" {
			resolved = normalizeMusicDirectories([]string{raw.MusicDirectory})
			if len(resolved) > 0 {
				cfg.MusicDirectories = resolved
				cfg.MusicDirectory = resolved[0]
			}
		}
	} else if raw.MusicDirectory != "" {
		resolved := normalizeMusicDirectories([]string{raw.MusicDirectory})
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
		trimmed := strings.TrimSpace(f)
		if trimmed == "" {
			continue
		}
		cfg.MPVExtraFlags = append(cfg.MPVExtraFlags, trimmed)
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
	case "warning":
		cfg.LogLevel = "warn"
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
		bands, ok := normalizeEQBands(*raw.EQBands)
		if !ok {
			slog.Warn("invalid config value, using default",
				"field", "eq_bands",
				"bands", len(*raw.EQBands),
				"default_bands", len(cfg.EQBands),
			)
			warnings = append(warnings, fmt.Sprintf("eq_bands has %d bands, expected %d; using default", len(*raw.EQBands), len(cfg.EQBands)))
		} else {
			cfg.EQBands = bands
		}
	}

	// Validate and deduplicate custom presets, clamping their bands.
	// Trim whitespace from name/key so a TOML entry like key = "  rock  "
	// cannot silently slip through dedup and produce a key that fails to
	// match any preset lookup later (lookups compare against trimmed keys).
	builtinEQKeys := make(map[string]bool, len(BuiltinEQPresets))
	for _, p := range BuiltinEQPresets {
		builtinEQKeys[p.Key] = true
	}
	seen := make(map[string]bool)
	for _, cp := range raw.EQCustomPresets {
		trimmedKey := strings.TrimSpace(cp.Key)
		trimmedName := strings.TrimSpace(cp.Name)
		if trimmedKey == "" {
			slog.Warn("eq custom preset dropped: empty key", "name", cp.Name)
			warnings = append(warnings, fmt.Sprintf("eq custom preset %q dropped: key is empty", cp.Name))
			continue
		}
		if builtinEQKeys[trimmedKey] {
			slog.Warn("eq custom preset dropped: key conflicts with built-in preset", "key", trimmedKey)
			warnings = append(warnings, fmt.Sprintf("eq custom preset with key %q conflicts with a built-in preset; ignored", trimmedKey))
			continue
		}
		if seen[trimmedKey] {
			slog.Warn("eq custom preset dropped: duplicate key", "key", trimmedKey)
			warnings = append(warnings, fmt.Sprintf("eq custom preset with key %q is duplicated; later occurrence ignored", trimmedKey))
			continue
		}
		clamped := [10]int{}
		if cp.Bands != nil {
			bands, ok := normalizeEQBands(*cp.Bands)
			if !ok {
				slog.Warn("eq custom preset dropped: invalid band count", "key", trimmedKey, "bands", len(*cp.Bands))
				warnings = append(warnings, fmt.Sprintf("eq custom preset with key %q has %d bands, expected %d; ignored", trimmedKey, len(*cp.Bands), len(clamped)))
				continue
			}
			clamped = bands
		}
		// Fall back to the key when the name is missing so the preset is still
		// addressable in the TUI.
		if trimmedName == "" {
			trimmedName = trimmedKey
		}
		seen[trimmedKey] = true
		cfg.EQCustomPresets = append(cfg.EQCustomPresets, EQCustomPreset{
			Name:  trimmedName,
			Key:   trimmedKey,
			Bands: clamped,
		})
	}

	// Validate preset key: must match a known built-in or custom key.
	raw.EQPreset = strings.TrimSpace(raw.EQPreset)
	if raw.EQPreset != "" {
		allPresets := make([]EQPresetDef, 0, len(BuiltinEQPresets)+len(cfg.EQCustomPresets)+1)
		allPresets = append(allPresets, BuiltinEQPresets...)
		for _, cp := range cfg.EQCustomPresets {
			allPresets = append(allPresets, EQPresetDef{Key: cp.Key})
		}
		allPresets = append(allPresets, EQPresetDef{Key: "custom"})
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

	if raw.NotifyOnTrackChange != nil {
		cfg.NotifyOnTrackChange = *raw.NotifyOnTrackChange
	}

	if raw.QueueFollowNowPlaying != nil {
		cfg.QueueFollowNowPlaying = *raw.QueueFollowNowPlaying
	}

	validateTheme(raw, &cfg, &warnings)

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
	if cfg.Mixes.DeepCutsMinPlays > cfg.Mixes.DeepCutsMaxPlays {
		slog.Warn("invalid config value, using defaults",
			"field", "mixes.deep_cuts_min_plays/mixes.deep_cuts_max_plays",
			"min", cfg.Mixes.DeepCutsMinPlays,
			"max", cfg.Mixes.DeepCutsMaxPlays,
			"default_min", defaultMixDeepCutsMinPlays,
			"default_max", defaultMixDeepCutsMaxPlays,
		)
		warnings = append(warnings, fmt.Sprintf(
			"mixes.deep_cuts_min_plays %d is greater than mixes.deep_cuts_max_plays %d, using defaults %d-%d",
			cfg.Mixes.DeepCutsMinPlays,
			cfg.Mixes.DeepCutsMaxPlays,
			defaultMixDeepCutsMinPlays,
			defaultMixDeepCutsMaxPlays,
		))
		cfg.Mixes.DeepCutsMinPlays = defaultMixDeepCutsMinPlays
		cfg.Mixes.DeepCutsMaxPlays = defaultMixDeepCutsMaxPlays
	}

	return cfg, warnings
}

func normalizeEQBands(values []int) ([10]int, bool) {
	var bands [10]int
	if len(values) != len(bands) {
		return bands, false
	}
	for i, v := range values {
		switch {
		case v < -12:
			bands[i] = -12
		case v > 12:
			bands[i] = 12
		default:
			bands[i] = v
		}
	}
	return bands, true
}

func normalizeMusicDirectories(rawDirs []string) []string {
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
	return resolved
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
	*warnings = append(*warnings, fmt.Sprintf("%s %g is out of range (%g-%g), using %g", field, v, lo, hi, dflt))
}
