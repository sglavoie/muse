package config

const configTemplate = `# Paths to your music library roots (supports multiple directories).
music_directories = [%q]

# Directory name patterns to exclude from the music library scan.
# Matches any directory component in a file's path (exact, case-sensitive).
# excluded_paths = ["Podcasts", "Audiobooks"]

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

# Equalizer settings.
# eq_enabled = false
# eq_preset = "flat"
# eq_bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
#
# [[eq_custom_presets]]
# name = "My Preset"
# key = "my_preset"
# bands = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

# Stereo widening settings.
# stereo_wide_enabled = false
# stereo_wide_amount = 10

# Show the context-aware footer hint strip above the status bar (true/false).
# show_footer_hints = true

# Auto-scroll the now-playing track into view in the Queue panel (true/false).
# queue_follow_now_playing = true

# Recommendation mix thresholds.
# [mixes]
# rediscover_min_plays = 10
# rediscover_age_days = 90
# hidden_gems_max_plays = 2
# hidden_gems_top_artists = 20
# hidden_gems_top_genres = 10
# on_repeat_min_plays = 3
# on_repeat_window_days = 7
# deep_cuts_min_plays = 1
# deep_cuts_max_plays = 5
# deep_cuts_min_completion = 0.8
# sufficient_history_count = 20
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
	defaultNormalizeVolume            = false
	defaultGaplessAudio               = true
	defaultEQEnabled                  = false
	defaultEQPreset                   = "flat"
	defaultStereoWideEnabled          = false
	defaultStereoWideAmount           = 10 // 1.0x = passthrough
	defaultShowFooterHints            = true
	defaultQueueFollowNowPlaying      = true

	defaultMixRediscoverMinPlays    = 10
	defaultMixRediscoverAgeDays     = 90
	defaultMixHiddenGemsMaxPlays    = 2
	defaultMixHiddenGemsTopArtists  = 20
	defaultMixHiddenGemsTopGenres   = 10
	defaultMixOnRepeatMinPlays      = 3
	defaultMixOnRepeatWindowDays    = 7
	defaultMixDeepCutsMinPlays      = 1
	defaultMixDeepCutsMaxPlays      = 5
	defaultMixDeepCutsMinCompletion = 0.8
	defaultMixSufficientHistory     = 20
)

// MixesConfig holds thresholds for the recommendation mix queries.
type MixesConfig struct {
	RediscoverMinPlays     int     `toml:"rediscover_min_plays"`
	RediscoverAgeDays      int     `toml:"rediscover_age_days"`
	HiddenGemsMaxPlays     int     `toml:"hidden_gems_max_plays"`
	HiddenGemsTopArtists   int     `toml:"hidden_gems_top_artists"`
	HiddenGemsTopGenres    int     `toml:"hidden_gems_top_genres"`
	OnRepeatMinPlays       int     `toml:"on_repeat_min_plays"`
	OnRepeatWindowDays     int     `toml:"on_repeat_window_days"`
	DeepCutsMinPlays       int     `toml:"deep_cuts_min_plays"`
	DeepCutsMaxPlays       int     `toml:"deep_cuts_max_plays"`
	DeepCutsMinCompletion  float64 `toml:"deep_cuts_min_completion"`
	SufficientHistoryCount int     `toml:"sufficient_history_count"`
}

// rawMixesConfig mirrors MixesConfig with pointer types for optional fields.
type rawMixesConfig struct {
	RediscoverMinPlays     *int     `toml:"rediscover_min_plays"`
	RediscoverAgeDays      *int     `toml:"rediscover_age_days"`
	HiddenGemsMaxPlays     *int     `toml:"hidden_gems_max_plays"`
	HiddenGemsTopArtists   *int     `toml:"hidden_gems_top_artists"`
	HiddenGemsTopGenres    *int     `toml:"hidden_gems_top_genres"`
	OnRepeatMinPlays       *int     `toml:"on_repeat_min_plays"`
	OnRepeatWindowDays     *int     `toml:"on_repeat_window_days"`
	DeepCutsMinPlays       *int     `toml:"deep_cuts_min_plays"`
	DeepCutsMaxPlays       *int     `toml:"deep_cuts_max_plays"`
	DeepCutsMinCompletion  *float64 `toml:"deep_cuts_min_completion"`
	SufficientHistoryCount *int     `toml:"sufficient_history_count"`
}

// Config holds all application configuration values.
type Config struct {
	MusicDirectories           []string         `toml:"music_directories"`
	ExcludedPaths              []string         `toml:"excluded_paths"`
	MusicDirectory             string           `toml:"-"` // legacy compat; always equals MusicDirectories[0]
	MPVSocketPath              string           `toml:"mpv_socket_path"`
	MPVExtraFlags              []string         `toml:"mpv_extra_flags"`
	DefaultVolume              int              `toml:"default_volume"`
	LogLevel                   string           `toml:"log_level"`
	VolumeIncrement            int              `toml:"volume_increment"`
	SeekOffsetShort            float64          `toml:"seek_offset_short"`
	SeekOffsetLong             float64          `toml:"seek_offset_long"`
	TransientDurationSecs      int              `toml:"transient_duration_secs"`
	VolumeFeedbackDurationSecs int              `toml:"volume_feedback_duration_secs"`
	NormalizeVolume            bool             `toml:"normalize_volume"`
	GaplessAudio               bool             `toml:"gapless_audio"`
	EQEnabled                  bool             `toml:"eq_enabled"`
	EQPreset                   string           `toml:"eq_preset"`
	EQBands                    [10]int          `toml:"eq_bands"`
	EQCustomPresets            []EQCustomPreset `toml:"eq_custom_presets"`
	StereoWideEnabled          bool             `toml:"stereo_wide_enabled"`
	StereoWideAmount           int              `toml:"stereo_wide_amount"`
	ShowFooterHints            bool             `toml:"show_footer_hints"`
	QueueFollowNowPlaying      bool             `toml:"queue_follow_now_playing"`
	Mixes                      MixesConfig      `toml:"mixes"`
}

// LoadResult holds the output of a successful Load call.
type LoadResult struct {
	Config   Config
	Created  bool
	Warnings []string
}

// rawConfig mirrors Config but uses pointer types for optional numeric fields
// so that omitted TOML keys can be distinguished from explicit zero values.
type rawConfig struct {
	MusicDirectories           []string            `toml:"music_directories"`
	ExcludedPaths              []string            `toml:"excluded_paths"`
	MusicDirectory             string              `toml:"music_directory"` // legacy single-dir field
	MPVSocketPath              string              `toml:"mpv_socket_path"`
	MPVExtraFlags              []string            `toml:"mpv_extra_flags"`
	DefaultVolume              *int                `toml:"default_volume"`
	LogLevel                   string              `toml:"log_level"`
	VolumeIncrement            *int                `toml:"volume_increment"`
	SeekOffsetShort            *float64            `toml:"seek_offset_short"`
	SeekOffsetLong             *float64            `toml:"seek_offset_long"`
	TransientDurationSecs      *int                `toml:"transient_duration_secs"`
	VolumeFeedbackDurationSecs *int                `toml:"volume_feedback_duration_secs"`
	NormalizeVolume            *bool               `toml:"normalize_volume"`
	GaplessAudio               *bool               `toml:"gapless_audio"`
	EQEnabled                  *bool               `toml:"eq_enabled"`
	EQPreset                   string              `toml:"eq_preset"`
	EQBands                    *[]int              `toml:"eq_bands"`
	EQCustomPresets            []rawEQCustomPreset `toml:"eq_custom_presets"`
	StereoWideEnabled          *bool               `toml:"stereo_wide_enabled"`
	StereoWideAmount           *int                `toml:"stereo_wide_amount"`
	ShowFooterHints            *bool               `toml:"show_footer_hints"`
	QueueFollowNowPlaying      *bool               `toml:"queue_follow_now_playing"`
	Mixes                      rawMixesConfig      `toml:"mixes"`
}

// rawEQCustomPreset mirrors EQCustomPreset with a slice for bands so an
// incorrectly sized user entry can be rejected without failing the whole file.
type rawEQCustomPreset struct {
	Name  string `toml:"name"`
	Key   string `toml:"key"`
	Bands *[]int `toml:"bands"`
}
