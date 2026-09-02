package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

// normalizeCapturingPlayer wraps a playPauser stub and records SetNormalize calls.
type normalizeCapturingPlayer struct {
	playPauser
	normalizeCalls  []bool
	setNormalizeErr func(bool) error
}

func (n *normalizeCapturingPlayer) SetNormalize(enabled bool) error {
	n.normalizeCalls = append(n.normalizeCalls, enabled)
	if n.setNormalizeErr != nil {
		return n.setNormalizeErr(enabled)
	}
	return nil
}

// gaplessCapturingPlayer wraps a playPauser stub and records SetGapless calls.
type gaplessCapturingPlayer struct {
	playPauser
	gaplessCalls  []bool
	setGaplessErr func(bool) error
}

func (g *gaplessCapturingPlayer) SetGapless(enabled bool) error {
	g.gaplessCalls = append(g.gaplessCalls, enabled)
	if g.setGaplessErr != nil {
		return g.setGaplessErr(enabled)
	}
	return nil
}

// stereoWideCapturingPlayer wraps a playPauser stub and records SetStereoWide calls.
type stereoWideCapturingPlayer struct {
	playPauser
	calls            []stereoWideCall
	setStereoWideErr func(bool, int) error
}

type stereoWideCall struct {
	enabled bool
	amount  int
}

func (s *stereoWideCapturingPlayer) SetStereoWide(enabled bool, amount int) error {
	s.calls = append(s.calls, stereoWideCall{enabled: enabled, amount: amount})
	if s.setStereoWideErr != nil {
		return s.setStereoWideErr(enabled, amount)
	}
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

func TestSettingsSaveNormalizeFailureKeepsOverlayOpen(t *testing.T) {
	stub := &normalizeCapturingPlayer{
		setNormalizeErr: func(enabled bool) error {
			if enabled {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnPlaybackTab(t, m)

	var next tea.Model
	for range 4 {
		next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	next, cmd := nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm = next.(Model)

	if cmd != nil {
		t.Fatal("ctrl+s: expected nil Cmd when normalize apply fails")
	}
	if nm.overlay.settings == nil {
		t.Fatal("ctrl+s: expected settings overlay to remain open on failure")
	}
	if nm.cfg.NormalizeVolume {
		t.Error("ctrl+s: expected cfg.NormalizeVolume to remain false after failure")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "failed") {
		t.Fatalf("ctrl+s: expected failure notification, got %q", nm.statusBar.TopNotification().Msg)
	}
	if len(stub.normalizeCalls) != 2 {
		t.Fatalf("ctrl+s: expected failed apply + rollback, got %d calls", len(stub.normalizeCalls))
	}
	last := stub.normalizeCalls[len(stub.normalizeCalls)-1]
	if last {
		t.Error("ctrl+s: expected rollback to restore normalize=false")
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

func TestStereoWideLivePreviewFailureRestoresOverlayState(t *testing.T) {
	stub := &stereoWideCapturingPlayer{
		setStereoWideErr: func(enabled bool, amount int) error {
			if enabled {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnPlaybackTab(t, m)

	for range 6 {
		next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	if nm.overlay.settings.fields[settingsTabPlayback][6].Value != 0 {
		t.Fatal("live preview: expected stereo wide toggle to revert on failure")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "failed") {
		t.Fatalf("live preview: expected failure notification, got %q", nm.statusBar.TopNotification().Msg)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("live preview: expected failed apply + rollback, got %d calls", len(stub.calls))
	}
	if stub.calls[len(stub.calls)-1].enabled {
		t.Error("live preview: expected rollback to restore stereo wide disabled")
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

	// Navigate to the width field and increase it.
	for range 7 {
		next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		nm = next.(Model)
	}
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	next, _ = nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
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
	if last.amount != 21 {
		t.Errorf("ctrl+s: expected SetStereoWide called with amount=21, got %d", last.amount)
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
