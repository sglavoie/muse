package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestNewSettingsOverlayInitializesFromConfig(t *testing.T) {
	cfg := config.Defaults()
	currentVolume := 75
	o := NewSettingsOverlay(cfg, currentVolume, cfg.MusicDirectories)

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
	if pb[3].Value != currentVolume {
		t.Errorf("DefaultVolume: expected %d, got %d", currentVolume, pb[3].Value)
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

func TestNewSettingsOverlayUsesCurrentVolume(t *testing.T) {
	cfg := config.Defaults()
	cfg.DefaultVolume = 100
	currentVolume := 75
	o := NewSettingsOverlay(cfg, currentVolume, cfg.MusicDirectories)
	pb := o.fields[settingsTabPlayback]
	if pb[3].Value != 75 {
		t.Errorf("expected Default volume field to show currentVolume 75, got %d", pb[3].Value)
	}
}

func TestNewSettingsOverlayDefaultsTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	if o.tab != settingsTabLibrary {
		t.Errorf("expected initial tab=settingsTabLibrary, got %v", o.tab)
	}
	if o.cursor != 0 {
		t.Errorf("expected initial cursor=0, got %d", o.cursor)
	}
}

func TestSettingsTabSwitchForward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabPlayback {
		t.Errorf("']': expected settingsTabPlayback, got %v", o.tab)
	}
	if o.cursor != 0 {
		t.Errorf("']': expected cursor reset to 0, got %d", o.cursor)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabEqualizer {
		t.Errorf("']' again: expected settingsTabEqualizer, got %v", o.tab)
	}
}

func TestSettingsTabSwitchBackward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if o.tab != settingsTabDisplay {
		t.Errorf("'[' from Library: expected wrap to settingsTabDisplay, got %v", o.tab)
	}
}

func TestSettingsTabSwitchWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabLibrary {
		t.Errorf("']' four times: expected wrap back to settingsTabLibrary, got %v", o.tab)
	}
}

func TestSettingsTabSwitchResetsCursor(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if o.cursor != 1 {
		t.Fatalf("precondition: expected cursor=1, got %d", o.cursor)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.cursor != 0 {
		t.Errorf("tab switch: expected cursor reset to 0, got %d", o.cursor)
	}
}

func TestSettingsCursorDownWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	n := len(o.fields[o.tab])
	for range n {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 0 {
		t.Errorf("j*%d: expected wrap to cursor=0, got %d", n, o.cursor)
	}
}

func TestSettingsCursorUpWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	n := len(o.fields[settingsTabPlayback])
	if o.cursor != n-1 {
		t.Errorf("k from 0: expected wrap to cursor=%d, got %d", n-1, o.cursor)
	}
}

func TestSettingsCursorArrowKeys(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if o.cursor != 1 {
		t.Errorf("down: expected cursor=1, got %d", o.cursor)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if o.cursor != 0 {
		t.Errorf("up: expected cursor=0, got %d", o.cursor)
	}
}

func TestSettingsIncrementValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	initial := o.fields[settingsTabPlayback][0].Value
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	got := o.fields[settingsTabPlayback][0].Value
	if got != initial+1 {
		t.Errorf("l: expected value %d, got %d", initial+1, got)
	}
	if !o.dirty {
		t.Error("l: expected dirty=true after increment")
	}
}

func TestSettingsDecrementValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	initial := o.fields[settingsTabPlayback][0].Value
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	got := o.fields[settingsTabPlayback][0].Value
	if got != initial-1 {
		t.Errorf("h: expected value %d, got %d", initial-1, got)
	}
}

func TestSettingsIncrementClampsAtMax(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	f := &o.fields[settingsTabPlayback][0]
	f.Value = f.Max
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if o.fields[settingsTabPlayback][0].Value != f.Max {
		t.Errorf("l at max: expected value clamped to %d, got %d", f.Max, o.fields[settingsTabPlayback][0].Value)
	}
}

func TestSettingsDecrementClampsAtMin(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	f := &o.fields[settingsTabPlayback][0]
	f.Value = f.Min
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if o.fields[settingsTabPlayback][0].Value != f.Min {
		t.Errorf("h at min: expected value clamped to %d, got %d", f.Min, o.fields[settingsTabPlayback][0].Value)
	}
}

func TestSettingsArrowKeysAdjustValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	initial := o.fields[settingsTabPlayback][0].Value
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if o.fields[settingsTabPlayback][0].Value != initial+1 {
		t.Errorf("right: expected value %d, got %d", initial+1, o.fields[settingsTabPlayback][0].Value)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if o.fields[settingsTabPlayback][0].Value != initial {
		t.Errorf("left: expected value restored to %d, got %d", initial, o.fields[settingsTabPlayback][0].Value)
	}
}

func TestSettingsEscReturnsCancel(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != settingsCancel {
		t.Errorf("esc: expected settingsCancel, got %v", action)
	}
}

func TestSettingsCtrlSReturnsSave(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if action != settingsSave {
		t.Errorf("ctrl+s: expected settingsSave, got %v", action)
	}
}

func TestSettingsCtrlCReturnsNone(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if action != settingsNone {
		t.Errorf("ctrl+c: expected settingsNone (fall-through), got %v", action)
	}
}

func TestSettingsToConfigRoundTrip(t *testing.T) {
	base := config.Defaults()
	o := NewSettingsOverlay(base, base.DefaultVolume, base.MusicDirectories)

	o.fields[settingsTabPlayback][0].Value = 10
	o.fields[settingsTabPlayback][1].Value = 15
	o.fields[settingsTabPlayback][2].Value = 60
	o.fields[settingsTabPlayback][3].Value = 80
	o.fields[settingsTabPlayback][4].Value = 1
	o.fields[settingsTabPlayback][5].Value = 0
	o.fields[settingsTabDisplay][0].Value = 8
	o.fields[settingsTabDisplay][1].Value = 4

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
	base.MusicDirectories = []string{"/my/music"}
	base.MPVSocketPath = "/tmp/mpv.sock"

	o := NewSettingsOverlay(base, base.DefaultVolume, base.MusicDirectories)
	got := o.ToConfig(base)

	if len(got.MusicDirectories) == 0 || got.MusicDirectories[0] != "/my/music" {
		t.Errorf("MusicDirectories[0]: expected %q, got %v", "/my/music", got.MusicDirectories)
	}
	if got.MPVSocketPath != base.MPVSocketPath {
		t.Errorf("MPVSocketPath: expected %q, got %q", base.MPVSocketPath, got.MPVSocketPath)
	}
}

func TestSettingsRenderContainsTitle(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := o.Render(80, 24, s)
	stripped := ansi.Strip(out)
	if !strings.Contains(stripped, "Settings") {
		t.Errorf("Render: expected title 'Settings', got:\n%s", stripped)
	}
}

func TestSettingsRenderContainsTabNames(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, name := range []string{"Playback", "Equalizer", "Display"} {
		if !strings.Contains(out, name) {
			t.Errorf("Render: expected tab name %q in output", name)
		}
	}
}

func TestSettingsRenderContainsFieldLabels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, label := range []string{"Volume step", "Seek short", "Seek long", "Default volume", "Normalize", "Gapless"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render: expected field label %q in output", label)
		}
	}
}

func TestSettingsRenderContainsBorderChars(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, ch := range []string{cornerTL, cornerBL, borderH, borderV} {
		if !strings.Contains(out, ch) {
			t.Errorf("Render: expected border char %q in output", ch)
		}
	}
}

func TestSettingsRenderContainsHint(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "ctrl+s save") {
		t.Errorf("Render: expected hint 'ctrl+s save' in output, got:\n%s", out)
	}
}

func TestSettingsRenderDisplayTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabDisplay
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "Notification") {
		t.Errorf("Render Display tab: expected 'Notification' field, got:\n%s", out)
	}
}

func TestStereoWideFieldsInitializeFromConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
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
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
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
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	got := o.ToConfig(cfg)
	if !got.StereoWideEnabled {
		t.Error("ToConfig: expected StereoWideEnabled=true")
	}
	if got.StereoWideAmount != 30 {
		t.Errorf("ToConfig: expected StereoWideAmount=30, got %d", got.StereoWideAmount)
	}
}

func TestStereoWideToggleReturnsLiveAction(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabPlayback
	for range 6 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 6 {
		t.Fatalf("precondition: expected cursor=6, got %d", o.cursor)
	}
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != settingsStereoLive {
		t.Errorf("stereo wide toggle: expected settingsStereoLive, got %v", action)
	}
}

func TestStereoWideAmountAdjustReturnsLiveAction(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabPlayback
	for range 7 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 7 {
		t.Fatalf("precondition: expected cursor=7, got %d", o.cursor)
	}
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if action != settingsStereoLive {
		t.Errorf("stereo wide amount: expected settingsStereoLive, got %v", action)
	}
}

func TestStereoWideRenderShowsTenthUnit(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabPlayback
	for range 7 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	if !strings.Contains(out, "2.0x") {
		t.Errorf("Render: expected '2.0x' for stereo amount=20, got:\n%s", out)
	}
}
