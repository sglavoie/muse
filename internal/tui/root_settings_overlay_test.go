package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

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
