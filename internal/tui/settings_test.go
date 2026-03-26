package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// --- NewSettingsOverlay ---

func TestNewSettingsOverlayInitializesFromConfig(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg)

	pb := o.fields[settingsTabPlayback]
	if len(pb) != 8 {
		t.Fatalf("expected 8 playback fields, got %d", len(pb))
	}
	if pb[0].Value != cfg.VolumeIncrement {
		t.Errorf("VolumeIncrement: expected %d, got %d", cfg.VolumeIncrement, pb[0].Value)
	}
	if pb[1].Value != int(cfg.SeekOffsetShort) {
		t.Errorf("SeekOffsetShort: expected %d, got %d", int(cfg.SeekOffsetShort), pb[1].Value)
	}
	if pb[2].Value != int(cfg.SeekOffsetLong) {
		t.Errorf("SeekOffsetLong: expected %d, got %d", int(cfg.SeekOffsetLong), pb[2].Value)
	}
	if pb[3].Value != cfg.DefaultVolume {
		t.Errorf("DefaultVolume: expected %d, got %d", cfg.DefaultVolume, pb[3].Value)
	}
	if pb[4].Value != boolToInt(cfg.NormalizeVolume) {
		t.Errorf("NormalizeVolume: expected %d, got %d", boolToInt(cfg.NormalizeVolume), pb[4].Value)
	}
	if pb[5].Label != "Gapless" {
		t.Errorf("Gapless label: expected %q, got %q", "Gapless", pb[5].Label)
	}
	if pb[5].Default != 1 {
		t.Errorf("Gapless Default: expected 1, got %d", pb[5].Default)
	}
	if pb[5].Min != 0 || pb[5].Max != 1 {
		t.Errorf("Gapless Min/Max: expected 0/1, got %d/%d", pb[5].Min, pb[5].Max)
	}
	if pb[5].Value != boolToInt(cfg.GaplessAudio) {
		t.Errorf("GaplessAudio: expected %d, got %d", boolToInt(cfg.GaplessAudio), pb[5].Value)
	}

	disp := o.fields[settingsTabDisplay]
	if len(disp) != 2 {
		t.Fatalf("expected 2 display fields, got %d", len(disp))
	}
	if disp[0].Value != cfg.TransientDurationSecs {
		t.Errorf("TransientDurationSecs: expected %d, got %d", cfg.TransientDurationSecs, disp[0].Value)
	}
	if disp[1].Value != cfg.VolumeFeedbackDurationSecs {
		t.Errorf("VolumeFeedbackDurationSecs: expected %d, got %d", cfg.VolumeFeedbackDurationSecs, disp[1].Value)
	}
}

func TestNewSettingsOverlayDefaultsTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	if o.tab != settingsTabPlayback {
		t.Errorf("expected initial tab=settingsTabPlayback, got %v", o.tab)
	}
	if o.cursor != 0 {
		t.Errorf("expected initial cursor=0, got %d", o.cursor)
	}
}

// --- Tab switching ---

func TestSettingsTabSwitchForward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Playback -> Equalizer
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabEqualizer {
		t.Errorf("']': expected settingsTabEqualizer, got %v", o.tab)
	}
	if o.cursor != 0 {
		t.Errorf("']': expected cursor reset to 0, got %d", o.cursor)
	}
	// Equalizer -> Display
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabDisplay {
		t.Errorf("']' again: expected settingsTabDisplay, got %v", o.tab)
	}
}

func TestSettingsTabSwitchBackward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Start at Playback, wrap backward to Display (last tab).
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if o.tab != settingsTabDisplay {
		t.Errorf("'[' from Playback: expected wrap to settingsTabDisplay, got %v", o.tab)
	}
}

func TestSettingsTabSwitchWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Forward three times wraps back to Playback (3 tabs: Playback, Equalizer, Display).
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabPlayback {
		t.Errorf("']' three times: expected wrap back to settingsTabPlayback, got %v", o.tab)
	}
}

func TestSettingsTabSwitchResetsCursor(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Move cursor down first.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if o.cursor != 1 {
		t.Fatalf("precondition: expected cursor=1, got %d", o.cursor)
	}
	// Switch tab — cursor must reset.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.cursor != 0 {
		t.Errorf("tab switch: expected cursor reset to 0, got %d", o.cursor)
	}
}

// --- Cursor movement ---

func TestSettingsCursorDownWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	n := len(o.fields[o.tab])
	for range n {
		o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 0 {
		t.Errorf("j*%d: expected wrap to cursor=0, got %d", n, o.cursor)
	}
}

func TestSettingsCursorUpWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	n := len(o.fields[o.tab])
	if o.cursor != n-1 {
		t.Errorf("k from 0: expected wrap to cursor=%d, got %d", n-1, o.cursor)
	}
}

func TestSettingsCursorArrowKeys(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if o.cursor != 1 {
		t.Errorf("down: expected cursor=1, got %d", o.cursor)
	}
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if o.cursor != 0 {
		t.Errorf("up: expected cursor=0, got %d", o.cursor)
	}
}

// --- Value adjustment ---

func TestSettingsIncrementValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	initial := o.fields[settingsTabPlayback][0].Value
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	got := o.fields[settingsTabPlayback][0].Value
	if got != initial+1 {
		t.Errorf("l: expected value %d, got %d", initial+1, got)
	}
	if !o.dirty {
		t.Error("l: expected dirty=true after increment")
	}
}

func TestSettingsDecrementValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	initial := o.fields[settingsTabPlayback][0].Value
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	got := o.fields[settingsTabPlayback][0].Value
	if got != initial-1 {
		t.Errorf("h: expected value %d, got %d", initial-1, got)
	}
}

func TestSettingsIncrementClampsAtMax(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	f := &o.fields[settingsTabPlayback][0]
	f.Value = f.Max
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if o.fields[settingsTabPlayback][0].Value != f.Max {
		t.Errorf("l at max: expected value clamped to %d, got %d", f.Max, o.fields[settingsTabPlayback][0].Value)
	}
}

func TestSettingsDecrementClampsAtMin(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	f := &o.fields[settingsTabPlayback][0]
	f.Value = f.Min
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if o.fields[settingsTabPlayback][0].Value != f.Min {
		t.Errorf("h at min: expected value clamped to %d, got %d", f.Min, o.fields[settingsTabPlayback][0].Value)
	}
}

func TestSettingsArrowKeysAdjustValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	initial := o.fields[settingsTabPlayback][0].Value
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if o.fields[settingsTabPlayback][0].Value != initial+1 {
		t.Errorf("right: expected value %d, got %d", initial+1, o.fields[settingsTabPlayback][0].Value)
	}
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if o.fields[settingsTabPlayback][0].Value != initial {
		t.Errorf("left: expected value restored to %d, got %d", initial, o.fields[settingsTabPlayback][0].Value)
	}
}

// --- Actions ---

func TestSettingsEscReturnsCancel(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	_, action := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != settingsCancel {
		t.Errorf("esc: expected settingsCancel, got %v", action)
	}
}

func TestSettingsCtrlSReturnsSave(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	_, action := o.HandleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if action != settingsSave {
		t.Errorf("ctrl+s: expected settingsSave, got %v", action)
	}
}

func TestSettingsCtrlCReturnsNone(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	_, action := o.HandleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if action != settingsNone {
		t.Errorf("ctrl+c: expected settingsNone (fall-through), got %v", action)
	}
}

// --- ToConfig ---

func TestSettingsToConfigRoundTrip(t *testing.T) {
	base := config.Defaults()
	o := NewSettingsOverlay(base)

	// Modify a field directly.
	o.fields[settingsTabPlayback][0].Value = 10 // VolumeIncrement
	o.fields[settingsTabPlayback][1].Value = 15 // SeekOffsetShort
	o.fields[settingsTabPlayback][2].Value = 60 // SeekOffsetLong
	o.fields[settingsTabPlayback][3].Value = 80 // DefaultVolume
	o.fields[settingsTabPlayback][4].Value = 1  // NormalizeVolume
	o.fields[settingsTabPlayback][5].Value = 0  // GaplessAudio (off)
	o.fields[settingsTabDisplay][0].Value = 8   // TransientDurationSecs
	o.fields[settingsTabDisplay][1].Value = 4   // VolumeFeedbackDurationSecs

	got := o.ToConfig(base)

	if got.VolumeIncrement != 10 {
		t.Errorf("VolumeIncrement: expected 10, got %d", got.VolumeIncrement)
	}
	if got.SeekOffsetShort != 15.0 {
		t.Errorf("SeekOffsetShort: expected 15.0, got %f", got.SeekOffsetShort)
	}
	if got.SeekOffsetLong != 60.0 {
		t.Errorf("SeekOffsetLong: expected 60.0, got %f", got.SeekOffsetLong)
	}
	if got.DefaultVolume != 80 {
		t.Errorf("DefaultVolume: expected 80, got %d", got.DefaultVolume)
	}
	if !got.NormalizeVolume {
		t.Errorf("NormalizeVolume: expected true, got false")
	}
	if got.GaplessAudio {
		t.Errorf("GaplessAudio: expected false, got true")
	}
	if got.TransientDurationSecs != 8 {
		t.Errorf("TransientDurationSecs: expected 8, got %d", got.TransientDurationSecs)
	}
	if got.VolumeFeedbackDurationSecs != 4 {
		t.Errorf("VolumeFeedbackDurationSecs: expected 4, got %d", got.VolumeFeedbackDurationSecs)
	}
}

func TestSettingsToConfigPreservesOtherFields(t *testing.T) {
	base := config.Defaults()
	base.MusicDirectory = "/my/music"
	base.MPVSocketPath = "/tmp/mpv.sock"

	o := NewSettingsOverlay(base)
	got := o.ToConfig(base)

	if got.MusicDirectory != base.MusicDirectory {
		t.Errorf("MusicDirectory: expected %q, got %q", base.MusicDirectory, got.MusicDirectory)
	}
	if got.MPVSocketPath != base.MPVSocketPath {
		t.Errorf("MPVSocketPath: expected %q, got %q", base.MPVSocketPath, got.MPVSocketPath)
	}
}

// --- Render ---

func TestSettingsRenderContainsTitle(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	s := styles.NewStyles(true)
	out := o.Render(80, 24, s)
	stripped := ansi.Strip(out)
	if !strings.Contains(stripped, "Settings") {
		t.Errorf("Render: expected title 'Settings', got:\n%s", stripped)
	}
}

func TestSettingsRenderContainsTabNames(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, name := range []string{"Playback", "Equalizer", "Display"} {
		if !strings.Contains(out, name) {
			t.Errorf("Render: expected tab name %q in output", name)
		}
	}
}

func TestSettingsRenderContainsFieldLabels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, label := range []string{"Volume step", "Seek short", "Seek long", "Default volume", "Normalize", "Gapless"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render: expected field label %q in output", label)
		}
	}
}

func TestSettingsRenderContainsBorderChars(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, ch := range []string{cornerTL, cornerBL, borderH, borderV} {
		if !strings.Contains(out, ch) {
			t.Errorf("Render: expected border char %q in output", ch)
		}
	}
}

func TestSettingsRenderContainsHint(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "ctrl+s save") {
		t.Errorf("Render: expected hint 'ctrl+s save' in output, got:\n%s", out)
	}
}

func TestSettingsRenderDisplayTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o.tab = settingsTabDisplay
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "Notification") {
		t.Errorf("Render Display tab: expected 'Notification' field, got:\n%s", out)
	}
}

// --- EQ tab ---

func TestEQTabFieldCount(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	eq := o.fields[settingsTabEqualizer]
	// Enabled (1) + Preset (1) + 10 bands = 12
	if len(eq) != 12 {
		t.Fatalf("EQ tab: expected 12 fields, got %d", len(eq))
	}
}

func TestEQTabPresetsPopulated(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Should have all built-in presets + "(Custom)" sentinel.
	want := len(config.BuiltinEQPresets) + 1
	if len(o.eqPresets) != want {
		t.Fatalf("eqPresets: expected %d entries, got %d", want, len(o.eqPresets))
	}
	last := o.eqPresets[len(o.eqPresets)-1]
	if last.Name != eqCustomSentinelName {
		t.Errorf("last eqPreset: expected %q, got %q", eqCustomSentinelName, last.Name)
	}
}

func TestEQPresetFieldMaxMatchesSentinelIndex(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	presetField := o.fields[settingsTabEqualizer][1]
	wantMax := len(o.eqPresets) - 1
	if presetField.Max != wantMax {
		t.Errorf("Preset field Max: expected %d, got %d", wantMax, presetField.Max)
	}
}

func TestEQDefaultPresetIsFlatWhenConfigIsFlat(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg)
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	if o.eqPresets[presetIdx].Key != "flat" {
		t.Errorf("default preset: expected key %q, got %q", "flat", o.eqPresets[presetIdx].Key)
	}
}

func TestEQUnknownPresetFallsBackToCustomSentinel(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "nonexistent_preset"
	o := NewSettingsOverlay(cfg)
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	sentinelIdx := len(o.eqPresets) - 1
	if presetIdx != sentinelIdx {
		t.Errorf("unknown preset: expected sentinel index %d, got %d", sentinelIdx, presetIdx)
	}
}

// switchToEQTab returns a copy of o with the tab set to Equalizer and cursor reset.
func switchToEQTab(o SettingsOverlay) SettingsOverlay {
	o.tab = settingsTabEqualizer
	o.cursor = 0
	return o
}

func TestEQPresetCyclingSnapssBands(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Move cursor to Preset field (index 1).
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if o.cursor != 1 {
		t.Fatalf("precondition: expected cursor=1, got %d", o.cursor)
	}

	// Press 'l' to advance to Bass Boost preset.
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != settingsEQLive {
		t.Errorf("preset cycle: expected settingsEQLive, got %v", action)
	}

	// Verify bands snapped to Bass Boost values.
	bassBoostPreset := config.BuiltinEQPresets[1] // "Bass Boost"
	eq := o.fields[settingsTabEqualizer]
	for i := range 10 {
		if eq[i+2].Value != bassBoostPreset.Bands[i] {
			t.Errorf("band %d: expected %d (Bass Boost), got %d", i, bassBoostPreset.Bands[i], eq[i+2].Value)
		}
	}
}

func TestEQBandAdjustmentSwitchesToCustomSentinel(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Move cursor to first band field (index 2).
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if o.cursor != 2 {
		t.Fatalf("precondition: expected cursor=2, got %d", o.cursor)
	}

	// Press 'l' to increase a band.
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != settingsEQLive {
		t.Errorf("band adjust: expected settingsEQLive, got %v", action)
	}

	// Preset field should now show the "(Custom)" sentinel.
	sentinelIdx := len(o.eqPresets) - 1
	if o.fields[settingsTabEqualizer][1].Value != sentinelIdx {
		t.Errorf("preset after band adjust: expected sentinel index %d, got %d",
			sentinelIdx, o.fields[settingsTabEqualizer][1].Value)
	}
}

func TestEQEnabledToggleReturnsEQLive(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)
	// cursor=0 is the Enabled field.
	_, action := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != settingsEQLive {
		t.Errorf("enabled toggle: expected settingsEQLive, got %v", action)
	}
}

func TestEQToConfigRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQPreset = "rock"
	cfg.EQBands = [10]int{5, 4, 2, 0, -1, 0, 2, 3, 4, 5}
	o := NewSettingsOverlay(cfg)

	got := o.ToConfig(cfg)

	if !got.EQEnabled {
		t.Error("ToConfig: expected EQEnabled=true")
	}
	if got.EQPreset != "rock" {
		t.Errorf("ToConfig: expected EQPreset=%q, got %q", "rock", got.EQPreset)
	}
	for i, v := range got.EQBands {
		if v != cfg.EQBands[i] {
			t.Errorf("ToConfig: band %d: expected %d, got %d", i, cfg.EQBands[i], v)
		}
	}
}

func TestEQToConfigCustomSentinelKeyIsCustom(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Force preset field to sentinel index.
	o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
	got := o.ToConfig(config.Defaults())
	if got.EQPreset != "custom" {
		t.Errorf("ToConfig with sentinel: expected EQPreset=%q, got %q", "custom", got.EQPreset)
	}
}

func TestEQStateHelper(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQBands = [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	o := NewSettingsOverlay(cfg)

	enabled, bands := o.EQState()
	if !enabled {
		t.Error("EQState: expected enabled=true")
	}
	for i, v := range bands {
		if v != cfg.EQBands[i] {
			t.Errorf("EQState: band %d: expected %d, got %d", i, cfg.EQBands[i], v)
		}
	}
}

func TestEQRenderEqualizerTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	for _, label := range []string{"Enabled", "Preset", "31 Hz", "16 kHz"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render EQ tab: expected field label %q in output", label)
		}
	}
}

func TestEQRenderShowsPresetName(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	o := NewSettingsOverlay(cfg)
	o.tab = settingsTabEqualizer
	o.cursor = 1 // Preset field
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Bass Boost") {
		t.Errorf("Render EQ tab: expected preset name 'Bass Boost' in output, got:\n%s", out)
	}
}

// --- Custom Preset CRUD ---

func TestCustomPresetsFromConfigPopulateOverlay(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Preset", Key: "my_preset", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	o := NewSettingsOverlay(cfg)

	// Should have built-ins + 1 custom + sentinel.
	want := len(config.BuiltinEQPresets) + 1 + 1
	if len(o.eqPresets) != want {
		t.Fatalf("eqPresets with custom: expected %d, got %d", want, len(o.eqPresets))
	}
	// Custom preset is just before sentinel.
	sentinel := len(o.eqPresets) - 1
	custom := o.eqPresets[sentinel-1]
	if custom.Key != "my_preset" {
		t.Errorf("custom preset key: expected %q, got %q", "my_preset", custom.Key)
	}
	if custom.Builtin {
		t.Error("custom preset should have Builtin=false")
	}
}

func TestEQPresetCyclingIncludesCustomPresets(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{-2, -1, 0, 0, 0, 0, 0, 0, -1, -2}},
	}
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Move cursor to Preset field.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	// Cycle through all built-ins to reach the custom preset.
	for range len(config.BuiltinEQPresets) {
		o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	}

	presetIdx := o.fields[settingsTabEqualizer][1].Value
	if o.eqPresets[presetIdx].Key != "night_mode" {
		t.Errorf("after cycling through built-ins: expected custom preset %q, got %q",
			"night_mode", o.eqPresets[presetIdx].Key)
	}
}

func TestEQSavePresetOpensNameInput(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)

	o, action := o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	if action != settingsNone {
		t.Errorf("S: expected settingsNone, got %v", action)
	}
	if o.eqNameInput == nil {
		t.Error("S: expected eqNameInput to be non-nil")
	}
}

func TestEQSavePresetTypingAndEnter(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	// Open name input.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})

	// Type "Hi".
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'H', Text: "H"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})

	// Press Enter.
	o, action := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != settingsEQSave {
		t.Errorf("Enter after typing: expected settingsEQSave, got %v", action)
	}
	if o.eqNameInput != nil {
		t.Error("Enter: expected eqNameInput to be nil after confirm")
	}
	if len(o.eqPresets) != initialPresetCount+1 {
		t.Errorf("Enter: expected %d presets, got %d", initialPresetCount+1, len(o.eqPresets))
	}
	// New preset is at index len-2 (before sentinel).
	newPreset := o.eqPresets[len(o.eqPresets)-2]
	if newPreset.Name != "Hi" {
		t.Errorf("new preset name: expected %q, got %q", "Hi", newPreset.Name)
	}
	if newPreset.Key != "hi" {
		t.Errorf("new preset key: expected %q, got %q", "hi", newPreset.Key)
	}
}

func TestEQSavePresetEmptyNameNoOp(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	// Open name input and immediately press Enter.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, action := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != settingsNone {
		t.Errorf("Enter with empty name: expected settingsNone, got %v", action)
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Enter with empty name: preset count changed from %d to %d", initialPresetCount, len(o.eqPresets))
	}
}

func TestEQSavePresetEscCancels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'X', Text: "X"})
	o, action := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != settingsNone {
		t.Errorf("Esc in name input: expected settingsNone, got %v", action)
	}
	if o.eqNameInput != nil {
		t.Error("Esc: expected eqNameInput to be nil")
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Esc: preset count changed from %d to %d", initialPresetCount, len(o.eqPresets))
	}
}

func TestEQSavePresetSlugDedup(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Sound", Key: "my_sound", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Save another preset with the same slug.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	for _, r := range "My Sound" {
		o, _ = o.HandleKey(tea.KeyPressMsg{Code: rune(r), Text: string(r)})
	}
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Find the new preset.
	var keys []string
	for _, p := range o.eqPresets {
		if !p.Builtin && p.Key != "custom" {
			keys = append(keys, p.Key)
		}
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 custom preset keys, got %v", keys)
	}
	// One should be "my_sound" and the other "my_sound_2".
	found2 := false
	for _, k := range keys {
		if k == "my_sound_2" {
			found2 = true
		}
	}
	if !found2 {
		t.Errorf("dedup: expected key %q in %v", "my_sound_2", keys)
	}
}

func TestEQDeleteBuiltinPresetIgnored(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Preset field starts on Flat (built-in). Press D — should be ignored.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"}) // cursor to Preset
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsNone {
		t.Errorf("D on built-in: expected settingsNone, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("D on built-in: expected eqConfirmDel=false")
	}
}

func TestEQDeleteSentinelIgnored(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)

	// Set preset to "(Custom)" sentinel.
	o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"}) // cursor to Preset
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsNone {
		t.Errorf("D on sentinel: expected settingsNone, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("D on sentinel: expected eqConfirmDel=false")
	}
}

func TestEQDeleteCustomPresetFlow(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	// Navigate preset to custom preset.
	customIdx := len(config.BuiltinEQPresets)
	o.fields[settingsTabEqualizer][1].Value = customIdx

	// Move cursor to Preset field.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	// Press D to start delete confirmation.
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsNone {
		t.Errorf("D: expected settingsNone, got %v", action)
	}
	if !o.eqConfirmDel {
		t.Error("D: expected eqConfirmDel=true")
	}

	// Confirm with D.
	o, action = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsEQDelete {
		t.Errorf("D confirm: expected settingsEQDelete, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("D confirm: expected eqConfirmDel=false after confirm")
	}
	if len(o.eqPresets) != initialPresetCount-1 {
		t.Errorf("D confirm: expected %d presets, got %d", initialPresetCount-1, len(o.eqPresets))
	}

	// Should fall back to Flat.
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	if o.eqPresets[presetIdx].Key != "flat" {
		t.Errorf("after delete: expected preset=flat, got %q", o.eqPresets[presetIdx].Key)
	}
}

func TestEQDeleteCustomPresetEscCancels(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	customIdx := len(config.BuiltinEQPresets)
	o.fields[settingsTabEqualizer][1].Value = customIdx
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"}) // start confirm
	o, action := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != settingsNone {
		t.Errorf("Esc in confirm: expected settingsNone, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("Esc: expected eqConfirmDel=false")
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Esc: preset count changed from %d to %d", initialPresetCount, len(o.eqPresets))
	}
}

func TestToConfigIncludesCustomPresets(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Sound", Key: "my_sound", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	o := NewSettingsOverlay(cfg)
	got := o.ToConfig(cfg)

	if len(got.EQCustomPresets) != 1 {
		t.Fatalf("ToConfig: expected 1 custom preset, got %d", len(got.EQCustomPresets))
	}
	if got.EQCustomPresets[0].Key != "my_sound" {
		t.Errorf("ToConfig: expected key %q, got %q", "my_sound", got.EQCustomPresets[0].Key)
	}
}

func TestToConfigExcludesBuiltinsAndSentinel(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	got := o.ToConfig(config.Defaults())

	if len(got.EQCustomPresets) != 0 {
		t.Errorf("ToConfig with no custom presets: expected 0, got %d", len(got.EQCustomPresets))
	}
}

func TestEQRenderShowsNameInputPrompt(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o.tab = settingsTabEqualizer
	o.eqNameInput = &textInput{}
	o.eqNameInput.HandleKey("M")
	o.eqNameInput.HandleKey("y")
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Save as:") {
		t.Errorf("Render with name input: expected 'Save as:' in output, got:\n%s", out)
	}
}

func TestEQRenderShowsDeleteConfirmation(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg)
	o.tab = settingsTabEqualizer
	o.cursor = 1                                                           // Preset field
	o.fields[settingsTabEqualizer][1].Value = len(config.BuiltinEQPresets) // custom preset index
	o.eqConfirmDel = true
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Delete") {
		t.Errorf("Render with delete confirm: expected 'Delete' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Night Mode") {
		t.Errorf("Render with delete confirm: expected 'Night Mode' in output, got:\n%s", out)
	}
}

func TestEQHintLineShowsOnEQTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "s save") {
		t.Errorf("EQ tab hint: expected 's save' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "d delete") {
		t.Errorf("EQ tab hint: expected 'd delete' in output, got:\n%s", out)
	}
}

// --- Text input ---

func TestTextInputTyping(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("H")
	ti.HandleKey("i")
	if ti.Value() != "Hi" {
		t.Errorf("typing: expected %q, got %q", "Hi", ti.Value())
	}
}

func TestTextInputBackspace(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("H")
	ti.HandleKey("i")
	ti.HandleKey("backspace")
	if ti.Value() != "H" {
		t.Errorf("backspace: expected %q, got %q", "H", ti.Value())
	}
}

func TestTextInputBackspaceAtStart(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("backspace") // no-op
	if ti.Value() != "" {
		t.Errorf("backspace at start: expected empty, got %q", ti.Value())
	}
}

func TestTextInputLeftRight(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("A")
	ti.HandleKey("B")
	// cursor at end (2)
	ti.HandleKey("left")
	// cursor at 1
	ti.HandleKey("A") // insert A between A and B
	if ti.Value() != "AAB" {
		t.Errorf("left+insert: expected %q, got %q", "AAB", ti.Value())
	}
}

func TestTextInputRenderShowsCursor(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("H")
	ti.HandleKey("i")
	r := ti.Render(40)
	if !strings.Contains(r, "|") {
		t.Errorf("Render: expected cursor '|' in output, got %q", r)
	}
}

func TestTextInputLeftAtStart(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("left") // no-op
	if ti.cursor != 0 {
		t.Errorf("left at start: expected cursor=0, got %d", ti.cursor)
	}
}

func TestTextInputRightAtEnd(t *testing.T) {
	ti := &textInput{}
	ti.HandleKey("A")
	cursorBefore := ti.cursor
	ti.HandleKey("right") // no-op at end
	if ti.cursor != cursorBefore {
		t.Errorf("right at end: expected cursor=%d, got %d", cursorBefore, ti.cursor)
	}
}

// --- Fix 1: h/l at min/max boundary should NOT reset preset to "(Custom)" ---

func TestEQBandAtMinDoesNotResetPreset(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = config.BuiltinEQPresets[1].Bands // Bass Boost bands
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Move cursor to first band (index 2), which is 6 dB for Bass Boost.
	o.cursor = 2
	// Force value to min so h won't change it.
	o.fields[settingsTabEqualizer][2].Value = -12

	presetBefore := o.fields[settingsTabEqualizer][1].Value
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})

	// Value didn't change, so preset should NOT have been touched.
	if o.fields[settingsTabEqualizer][1].Value != presetBefore {
		t.Errorf("h at min: preset changed from %d to %d", presetBefore, o.fields[settingsTabEqualizer][1].Value)
	}
	if action != settingsNone {
		t.Errorf("h at min: expected settingsNone, got %v", action)
	}
}

func TestEQBandAtMaxDoesNotResetPreset(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = config.BuiltinEQPresets[1].Bands
	o := NewSettingsOverlay(cfg)
	o = switchToEQTab(o)

	// Move cursor to first band (index 2) and set to max.
	o.cursor = 2
	o.fields[settingsTabEqualizer][2].Value = 12

	presetBefore := o.fields[settingsTabEqualizer][1].Value
	o, action := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if o.fields[settingsTabEqualizer][1].Value != presetBefore {
		t.Errorf("l at max: preset changed from %d to %d", presetBefore, o.fields[settingsTabEqualizer][1].Value)
	}
	if action != settingsNone {
		t.Errorf("l at max: expected settingsNone, got %v", action)
	}
}

// --- Fix 2: saving preset with all-special-chars name is a no-op ---

func TestEQSavePresetAllSpecialCharsNameNoOp(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	// Open name input and type "!!!".
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, action := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if action != settingsNone {
		t.Errorf("Enter with all-special-chars name: expected settingsNone, got %v", action)
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Enter with all-special-chars name: preset count changed from %d to %d",
			initialPresetCount, len(o.eqPresets))
	}
}

// --- Fix 3: EQ hint line includes j/k fields ---

func TestEQHintLineIncludesJKFields(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "j/k fields") {
		t.Errorf("EQ tab hint: expected 'j/k fields' in output, got:\n%s", out)
	}
}

// --- Stereo Wide ---

func TestStereoWideFieldsInitializeFromConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg)
	pb := o.fields[settingsTabPlayback]
	if pb[6].Value != 1 {
		t.Errorf("StereoWideEnabled field: expected 1, got %d", pb[6].Value)
	}
	if pb[7].Value != 20 {
		t.Errorf("StereoWideAmount field: expected 20, got %d", pb[7].Value)
	}
}

func TestStereoWideStateHelper(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 25
	o := NewSettingsOverlay(cfg)
	enabled, amount := o.StereoWideState()
	if !enabled {
		t.Error("StereoWideState: expected enabled=true")
	}
	if amount != 25 {
		t.Errorf("StereoWideState: expected amount=25, got %d", amount)
	}
}

func TestStereoWideToConfigRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 30
	o := NewSettingsOverlay(cfg)
	got := o.ToConfig(cfg)
	if !got.StereoWideEnabled {
		t.Error("ToConfig: expected StereoWideEnabled=true")
	}
	if got.StereoWideAmount != 30 {
		t.Errorf("ToConfig: expected StereoWideAmount=30, got %d", got.StereoWideAmount)
	}
}

func TestStereoWideToggleReturnsLiveAction(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Navigate to cursor=6 (Stereo wide toggle).
	for range 6 {
		o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 6 {
		t.Fatalf("precondition: expected cursor=6, got %d", o.cursor)
	}
	_, action := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != settingsStereoLive {
		t.Errorf("stereo wide toggle: expected settingsStereoLive, got %v", action)
	}
}

func TestStereoWideAmountAdjustReturnsLiveAction(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideAmount = 20 // room to go up or down
	o := NewSettingsOverlay(cfg)
	// Navigate to cursor=7 (Width).
	for range 7 {
		o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 7 {
		t.Fatalf("precondition: expected cursor=7, got %d", o.cursor)
	}
	_, action := o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if action != settingsStereoLive {
		t.Errorf("stereo wide amount: expected settingsStereoLive, got %v", action)
	}
}

func TestStereoWideRenderShowsTenthUnit(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg)
	// Navigate to Width field so it's visible and active.
	for range 7 {
		o, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "2.0x") {
		t.Errorf("Render: expected '2.0x' for stereo amount=20, got:\n%s", out)
	}
}
