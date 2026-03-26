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
	if len(pb) != 4 {
		t.Fatalf("expected 4 playback fields, got %d", len(pb))
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
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabDisplay {
		t.Errorf("']': expected settingsTabDisplay, got %v", o.tab)
	}
	if o.cursor != 0 {
		t.Errorf("']': expected cursor reset to 0, got %d", o.cursor)
	}
}

func TestSettingsTabSwitchBackward(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Start at Playback, wrap backward to Display.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if o.tab != settingsTabDisplay {
		t.Errorf("'[' from Playback: expected wrap to settingsTabDisplay, got %v", o.tab)
	}
}

func TestSettingsTabSwitchWraps(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	// Forward twice wraps back to Playback.
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	o, _ = o.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if o.tab != settingsTabPlayback {
		t.Errorf("']' twice: expected wrap back to settingsTabPlayback, got %v", o.tab)
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
	for _, name := range []string{"Playback", "Display"} {
		if !strings.Contains(out, name) {
			t.Errorf("Render: expected tab name %q in output", name)
		}
	}
}

func TestSettingsRenderContainsFieldLabels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults())
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 24, s))
	for _, label := range []string{"Volume step", "Seek short", "Seek long", "Default volume"} {
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
