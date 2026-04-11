package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

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

// gaplessCapturingPlayer wraps a playPauser stub and records SetGapless calls.
type gaplessCapturingPlayer struct {
	playPauser
	gaplessCalls []bool
}

func (g *gaplessCapturingPlayer) SetGapless(enabled bool) error {
	g.gaplessCalls = append(g.gaplessCalls, enabled)
	return nil
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
	// Advance one tab from Library to Playback.
	next, _ = nm.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	nm = next.(Model)
	if nm.overlay.settings.tab != settingsTabPlayback {
		t.Fatalf("precondition: expected Playback tab, got %v", nm.overlay.settings.tab)
	}
	return nm
}

// TestSettingsSaveAppliesNormalizeToPlayer verifies that ctrl+s calls SetNormalize on the player.
func TestSettingsSaveAppliesNormalizeToPlayer(t *testing.T) {
	stub := &normalizeCapturingPlayer{}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))

	// Open settings overlay and switch to Playback tab.
	nm := openSettingsOnPlaybackTab(t, m)

	// Navigate to the Normalize field (index 4 on Playback tab) and toggle it on.
	var next tea.Model
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

	// Open settings overlay and switch to Playback tab.
	nm := openSettingsOnPlaybackTab(t, m)

	// Navigate to the Gapless field (index 5 on Playback tab).
	var next tea.Model
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
