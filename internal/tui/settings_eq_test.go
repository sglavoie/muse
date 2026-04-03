package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func switchToEQTab(o SettingsOverlay) SettingsOverlay {
	o.tab = settingsTabEqualizer
	o.cursor = 0
	return o
}

func TestEQTabFieldCount(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	eq := o.fields[settingsTabEqualizer]
	if len(eq) != 12 {
		t.Fatalf("EQ tab: expected 12 fields, got %d", len(eq))
	}
}

func TestEQTabPresetsPopulated(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	want := len(config.BuiltinEQPresets) + 1
	if len(o.eqPresets) != want {
		t.Fatalf("eqPresets: expected %d entries, got %d", want, len(o.eqPresets))
	}
	last := o.eqPresets[len(o.eqPresets)-1]
	if last.Name != eqCustomSentinelName {
		t.Errorf("last eqPreset: expected %q, got %q", eqCustomSentinelName, last.Name)
	}
}

func TestEQPresetFieldMaxMatchesSentinelIndex(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	presetField := o.fields[settingsTabEqualizer][1]
	wantMax := len(o.eqPresets) - 1
	if presetField.Max != wantMax {
		t.Errorf("Preset field Max: expected %d, got %d", wantMax, presetField.Max)
	}
}

func TestEQDefaultPresetIsFlatWhenConfigIsFlat(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "flat"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	if o.eqPresets[presetIdx].Key != "flat" {
		t.Errorf("default preset: expected key %q, got %q", "flat", o.eqPresets[presetIdx].Key)
	}
}

func TestEQUnknownPresetFallsBackToCustomSentinel(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "nonexistent_preset"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	sentinelIdx := len(o.eqPresets) - 1
	if presetIdx != sentinelIdx {
		t.Errorf("unknown preset: expected sentinel index %d, got %d", sentinelIdx, presetIdx)
	}
}

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
	if action != settingsEQLive {
		t.Errorf("preset cycle: expected settingsEQLive, got %v", action)
	}

	bassBoostPreset := config.BuiltinEQPresets[1]
	eq := o.fields[settingsTabEqualizer]
	for i := range 10 {
		if eq[i+2].Value != bassBoostPreset.Bands[i] {
			t.Errorf("band %d: expected %d (Bass Boost), got %d", i, bassBoostPreset.Bands[i], eq[i+2].Value)
		}
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
	if action != settingsEQLive {
		t.Errorf("band adjust: expected settingsEQLive, got %v", action)
	}

	sentinelIdx := len(o.eqPresets) - 1
	if o.fields[settingsTabEqualizer][1].Value != sentinelIdx {
		t.Errorf("preset after band adjust: expected sentinel index %d, got %d",
			sentinelIdx, o.fields[settingsTabEqualizer][1].Value)
	}
}

func TestEQEnabledToggleReturnsEQLive(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	_, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if action != settingsEQLive {
		t.Errorf("enabled toggle: expected settingsEQLive, got %v", action)
	}
}

func TestEQToConfigRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQPreset = "rock"
	cfg.EQBands = [10]int{5, 4, 2, 0, -1, 0, 2, 3, 4, 5}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)

	got := o.ToConfig(cfg)

	if !got.EQEnabled {
		t.Error("ToConfig: expected EQEnabled=true")
	}
	if got.EQPreset != "rock" {
		t.Errorf("ToConfig: expected EQPreset=%q, got %q", "rock", got.EQPreset)
	}
	for i, v := range got.EQBands {
		if v != cfg.EQBands[i] {
			t.Errorf("ToConfig: band %d: expected %d, got %d", i, cfg.EQBands[i], v)
		}
	}
}

func TestEQToConfigCustomSentinelKeyIsCustom(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
	got := o.ToConfig(config.Defaults())
	if got.EQPreset != "custom" {
		t.Errorf("ToConfig with sentinel: expected EQPreset=%q, got %q", "custom", got.EQPreset)
	}
}

func TestEQStateHelper(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQEnabled = true
	cfg.EQBands = [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)

	enabled, bands := o.EQState()
	if !enabled {
		t.Error("EQState: expected enabled=true")
	}
	for i, v := range bands {
		if v != cfg.EQBands[i] {
			t.Errorf("EQState: band %d: expected %d, got %d", i, cfg.EQBands[i], v)
		}
	}
}

func TestEQRenderEqualizerTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	for _, label := range []string{"Enabled", "Preset", "31 Hz", "16 kHz"} {
		if !strings.Contains(out, label) {
			t.Errorf("Render EQ tab: expected field label %q in output", label)
		}
	}
}

func TestEQRenderShowsPresetName(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabEqualizer
	o.cursor = 1
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Bass Boost") {
		t.Errorf("Render EQ tab: expected preset name 'Bass Boost' in output, got:\n%s", out)
	}
}

func TestCustomPresetsFromConfigPopulateOverlay(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Preset", Key: "my_preset", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)

	want := len(config.BuiltinEQPresets) + 1 + 1
	if len(o.eqPresets) != want {
		t.Fatalf("eqPresets with custom: expected %d, got %d", want, len(o.eqPresets))
	}
	sentinel := len(o.eqPresets) - 1
	custom := o.eqPresets[sentinel-1]
	if custom.Key != "my_preset" {
		t.Errorf("custom preset key: expected %q, got %q", "my_preset", custom.Key)
	}
	if custom.Builtin {
		t.Error("custom preset should have Builtin=false")
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

	presetIdx := o.fields[settingsTabEqualizer][1].Value
	if o.eqPresets[presetIdx].Key != "night_mode" {
		t.Errorf("after cycling through built-ins: expected custom preset %q, got %q",
			"night_mode", o.eqPresets[presetIdx].Key)
	}
}

func TestEQSavePresetOpensNameInput(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	if action != settingsNone {
		t.Errorf("S: expected settingsNone, got %v", action)
	}
	if o.eqNameInput == nil {
		t.Error("S: expected eqNameInput to be non-nil")
	}
}

func TestEQSavePresetTypingAndEnter(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'H', Text: "H"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != settingsEQSave {
		t.Errorf("Enter after typing: expected settingsEQSave, got %v", action)
	}
	if o.eqNameInput != nil {
		t.Error("Enter: expected eqNameInput to be nil after confirm")
	}
	if len(o.eqPresets) != initialPresetCount+1 {
		t.Errorf("Enter: expected %d presets, got %d", initialPresetCount+1, len(o.eqPresets))
	}
	newPreset := o.eqPresets[len(o.eqPresets)-2]
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
	initialPresetCount := len(o.eqPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != settingsNone {
		t.Errorf("Enter with empty name: expected settingsNone, got %v", action)
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Enter with empty name: preset count changed from %d to %d", initialPresetCount, len(o.eqPresets))
	}
}

func TestEQSavePresetEscCancels(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'X', Text: "X"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != settingsNone {
		t.Errorf("Esc in name input: expected settingsNone, got %v", action)
	}
	if o.eqNameInput != nil {
		t.Error("Esc: expected eqNameInput to be nil")
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Esc: preset count changed from %d to %d", initialPresetCount, len(o.eqPresets))
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
	for _, p := range o.eqPresets {
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
	if action != settingsNone {
		t.Errorf("D on built-in: expected settingsNone, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("D on built-in: expected eqConfirmDel=false")
	}
}

func TestEQDeleteSentinelIgnored(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)

	o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsNone {
		t.Errorf("D on sentinel: expected settingsNone, got %v", action)
	}
	if o.eqConfirmDel {
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
	initialPresetCount := len(o.eqPresets)

	customIdx := len(config.BuiltinEQPresets)
	o.fields[settingsTabEqualizer][1].Value = customIdx
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsNone {
		t.Errorf("D: expected settingsNone, got %v", action)
	}
	if !o.eqConfirmDel {
		t.Error("D: expected eqConfirmDel=true")
	}

	o, action, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if action != settingsEQDelete {
		t.Errorf("D confirm: expected settingsEQDelete, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("D confirm: expected eqConfirmDel=false after confirm")
	}
	if len(o.eqPresets) != initialPresetCount-1 {
		t.Errorf("D confirm: expected %d presets, got %d", initialPresetCount-1, len(o.eqPresets))
	}

	presetIdx := o.fields[settingsTabEqualizer][1].Value
	if o.eqPresets[presetIdx].Key != "flat" {
		t.Errorf("after delete: expected preset=flat, got %q", o.eqPresets[presetIdx].Key)
	}
}

func TestEQDeleteCustomPresetEscCancels(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	customIdx := len(config.BuiltinEQPresets)
	o.fields[settingsTabEqualizer][1].Value = customIdx
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != settingsNone {
		t.Errorf("Esc in confirm: expected settingsNone, got %v", action)
	}
	if o.eqConfirmDel {
		t.Error("Esc: expected eqConfirmDel=false")
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Esc: preset count changed from %d to %d", initialPresetCount, len(o.eqPresets))
	}
}

func TestToConfigIncludesCustomPresets(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "My Sound", Key: "my_sound", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	got := o.ToConfig(cfg)

	if len(got.EQCustomPresets) != 1 {
		t.Fatalf("ToConfig: expected 1 custom preset, got %d", len(got.EQCustomPresets))
	}
	if got.EQCustomPresets[0].Key != "my_sound" {
		t.Errorf("ToConfig: expected key %q, got %q", "my_sound", got.EQCustomPresets[0].Key)
	}
}

func TestToConfigExcludesBuiltinsAndSentinel(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	got := o.ToConfig(config.Defaults())

	if len(got.EQCustomPresets) != 0 {
		t.Errorf("ToConfig with no custom presets: expected 0, got %d", len(got.EQCustomPresets))
	}
}

func TestEQRenderShowsNameInputPrompt(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabEqualizer
	input := newSettingsInput()
	input.SetValue("My")
	o.eqNameInput = input
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Save as:") {
		t.Errorf("Render with name input: expected 'Save as:' in output, got:\n%s", out)
	}
}

func TestEQRenderShowsDeleteConfirmation(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQCustomPresets = []config.EQCustomPreset{
		{Name: "Night Mode", Key: "night_mode", Bands: [10]int{}},
	}
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o.tab = settingsTabEqualizer
	o.cursor = 1
	o.fields[settingsTabEqualizer][1].Value = len(config.BuiltinEQPresets)
	o.eqConfirmDel = true
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "Delete") {
		t.Errorf("Render with delete confirm: expected 'Delete' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Night Mode") {
		t.Errorf("Render with delete confirm: expected 'Night Mode' in output, got:\n%s", out)
	}
}

func TestEQHintLineShowsOnEQTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "s save") {
		t.Errorf("EQ tab hint: expected 's save' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "d delete") {
		t.Errorf("EQ tab hint: expected 'd delete' in output, got:\n%s", out)
	}
}

func TestEQBandAtMinDoesNotResetPreset(t *testing.T) {
	cfg := config.Defaults()
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = config.BuiltinEQPresets[1].Bands
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	o = switchToEQTab(o)

	o.cursor = 2
	o.fields[settingsTabEqualizer][2].Value = -12

	presetBefore := o.fields[settingsTabEqualizer][1].Value
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if o.fields[settingsTabEqualizer][1].Value != presetBefore {
		t.Errorf("h at min: preset changed from %d to %d", presetBefore, o.fields[settingsTabEqualizer][1].Value)
	}
	if action != settingsNone {
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
	o.fields[settingsTabEqualizer][2].Value = 12

	presetBefore := o.fields[settingsTabEqualizer][1].Value
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if o.fields[settingsTabEqualizer][1].Value != presetBefore {
		t.Errorf("l at max: preset changed from %d to %d", presetBefore, o.fields[settingsTabEqualizer][1].Value)
	}
	if action != settingsNone {
		t.Errorf("l at max: expected settingsNone, got %v", action)
	}
}

func TestEQSavePresetAllSpecialCharsNameNoOp(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o = switchToEQTab(o)
	initialPresetCount := len(o.eqPresets)

	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if action != settingsNone {
		t.Errorf("Enter with all-special-chars name: expected settingsNone, got %v", action)
	}
	if len(o.eqPresets) != initialPresetCount {
		t.Errorf("Enter with all-special-chars name: preset count changed from %d to %d",
			initialPresetCount, len(o.eqPresets))
	}
}

func TestEQHintLineIncludesJKFields(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	o.tab = settingsTabEqualizer
	s := styles.NewStyles(true)
	out := ansi.Strip(o.Render(80, 40, s))
	if !strings.Contains(out, "j/k fields") {
		t.Errorf("EQ tab hint: expected 'j/k fields' in output, got:\n%s", out)
	}
}
