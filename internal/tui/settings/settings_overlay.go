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
	tabLibrary   Tab = iota // directory management
	TabPlayback             // playback settings
	TabEqualizer            // EQ settings
	TabDisplay              // display settings
	tabCount
)

// librarySubView identifies which sub-view is active in the Library tab.
type librarySubView int

const (
	libSubViewDirs     librarySubView = iota // directory management
	libSubViewExcluded                       // excluded path patterns
)

// Field describes a single adjustable setting row.
type Field struct {
	label        string
	Value        int
	defaultValue int
	min          int
	max          int
	step         int
	unit         string
}

// Action is the result of a key press in the settings overlay.
type Action int

const (
	actionNone       Action = iota
	ActionSave              // ctrl+s was pressed
	ActionCancel            // esc was pressed
	ActionEQLive            // live EQ preview on band/toggle change
	ActionEQSave            // custom preset saved
	ActionEQDelete          // custom preset deleted
	ActionStereoLive        // live stereo wide preview on toggle/amount change
	ActionThemeLive         // live theme preview
)

// SettingsOverlay is a centered overlay for editing runtime-tunable config values.
type SettingsOverlay struct {
	Tab          Tab
	cursor       int
	Fields       [tabCount][]Field
	EQPresets    []config.EQPresetDef
	EQNameInput  *textinput.Model // non-nil when prompting for preset name
	EQConfirmDel bool             // true when showing delete confirmation
	dirty        bool
	primaryColor string
	PrimaryInput *textinput.Model
	primaryError string

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
		{label: "Volume step", Value: cfg.VolumeIncrement, defaultValue: 5, min: 1, max: 25, step: 1, unit: "%"},
		{label: "Seek short", Value: int(cfg.SeekOffsetShort), defaultValue: 5, min: 1, max: 60, step: 1, unit: "s"},
		{label: "Seek long", Value: int(cfg.SeekOffsetLong), defaultValue: 30, min: 5, max: 300, step: 5, unit: "s"},
		{label: "Default volume", Value: currentVolume, defaultValue: 100, min: 0, max: 100, step: 5, unit: "%"},
		{label: "Normalize", Value: boolToInt(cfg.NormalizeVolume), defaultValue: 0, min: 0, max: 1, step: 1, unit: ""},
		{label: "Gapless", Value: boolToInt(cfg.GaplessAudio), defaultValue: 1, min: 0, max: 1, step: 1, unit: ""},
		{label: "Stereo wide", Value: boolToInt(cfg.StereoWideEnabled), defaultValue: 0, min: 0, max: 1, step: 1, unit: ""},
		{label: "Width", Value: cfg.StereoWideAmount, defaultValue: 10, min: 0, max: 40, step: 1, unit: "tenth"},
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
		{label: "Enabled", Value: boolToInt(cfg.EQEnabled), defaultValue: 0, min: 0, max: 1, step: 1, unit: ""},
		{label: "Preset", Value: presetIdx, defaultValue: 0, min: 0, max: len(eqPresets) - 1, step: 1, unit: "preset"},
		{label: "31 Hz", Value: cfg.EQBands[0], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "62 Hz", Value: cfg.EQBands[1], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "125 Hz", Value: cfg.EQBands[2], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "250 Hz", Value: cfg.EQBands[3], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "500 Hz", Value: cfg.EQBands[4], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "1 kHz", Value: cfg.EQBands[5], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "2 kHz", Value: cfg.EQBands[6], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "4 kHz", Value: cfg.EQBands[7], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "8 kHz", Value: cfg.EQBands[8], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
		{label: "16 kHz", Value: cfg.EQBands[9], defaultValue: 0, min: -12, max: 12, step: 1, unit: "dB"},
	}

	o.initDisplay(cfg)

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

// keepFractional returns base while the whole-second field still shows it,
// and the shown value once the user has changed it. The seek fields hold whole
// seconds, so writing them back unconditionally would truncate a configured
// 2.5 to 2 whenever any unrelated setting was saved.
func keepFractional(base float64, shown int) float64 {
	if int(base) == shown {
		return base
	}
	return float64(shown)
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
	cfg.SeekOffsetShort = keepFractional(base.SeekOffsetShort, pb[1].Value)
	cfg.SeekOffsetLong = keepFractional(base.SeekOffsetLong, pb[2].Value)
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
	cfg.Theme, cfg.PrimaryColor = o.ThemeState()
	return cfg
}
