package settings

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

func TestEQPresetCyclingSnapssBands(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if o.cursor != 1 {
		t.Fatalf("precondition: expected cursor=1, got %d", o.cursor)
	}

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != ActionEQLive {
		t.Errorf("preset cycle: expected settingsEQLive, got %v", action)
	}

	bassBoostPreset := config.BuiltinEQPresets[1]
	eq := o.Fields[TabEqualizer]
	for i := range 10 {
		if eq[i+2].Value != bassBoostPreset.Bands[i] {
			t.Errorf("band %d: expected %d (Bass Boost), got %d", i, bassBoostPreset.Bands[i], eq[i+2].Value)
		}
	}
}

func TestEQPresetCyclingIncludesCustomPresets(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{-2, -1, 0, 0, 0, 0, 0, 0, -1, -2}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	for range len(config.BuiltinEQPresets) {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	}

	presetIdx := o.Fields[TabEqualizer][1].Value
	if o.EQPresets[presetIdx].Key != "night_mode" {
		t.Errorf("after cycling through built-ins: expected custom preset %q, got %q",
			"night_mode", o.EQPresets[presetIdx].Key)
	}
}

func TestEQBandAdjustmentSwitchesToCustomSentinel(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if o.cursor != 2 {
		t.Fatalf("precondition: expected cursor=2, got %d", o.cursor)
	}

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != ActionEQLive {
		t.Errorf("band adjust: expected settingsEQLive, got %v", action)
	}

	sentinelIdx := len(o.EQPresets) - 1
	if o.Fields[TabEqualizer][1].Value != sentinelIdx {
		t.Errorf("preset after band adjust: expected sentinel index %d, got %d",
			sentinelIdx, o.Fields[TabEqualizer][1].Value)
	}
}

func TestEQEnabledToggleReturnsEQLive(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != ActionEQLive {
		t.Errorf("enabled toggle: expected settingsEQLive, got %v", action)
	}
}

func TestEQSavePresetOpensNameInput(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	if action != actionNone {
		t.Errorf("S: expected settingsNone, got %v", action)
	}
	if o.EQNameInput == nil {
		t.Error("S: expected eqNameInput to be non-nil")
	}
}

func TestEQSavePresetTypingAndEnter(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.EQPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'H', Text: "H"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != ActionEQSave {
		t.Errorf("Enter after typing: expected settingsEQSave, got %v", action)
	}
	if o.EQNameInput != nil {
		t.Error("Enter: expected eqNameInput to be nil after confirm")
	}
	if len(o.EQPresets) != initialPresetCount+1 {
		t.Errorf("Enter: expected %d presets, got %d", initialPresetCount+1, len(o.EQPresets))
	}
	newPreset := o.EQPresets[len(o.EQPresets)-2]
	if newPreset.Name != "Hi" {
		t.Errorf("new preset name: expected %q, got %q", "Hi", newPreset.Name)
	}
	if newPreset.Key != "hi" {
		t.Errorf("new preset key: expected %q, got %q", "hi", newPreset.Key)
	}
}

func TestEQSavePresetEmptyNameNoOp(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.EQPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != actionNone {
		t.Errorf("Enter with empty name: expected settingsNone, got %v", action)
	}
	if len(o.EQPresets) != initialPresetCount {
		t.Errorf("Enter with empty name: preset count changed from %d to %d", initialPresetCount, len(o.EQPresets))
	}
}

func TestEQSavePresetEscCancels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.EQPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'X', Text: "X"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != actionNone {
		t.Errorf("Esc in name input: expected settingsNone, got %v", action)
	}
	if o.EQNameInput != nil {
		t.Error("Esc: expected eqNameInput to be nil")
	}
	if len(o.EQPresets) != initialPresetCount {
		t.Errorf("Esc: preset count changed from %d to %d", initialPresetCount, len(o.EQPresets))
	}
}

func TestEQSavePresetSlugDedup(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Sound", Key: "my_sound", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	for _, r := range "My Sound" {
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: rune(r), Text: string(r)})
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	var keys []string
	for _, p := range o.EQPresets {
		if !p.Builtin && p.Key != "custom" {
			keys = append(keys, p.Key)
		}
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 custom preset keys, got %v", keys)
	}
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
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != actionNone {
		t.Errorf("D on built-in: expected settingsNone, got %v", action)
	}
	if o.EQConfirmDel {
		t.Error("D on built-in: expected eqConfirmDel=false")
	}
}

func TestEQDeleteSentinelIgnored(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)

	o.Fields[TabEqualizer][1].Value = len(o.EQPresets) - 1
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != actionNone {
		t.Errorf("D on sentinel: expected settingsNone, got %v", action)
	}
	if o.EQConfirmDel {
		t.Error("D on sentinel: expected eqConfirmDel=false")
	}
}

func TestEQDeleteCustomPresetFlow(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.EQPresets)

	customIdx := len(config.BuiltinEQPresets)
	o.Fields[TabEqualizer][1].Value = customIdx
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != actionNone {
		t.Errorf("D: expected settingsNone, got %v", action)
	}
	if !o.EQConfirmDel {
		t.Error("D: expected eqConfirmDel=true")
	}

	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != ActionEQDelete {
		t.Errorf("D confirm: expected settingsEQDelete, got %v", action)
	}
	if o.EQConfirmDel {
		t.Error("D confirm: expected eqConfirmDel=false after confirm")
	}
	if len(o.EQPresets) != initialPresetCount-1 {
		t.Errorf("D confirm: expected %d presets, got %d", initialPresetCount-1, len(o.EQPresets))
	}

	presetIdx := o.Fields[TabEqualizer][1].Value
	if o.EQPresets[presetIdx].Key != "flat" {
		t.Errorf("after delete: expected preset=flat, got %q", o.EQPresets[presetIdx].Key)
	}
}

func TestEQDeleteCustomPresetEscCancels(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.EQPresets)

	customIdx := len(config.BuiltinEQPresets)
	o.Fields[TabEqualizer][1].Value = customIdx
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != actionNone {
		t.Errorf("Esc in confirm: expected settingsNone, got %v", action)
	}
	if o.EQConfirmDel {
		t.Error("Esc: expected eqConfirmDel=false")
	}
	if len(o.EQPresets) != initialPresetCount {
		t.Errorf("Esc: preset count changed from %d to %d", initialPresetCount, len(o.EQPresets))
	}
}

func TestEQBandAtMinDoesNotResetPreset(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = config.BuiltinEQPresets[1].Bands
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o.cursor = 2
	o.Fields[TabEqualizer][2].Value = -12

	presetBefore := o.Fields[TabEqualizer][1].Value
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if o.Fields[TabEqualizer][1].Value != presetBefore {
		t.Errorf("h at min: preset changed from %d to %d", presetBefore, o.Fields[TabEqualizer][1].Value)
	}
	if action != actionNone {
		t.Errorf("h at min: expected settingsNone, got %v", action)
	}
}

func TestEQBandAtMaxDoesNotResetPreset(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = config.BuiltinEQPresets[1].Bands
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o.cursor = 2
	o.Fields[TabEqualizer][2].Value = 12

	presetBefore := o.Fields[TabEqualizer][1].Value
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if o.Fields[TabEqualizer][1].Value != presetBefore {
		t.Errorf("l at max: preset changed from %d to %d", presetBefore, o.Fields[TabEqualizer][1].Value)
	}
	if action != actionNone {
		t.Errorf("l at max: expected settingsNone, got %v", action)
	}
}

// Navigating onto the "(Custom)" sentinel preset via h/l must preserve the
// user's current band values (not zero them out). The sentinel represents
// "the current edited band values"; snapping to its zero-initialized bands
// would silently destroy the user's work.
func TestEQPresetCyclingOntoCustomSentinelPreservesBands(t *testing.T) {
	cfg := config.Defaults()
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	// Position the Preset cursor just before the (Custom) sentinel.
	sentinelIdx := len(o.EQPresets) - 1
	o.Fields[TabEqualizer][1].Value = sentinelIdx - 1
	o.cursor = 1

	// Set custom band values (simulate user edits that diverged from the
	// preset just before the sentinel).
	custom := [10]int{1, -2, 3, -4, 5, -6, 7, -8, 9, -10}
	for i := range 10 {
		o.Fields[TabEqualizer][i+2].Value = custom[i]
	}

	// Advance one step to land on the (Custom) sentinel.
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if o.Fields[TabEqualizer][1].Value != sentinelIdx {
		t.Fatalf("precondition: expected to land on sentinel idx %d, got %d",
			sentinelIdx, o.Fields[TabEqualizer][1].Value)
	}

	// Bands must be unchanged after landing on sentinel.
	for i := range 10 {
		got := o.Fields[TabEqualizer][i+2].Value
		if got != custom[i] {
			t.Errorf("band %d after sentinel snap: expected %d (preserved), got %d",
				i, custom[i], got)
		}
	}
}

func TestEQSavePresetAllSpecialCharsNameNoOp(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.EQPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if action != actionNone {
		t.Errorf("Enter with all-special-chars name: expected settingsNone, got %v", action)
	}
	if len(o.EQPresets) != initialPresetCount {
		t.Errorf("Enter with all-special-chars name: preset count changed from %d to %d",
			initialPresetCount, len(o.EQPresets))
	}
}
