package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
)

// normalizeCapturingPlayer wraps a playPauser stub and records SetNormalize calls.
type normalizeCapturingPlayer struct {
	playPauser
	normalizeCalls []bool
}

func (n *normalizeCapturingPlayer) SetNormalize(enabled bool) error {
	n.normalizeCalls = append(n.normalizeCalls, enabled)
	return nil
}

// TestCtrlCommaOpensSettingsOverlay verifies that ctrl+, opens the settings overlay.
func TestCtrlCommaOpensSettingsOverlay(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)
	if nm.overlay.settings == nil {
		t.Fatal("ctrl+,: expected settings overlay to be non-nil")
	}
}

// TestSettingsOverlayEscClosesWithoutChangingConfig verifies esc cancels.
func TestSettingsOverlayEscClosesWithoutChangingConfig(t *testing.T) {
	m := New(context.Background())
	origCfg := m.cfg

	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// Adjust a value.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	// Esc: cancel.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("esc: expected settings overlay to be nil after cancel")
	}
	if nm.cfg.VolumeIncrement != origCfg.VolumeIncrement ||
		nm.cfg.SeekOffsetShort != origCfg.SeekOffsetShort ||
		nm.cfg.SeekOffsetLong != origCfg.SeekOffsetLong {
		t.Fatal("esc: expected config to be unchanged after cancel")
	}
}

// TestSettingsOverlayCtrlSSavesConfig verifies ctrl+s applies values to the model config.
func TestSettingsOverlayCtrlSSavesConfig(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// Increment VolumeIncrement (first field, cursor=0 on Playback tab).
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)
	expectedVolInc := nm.overlay.settings.fields[settingsTabPlayback][0].Value

	// Save.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("ctrl+s: expected settings overlay to be nil after save")
	}
	if nm.cfg.VolumeIncrement != expectedVolInc {
		t.Errorf("ctrl+s: expected VolumeIncrement=%d, got %d", expectedVolInc, nm.cfg.VolumeIncrement)
	}
}

// TestSettingsOverlayBlocksOtherKeyActions verifies that keys consumed by the overlay
// do not reach the normal key-action handler while the overlay is active.
func TestSettingsOverlayBlocksOtherKeyActions(t *testing.T) {
	m := New(context.Background())
	// Open settings.
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// '1' normally switches to Library view, but with settings open it should
	// be swallowed by the overlay (no view switch).
	prevView := nm.activeView
	next, _ = nm.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm2 := next.(Model)

	if nm2.activeView != prevView {
		t.Errorf("key '1' with settings open: expected no view switch, got view=%v", nm2.activeView)
	}
	if nm2.overlay.settings == nil {
		t.Error("key '1' with settings open: expected overlay still active")
	}
}

// TestSettingsOverlayCtrlCFallsThrough verifies ctrl+c is not consumed by settings.
func TestSettingsOverlayCtrlCFallsThrough(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// ctrl+c should still trigger quit even while settings is open.
	_, cmd := nm.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c with settings open: expected quit command, got nil")
	}
}

// TestSettingsOverlayRenderedInView verifies the overlay appears in View() output.
func TestSettingsOverlayRenderedInView(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 24
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	v := nm.View()
	stripped := ansi.Strip(v.Content)
	if !strings.Contains(stripped, "Settings") {
		t.Errorf("View() with settings overlay: expected 'Settings' in output, got:\n%s", stripped)
	}
}

// TestSettingsSavedMsgShowsNotification verifies the notification is set on save.
func TestSettingsSavedMsgShowsNotification(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(settingsSavedMsg{})
	nm := next.(Model)
	if nm.statusBar.notification.msg == "" {
		t.Error("settingsSavedMsg: expected notification to be set")
	}
}

// TestSettingsSavedMsgErrorShowsErrorNotification verifies error notification.
func TestSettingsSavedMsgErrorShowsErrorNotification(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(settingsSavedMsg{err: errors.New("save error")})
	nm := next.(Model)
	if !strings.Contains(nm.statusBar.notification.msg, "failed") {
		t.Errorf("settingsSavedMsg with error: expected 'failed' in notification, got %q", nm.statusBar.notification.msg)
	}
}

// gaplessCapturingPlayer wraps a playPauser stub and records SetGapless calls.
type gaplessCapturingPlayer struct {
	playPauser
	gaplessCalls []bool
}

func (g *gaplessCapturingPlayer) SetGapless(enabled bool) error {
	g.gaplessCalls = append(g.gaplessCalls, enabled)
	return nil
}

// TestSettingsSaveAppliesNormalizeToPlayer verifies that ctrl+s calls SetNormalize on the player.
func TestSettingsSaveAppliesNormalizeToPlayer(t *testing.T) {
	stub := &normalizeCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))

	// Open settings overlay.
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// Navigate to the Normalize field (index 4 on Playback tab) and toggle it on.
	for range 4 {
		next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	// Save.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("ctrl+s: expected settings overlay to be nil after save")
	}
	if !nm.cfg.NormalizeVolume {
		t.Error("ctrl+s: expected cfg.NormalizeVolume=true")
	}
	if len(stub.normalizeCalls) == 0 {
		t.Fatal("ctrl+s: expected SetNormalize to be called")
	}
	if !stub.normalizeCalls[0] {
		t.Error("ctrl+s: expected SetNormalize(true)")
	}
}

// TestSettingsSaveAppliesGaplessToPlayer verifies that ctrl+s calls SetGapless on the player.
func TestSettingsSaveAppliesGaplessToPlayer(t *testing.T) {
	stub := &gaplessCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))

	// Open settings overlay.
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)

	// Navigate to the Gapless field (index 5 on Playback tab).
	for range 5 {
		next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	// Default is On (1); press 'h' to toggle to Off (0).
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	nm = next.(Model)

	// Save.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("ctrl+s: expected settings overlay to be nil after save")
	}
	if nm.cfg.GaplessAudio {
		t.Error("ctrl+s: expected cfg.GaplessAudio=false")
	}
	if len(stub.gaplessCalls) == 0 {
		t.Fatal("ctrl+s: expected SetGapless to be called")
	}
	if stub.gaplessCalls[0] {
		t.Error("ctrl+s: expected SetGapless(false)")
	}
}

// equalizerCapturingPlayer wraps a playPauser stub and records SetEqualizer calls.
type equalizerCapturingPlayer struct {
	playPauser
	calls []equalizerCall
}

type equalizerCall struct {
	enabled bool
	bands   [10]int
}

func (e *equalizerCapturingPlayer) SetEqualizer(enabled bool, bands [10]int) error {
	e.calls = append(e.calls, equalizerCall{enabled: enabled, bands: bands})
	return nil
}

// openSettingsOnEQTab opens the settings overlay and navigates to the Equalizer tab.
func openSettingsOnEQTab(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)
	// Advance one tab to reach Equalizer (Playback -> Equalizer).
	next, _ = nm.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	nm = next.(Model)
	if nm.overlay.settings.tab != settingsTabEqualizer {
		t.Fatalf("precondition: expected EQ tab, got %v", nm.overlay.settings.tab)
	}
	return nm
}

// TestEQLivePreviewFiresOnBandChange verifies SetEqualizer is called during live preview.
func TestEQLivePreviewFiresOnBandChange(t *testing.T) {
	stub := &equalizerCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Navigate to first band field (cursor 2) and adjust it.
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // cursor=1 (Preset)
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // cursor=2 (31 Hz)
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // increase band
	_ = next.(Model)

	if len(stub.calls) == 0 {
		t.Fatal("live preview: expected SetEqualizer to be called on band change")
	}
}

// TestEQLivePreviewFiresOnPresetCycle verifies SetEqualizer is called when cycling presets.
func TestEQLivePreviewFiresOnPresetCycle(t *testing.T) {
	stub := &equalizerCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Navigate to Preset field (cursor 1) and cycle.
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // cursor=1
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // advance preset
	_ = next.(Model)

	if len(stub.calls) == 0 {
		t.Fatal("live preview: expected SetEqualizer to be called on preset cycle")
	}
}

// TestEQSaveAppliesEqualizerToPlayer verifies ctrl+s calls SetEqualizer with saved config.
func TestEQSaveAppliesEqualizerToPlayer(t *testing.T) {
	stub := &equalizerCapturingPlayer{}
	cfg := config.Defaults()
	cfg.EQEnabled = true
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Save without any changes.
	next, _ := nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("ctrl+s: expected settings overlay closed after save")
	}
	if len(stub.calls) == 0 {
		t.Fatal("ctrl+s: expected SetEqualizer to be called on save")
	}
	last := stub.calls[len(stub.calls)-1]
	if !last.enabled {
		t.Error("ctrl+s: expected SetEqualizer called with enabled=true")
	}
}

// TestEQCancelRestoresOriginalEQState verifies esc restores the pre-edit EQ state.
func TestEQCancelRestoresOriginalEQState(t *testing.T) {
	stub := &equalizerCapturingPlayer{}
	cfg := config.Defaults()
	cfg.EQEnabled = false
	cfg.EQBands = [10]int{} // all zero
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Enable EQ via the overlay (live preview fires).
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"}) // toggle Enabled on
	nm = next.(Model)

	callsBefore := len(stub.calls)
	if callsBefore == 0 {
		t.Fatal("precondition: expected at least one SetEqualizer call from live preview")
	}

	// Cancel — should restore to original disabled state.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("esc: expected settings overlay closed")
	}
	if len(stub.calls) <= callsBefore {
		t.Fatal("esc: expected SetEqualizer to be called on cancel to restore state")
	}
	last := stub.calls[len(stub.calls)-1]
	if last.enabled {
		t.Error("esc: expected SetEqualizer called with enabled=false (original state)")
	}
}

// TestEQSaveActionIsConsumed verifies that settingsEQSave is handled without closing the overlay.
func TestEQSaveActionIsConsumed(t *testing.T) {
	stub := &equalizerCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Open name input (S key).
	next, _ := nm.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	nm = next.(Model)
	if nm.overlay.settings == nil {
		t.Fatal("after S: expected settings overlay still open")
	}

	// Type a name and confirm.
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)

	// Overlay should still be open (save only persists custom preset, not settings).
	if nm.overlay.settings == nil {
		t.Fatal("after preset save: expected settings overlay still open")
	}
}

// stereoWideCapturingPlayer wraps a playPauser stub and records SetStereoWide calls.
type stereoWideCapturingPlayer struct {
	playPauser
	calls []stereoWideCall
}

type stereoWideCall struct {
	enabled bool
	amount  int
}

func (s *stereoWideCapturingPlayer) SetStereoWide(enabled bool, amount int) error {
	s.calls = append(s.calls, stereoWideCall{enabled: enabled, amount: amount})
	return nil
}

// openSettingsOnPlaybackTab opens the settings overlay on the Playback tab.
func openSettingsOnPlaybackTab(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)
	if nm.overlay.settings.tab != settingsTabPlayback {
		t.Fatalf("precondition: expected Playback tab, got %v", nm.overlay.settings.tab)
	}
	return nm
}

// TestStereoWideLivePreviewFiresOnToggle verifies SetStereoWide is called during live preview.
func TestStereoWideLivePreviewFiresOnToggle(t *testing.T) {
	stub := &stereoWideCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnPlaybackTab(t, m)

	// Navigate to cursor=6 (Stereo wide toggle).
	for range 6 {
		next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	_ = next.(Model)

	if len(stub.calls) == 0 {
		t.Fatal("live preview: expected SetStereoWide to be called on toggle")
	}
}

// TestStereoWideSaveAppliesSettingsToPlayer verifies ctrl+s calls SetStereoWide with saved values.
func TestStereoWideSaveAppliesSettingsToPlayer(t *testing.T) {
	stub := &stereoWideCapturingPlayer{}
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 20
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnPlaybackTab(t, m)

	// Save without changes.
	next, _ := nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("ctrl+s: expected settings overlay closed after save")
	}
	if len(stub.calls) == 0 {
		t.Fatal("ctrl+s: expected SetStereoWide to be called on save")
	}
	last := stub.calls[len(stub.calls)-1]
	if !last.enabled {
		t.Error("ctrl+s: expected SetStereoWide called with enabled=true")
	}
	if last.amount != 20 {
		t.Errorf("ctrl+s: expected SetStereoWide called with amount=20, got %d", last.amount)
	}
}

// TestStereoWideCancelRevertsToSavedState verifies esc restores stereo wide to saved config values.
func TestStereoWideCancelRevertsToSavedState(t *testing.T) {
	stub := &stereoWideCapturingPlayer{}
	cfg := config.Defaults()
	cfg.StereoWideEnabled = false
	cfg.StereoWideAmount = 10
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnPlaybackTab(t, m)

	// Navigate to Stereo wide toggle (cursor=6) and enable it.
	for range 6 {
		next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	callsBefore := len(stub.calls)
	if callsBefore == 0 {
		t.Fatal("precondition: expected at least one SetStereoWide call from live preview")
	}

	// Cancel — should restore to original disabled state.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.overlay.settings != nil {
		t.Fatal("esc: expected settings overlay closed")
	}
	if len(stub.calls) <= callsBefore {
		t.Fatal("esc: expected SetStereoWide to be called on cancel to restore state")
	}
	last := stub.calls[len(stub.calls)-1]
	if last.enabled {
		t.Error("esc: expected SetStereoWide called with enabled=false (original state)")
	}
	if last.amount != 10 {
		t.Errorf("esc: expected SetStereoWide called with amount=10, got %d", last.amount)
	}
}

// TestEQDeleteActionAppliesEqualizer verifies SetEqualizer is called after deleting a preset.
func TestEQDeleteActionAppliesEqualizer(t *testing.T) {
	stub := &equalizerCapturingPlayer{}
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Sound", Key: "my_sound", Bands: [10]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
	}
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Navigate to the custom preset.
	customIdx := len(config.BuiltinEQPresets)
	nm.overlay.settings.fields[settingsTabEqualizer][1].Value = customIdx

	// Move cursor to Preset field (j once) then press D.
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm = next.(Model)

	callsBefore := len(stub.calls)

	// Confirm with Enter.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)

	if len(stub.calls) <= callsBefore {
		t.Fatal("after delete confirm: expected SetEqualizer to be called")
	}
}
