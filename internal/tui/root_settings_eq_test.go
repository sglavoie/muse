package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

// equalizerCapturingPlayer wraps a playPauser stub and records SetEqualizer calls.
type equalizerCapturingPlayer struct {
	playPauser
	calls           []equalizerCall
	setEqualizerErr func(bool, [10]int) error
}

type equalizerCall struct {
	enabled bool
	bands   [10]int
}

func (e *equalizerCapturingPlayer) SetEqualizer(enabled bool, bands [10]int) error {
	e.calls = append(e.calls, equalizerCall{enabled: enabled, bands: bands})
	if e.setEqualizerErr != nil {
		return e.setEqualizerErr(enabled, bands)
	}
	return nil
}

// openSettingsOnEQTab opens the settings overlay and navigates to the Equalizer tab.
func openSettingsOnEQTab(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)
	// Advance two tabs to reach Equalizer (Library -> Playback -> Equalizer).
	next, _ = nm.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	nm = next.(Model)
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

func TestEQLivePreviewFailureRestoresOverlayState(t *testing.T) {
	stub := &equalizerCapturingPlayer{
		setEqualizerErr: func(enabled bool, bands [10]int) error {
			if enabled {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := config.Defaults()
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	if nm.overlay.settings.fields[settingsTabEqualizer][0].Value != 0 {
		t.Fatal("live preview: expected EQ enabled toggle to revert on failure")
	}
	if !strings.Contains(nm.statusBar.topNotification().msg, "failed") {
		t.Fatalf("live preview: expected failure notification, got %q", nm.statusBar.topNotification().msg)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("live preview: expected failed apply + rollback, got %d calls", len(stub.calls))
	}
	if stub.calls[len(stub.calls)-1].enabled {
		t.Error("live preview: expected rollback to restore EQ disabled")
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

	// Navigate to the first band and increase it so save applies the change.
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm = next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	nm = next.(Model)

	next, _ = nm.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
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
	cfg.EQBands = [10]int{}
	m := New(context.Background(), WithConfig(cfg), WithPlayer(stub))
	nm := openSettingsOnEQTab(t, m)

	// Enable EQ via the overlay (live preview fires).
	next, _ := nm.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
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
