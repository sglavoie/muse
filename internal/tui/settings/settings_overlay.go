package settings

import (
	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/config"
)

func NewSettingsInput() *textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Focus()
	return &ti
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Tab identifies the active tab in the settings overlay.
type Tab int

const (
	TabLibrary   Tab = iota // directory management
	TabPlayback             // playback settings
	TabEqualizer            // EQ settings
	TabDisplay              // display settings
	TabCount
)

// librarySubView identifies which sub-view is active in the Library tab.
type librarySubView int

const (
	libSubViewDirs     librarySubView = iota // directory management
	libSubViewExcluded                       // excluded path patterns
)

// Field describes a single adjustable setting row.
type Field struct {
	Label   string
	Value   int
	Default int
	Min     int
	Max     int
	Step    int
	Unit    string
}

// Action is the result of a key press in the settings overlay.
type Action int

const (
	ActionNone       Action = iota
	ActionSave              // ctrl+s was pressed
	ActionCancel            // esc was pressed
	ActionEQLive            // live EQ preview on band/toggle change
	ActionEQSave            // custom preset saved
	ActionEQDelete          // custom preset deleted
	ActionStereoLive        // live stereo wide preview on toggle/amount change
)

// SettingsOverlay is a centered overlay for editing runtime-tunable config values.
type SettingsOverlay struct {
	Tab          Tab
	cursor       int
	Fields       [TabCount][]Field
	EQPresets    []config.EQPresetDef
	EQNameInput  *textinput.Model // non-nil when prompting for preset name
	EQConfirmDel bool             // true when showing delete confirmation
	dirty        bool

	// Library tab state.
	libSubView     librarySubView
	LibraryDirs    []string
	libCursor      int
	LibDirInput    *textinput.Model // non-nil when adding a directory
	libConfirmDel  bool             // true when showing directory delete confirmation
	ExcludedPaths  []string
	exclCursor     int
	ExclPathInput  *textinput.Model // non-nil when adding an excluded path pattern
	exclConfirmDel bool             // true when showing delete confirmation for excluded paths
}

// eqCustomSentinelName is the display name of the transient "not a saved preset" state.
const eqCustomSentinelName = "(Custom)"

// NewSettingsOverlay creates a SettingsOverlay pre-populated from cfg.
func NewSettingsOverlay(cfg config.Config, currentVolume int, musicDirs []string) SettingsOverlay {
	o := SettingsOverlay{}

	// Copy library dirs.
	o.LibraryDirs = make([]string, len(musicDirs))
	copy(o.LibraryDirs, musicDirs)

	// Copy excluded paths.
	o.ExcludedPaths = make([]string, len(cfg.ExcludedPaths))
	copy(o.ExcludedPaths, cfg.ExcludedPaths)

	o.Fields[TabPlayback] = []Field{
		{Label: "Volume step", Value: cfg.VolumeIncrement, Default: 5, Min: 1, Max: 25, Step: 1, Unit: "%"},
		{Label: "Seek short", Value: int(cfg.SeekOffsetShort), Default: 5, Min: 1, Max: 60, Step: 1, Unit: "s"},
		{Label: "Seek long", Value: int(cfg.SeekOffsetLong), Default: 30, Min: 5, Max: 300, Step: 5, Unit: "s"},
		{Label: "Default volume", Value: currentVolume, Default: 100, Min: 0, Max: 100, Step: 5, Unit: "%"},
		{Label: "Normalize", Value: boolToInt(cfg.NormalizeVolume), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Gapless", Value: boolToInt(cfg.GaplessAudio), Default: 1, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Stereo wide", Value: boolToInt(cfg.StereoWideEnabled), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Width", Value: cfg.StereoWideAmount, Default: 10, Min: 0, Max: 40, Step: 1, Unit: "tenth"},
	}

	// Build the preset list: built-ins + custom presets + "(Custom)" sentinel.
	eqPresets := make([]config.EQPresetDef, 0, len(config.BuiltinEQPresets)+len(cfg.EQCustomPresets)+1)
	eqPresets = append(eqPresets, config.BuiltinEQPresets...)
	for _, cp := range cfg.EQCustomPresets {
		eqPresets = append(eqPresets, config.EQPresetDef{Name: cp.Name, Key: cp.Key, Bands: cp.Bands})
	}
	eqPresets = append(eqPresets, config.EQPresetDef{Name: eqCustomSentinelName, Key: "custom"})
	o.EQPresets = eqPresets

	presetIdx := config.EQPresetIndex(cfg.EQPreset, eqPresets)
	if presetIdx < 0 {
		presetIdx = len(eqPresets) - 1 // fall back to "(Custom)" sentinel
	}

	o.Fields[TabEqualizer] = []Field{
		{Label: "Enabled", Value: boolToInt(cfg.EQEnabled), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Preset", Value: presetIdx, Default: 0, Min: 0, Max: len(eqPresets) - 1, Step: 1, Unit: "preset"},
		{Label: "31 Hz", Value: cfg.EQBands[0], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "62 Hz", Value: cfg.EQBands[1], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "125 Hz", Value: cfg.EQBands[2], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "250 Hz", Value: cfg.EQBands[3], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "500 Hz", Value: cfg.EQBands[4], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "1 kHz", Value: cfg.EQBands[5], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "2 kHz", Value: cfg.EQBands[6], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "4 kHz", Value: cfg.EQBands[7], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "8 kHz", Value: cfg.EQBands[8], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "16 kHz", Value: cfg.EQBands[9], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
	}

	o.Fields[TabDisplay] = []Field{
		{Label: "Notification", Value: cfg.TransientDurationSecs, Default: 5, Min: 1, Max: 30, Step: 1, Unit: "s"},
		{Label: "Vol feedback", Value: cfg.VolumeFeedbackDurationSecs, Default: 2, Min: 1, Max: 10, Step: 1, Unit: "s"},
		{Label: "Track notify", Value: boolToInt(cfg.NotifyOnTrackChange), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
	}
	return o
}

// EQState returns the current EQ enabled flag and band values from the overlay
// without triggering a full ToConfig merge. Used for live preview.
func (o SettingsOverlay) EQState() (enabled bool, bands [10]int) {
	eq := o.Fields[TabEqualizer]
	enabled = eq[0].Value != 0
	for i := range 10 {
		bands[i] = eq[i+2].Value
	}
	return
}

// StereoWideState returns the current stereo wide enabled flag and amount
// from the overlay without triggering a full ToConfig merge. Used for live preview.
func (o SettingsOverlay) StereoWideState() (enabled bool, amount int) {
	pb := o.Fields[TabPlayback]
	enabled = pb[6].Value != 0
	amount = pb[7].Value
	return
}

// ToConfig merges the overlay's field values back into a copy of base.
func (o SettingsOverlay) ToConfig(base config.Config) config.Config {
	cfg := base

	// Library dirs.
	if len(o.LibraryDirs) > 0 {
		cfg.MusicDirectories = make([]string, len(o.LibraryDirs))
		copy(cfg.MusicDirectories, o.LibraryDirs)
		cfg.MusicDirectory = cfg.MusicDirectories[0]
	}

	// Excluded paths.
	cfg.ExcludedPaths = make([]string, len(o.ExcludedPaths))
	copy(cfg.ExcludedPaths, o.ExcludedPaths)

	pb := o.Fields[TabPlayback]
	cfg.VolumeIncrement = pb[0].Value
	cfg.SeekOffsetShort = float64(pb[1].Value)
	cfg.SeekOffsetLong = float64(pb[2].Value)
	cfg.DefaultVolume = pb[3].Value
	cfg.NormalizeVolume = pb[4].Value != 0
	cfg.GaplessAudio = pb[5].Value != 0
	cfg.StereoWideEnabled = pb[6].Value != 0
	cfg.StereoWideAmount = pb[7].Value

	eq := o.Fields[TabEqualizer]
	cfg.EQEnabled = eq[0].Value != 0
	if eq[1].Value >= 0 && eq[1].Value < len(o.EQPresets) {
		cfg.EQPreset = o.EQPresets[eq[1].Value].Key
	} else {
		cfg.EQPreset = "custom"
	}
	for i := range 10 {
		cfg.EQBands[i] = eq[i+2].Value
	}
	cfg.EQCustomPresets = nil
	for _, p := range o.EQPresets {
		if !p.Builtin && p.Key != "custom" {
			cfg.EQCustomPresets = append(cfg.EQCustomPresets, config.EQCustomPreset{
				Name: p.Name, Key: p.Key, Bands: p.Bands,
			})
		}
	}

	disp := o.Fields[TabDisplay]
	cfg.TransientDurationSecs = disp[0].Value
	cfg.VolumeFeedbackDurationSecs = disp[1].Value
	cfg.NotifyOnTrackChange = disp[2].Value != 0
	return cfg
}
