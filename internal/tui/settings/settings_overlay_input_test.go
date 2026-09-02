package settings

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

func TestSettingsTabSwitchForward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.Tab != TabPlayback {
		t.Errorf("']': expected settingsTabPlayback, got %v", o.Tab)
	}
	if o.cursor != 0 {
		t.Errorf("']': expected cursor reset to 0, got %d", o.cursor)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.Tab != TabEqualizer {
		t.Errorf("']' again: expected settingsTabEqualizer, got %v", o.Tab)
	}
}

func TestSettingsTabSwitchBackward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if o.Tab != TabDisplay {
		t.Errorf("'[' from Library: expected wrap to settingsTabDisplay, got %v", o.Tab)
	}
}

func TestSettingsTabSwitchWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.Tab != tabLibrary {
		t.Errorf("']' four times: expected wrap back to settingsTabLibrary, got %v", o.Tab)
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
	o.Tab = TabPlayback
	n := len(o.Fields[o.Tab])
	for range n {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 0 {
		t.Errorf("j*%d: expected wrap to cursor=0, got %d", n, o.cursor)
	}
}

func TestSettingsCursorUpWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.Tab = TabPlayback
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	n := len(o.Fields[TabPlayback])
	if o.cursor != n-1 {
		t.Errorf("k from 0: expected wrap to cursor=%d, got %d", n-1, o.cursor)
	}
}

func TestSettingsCursorArrowKeys(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.Tab = TabPlayback
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
	o.Tab = TabPlayback
	initial := o.Fields[TabPlayback][0].Value
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	got := o.Fields[TabPlayback][0].Value
	if got != initial+1 {
		t.Errorf("l: expected value %d, got %d", initial+1, got)
	}
	if !o.dirty {
		t.Error("l: expected dirty=true after increment")
	}
}

func TestSettingsDecrementValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.Tab = TabPlayback
	initial := o.Fields[TabPlayback][0].Value
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	got := o.Fields[TabPlayback][0].Value
	if got != initial-1 {
		t.Errorf("h: expected value %d, got %d", initial-1, got)
	}
}

func TestSettingsIncrementClampsAtMax(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	f := &o.Fields[TabPlayback][0]
	f.Value = f.max
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if o.Fields[TabPlayback][0].Value != f.max {
		t.Errorf("l at max: expected value clamped to %d, got %d", f.max, o.Fields[TabPlayback][0].Value)
	}
}

func TestSettingsDecrementClampsAtMin(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	f := &o.Fields[TabPlayback][0]
	f.Value = f.min
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if o.Fields[TabPlayback][0].Value != f.min {
		t.Errorf("h at min: expected value clamped to %d, got %d", f.min, o.Fields[TabPlayback][0].Value)
	}
}

func TestSettingsArrowKeysAdjustValue(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.Tab = TabPlayback
	initial := o.Fields[TabPlayback][0].Value
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if o.Fields[TabPlayback][0].Value != initial+1 {
		t.Errorf("right: expected value %d, got %d", initial+1, o.Fields[TabPlayback][0].Value)
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if o.Fields[TabPlayback][0].Value != initial {
		t.Errorf("left: expected value restored to %d, got %d", initial, o.Fields[TabPlayback][0].Value)
	}
}

func TestSettingsEscReturnsCancel(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != ActionCancel {
		t.Errorf("esc: expected settingsCancel, got %v", action)
	}
}

func TestSettingsCtrlSReturnsSave(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if action != ActionSave {
		t.Errorf("ctrl+s: expected settingsSave, got %v", action)
	}
}

func TestSettingsCtrlCReturnsNone(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if action != actionNone {
		t.Errorf("ctrl+c: expected settingsNone (fall-through), got %v", action)
	}
}

func TestStereoWideToggleReturnsLiveAction(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.Tab = TabPlayback
	for range 6 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 6 {
		t.Fatalf("precondition: expected cursor=6, got %d", o.cursor)
	}
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != ActionStereoLive {
		t.Errorf("stereo wide toggle: expected settingsStereoLive, got %v", action)
	}
}

func TestStereoWideAmountAdjustReturnsLiveAction(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.Tab = TabPlayback
	for range 7 {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if o.cursor != 7 {
		t.Fatalf("precondition: expected cursor=7, got %d", o.cursor)
	}
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if action != ActionStereoLive {
		t.Errorf("stereo wide amount: expected settingsStereoLive, got %v", action)
	}
}
