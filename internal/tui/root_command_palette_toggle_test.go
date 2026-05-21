package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigToggleMsg_ReturnsSettingsSavedMsg verifies that handleConfigToggleMsg
// returns a Cmd that produces a settingsSavedMsg (surfacing save errors) rather
// than silently discarding the error.
func TestConfigToggleMsg_ReturnsSettingsSavedMsg(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	m := New(context.Background(), WithConfigPath(cfgPath, false))

	for _, tc := range []struct {
		name  string
		field configToggleField
	}{
		{"Normalize", toggleNormalize},
		{"EQ", toggleEQ},
		{"StereoWide", toggleStereoWide},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, cmd := m.handleConfigToggleMsg(configToggleMsg{field: tc.field})
			if cmd == nil {
				t.Fatal("expected non-nil Cmd when configPath is set")
			}
			msg := cmd()
			saved, ok := msg.(settingsSavedMsg)
			if !ok {
				t.Fatalf("expected settingsSavedMsg, got %T", msg)
			}
			if saved.err != nil {
				t.Fatalf("unexpected save error: %v", saved.err)
			}
			_ = result
		})
	}
}

// TestConfigToggleMsg_NoPath verifies that handleConfigToggleMsg returns nil
// Cmd when configPath is empty (no config file to persist to).
func TestConfigToggleMsg_NoPath(t *testing.T) {
	m := New(context.Background()) // no WithConfigPath
	_, cmd := m.handleConfigToggleMsg(configToggleMsg{field: toggleNormalize})
	if cmd != nil {
		t.Fatal("expected nil Cmd when configPath is empty")
	}
}

// TestConfigToggleMsg_NotifyContent verifies that the status bar notification
// reflects the toggled setting.
func TestConfigToggleMsg_NotifyContent(t *testing.T) {
	m := New(context.Background())
	m.cfg.NormalizeVolume = false

	result, _ := m.handleConfigToggleMsg(configToggleMsg{field: toggleNormalize})
	if !strings.Contains(result.statusBar.topNotification().msg, "normalization") {
		t.Errorf("expected 'normalization' in notification, got %q", result.statusBar.topNotification().msg)
	}
	if !strings.Contains(result.statusBar.topNotification().msg, "On") {
		t.Errorf("expected 'On' in notification, got %q", result.statusBar.topNotification().msg)
	}
}

func TestConfigToggleMsg_NormalizeApplyFailureDoesNotPersist(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	stub := &normalizeCapturingPlayer{
		setNormalizeErr: func(enabled bool) error {
			if enabled {
				return errors.New("boom")
			}
			return nil
		},
	}
	m := New(context.Background(), WithConfigPath(cfgPath, false), WithPlayer(stub))

	result, cmd := m.handleConfigToggleMsg(configToggleMsg{field: toggleNormalize})

	if cmd != nil {
		t.Fatal("expected nil Cmd when normalize apply fails")
	}
	if result.cfg.NormalizeVolume {
		t.Fatal("expected NormalizeVolume to remain false after failure")
	}
	if !strings.Contains(result.statusBar.topNotification().msg, "failed") {
		t.Fatalf("expected failure notification, got %q", result.statusBar.topNotification().msg)
	}
}

func TestConfigToggleMsg_EQApplyFailureDoesNotPersist(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	stub := &equalizerCapturingPlayer{
		setEqualizerErr: func(enabled bool, bands [10]int) error {
			if enabled {
				return errors.New("boom")
			}
			return nil
		},
	}
	m := New(context.Background(), WithConfigPath(cfgPath, false), WithPlayer(stub))

	result, cmd := m.handleConfigToggleMsg(configToggleMsg{field: toggleEQ})

	if cmd != nil {
		t.Fatal("expected nil Cmd when EQ apply fails")
	}
	if result.cfg.EQEnabled {
		t.Fatal("expected EQEnabled to remain false after failure")
	}
	if !strings.Contains(result.statusBar.topNotification().msg, "failed") {
		t.Fatalf("expected failure notification, got %q", result.statusBar.topNotification().msg)
	}
}

func TestConfigToggleMsg_StereoWideApplyFailureDoesNotPersist(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	stub := &stereoWideCapturingPlayer{
		setStereoWideErr: func(enabled bool, amount int) error {
			if enabled {
				return errors.New("boom")
			}
			return nil
		},
	}
	m := New(context.Background(), WithConfigPath(cfgPath, false), WithPlayer(stub))

	result, cmd := m.handleConfigToggleMsg(configToggleMsg{field: toggleStereoWide})

	if cmd != nil {
		t.Fatal("expected nil Cmd when stereo widening apply fails")
	}
	if result.cfg.StereoWideEnabled {
		t.Fatal("expected StereoWideEnabled to remain false after failure")
	}
	if !strings.Contains(result.statusBar.topNotification().msg, "failed") {
		t.Fatalf("expected failure notification, got %q", result.statusBar.topNotification().msg)
	}
}
