package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// settingsTab identifies the active tab in the settings overlay.
type settingsTab int

const (
	settingsTabPlayback  settingsTab = iota
	settingsTabEqualizer             // inserted between Playback and Display
	settingsTabDisplay
	settingsTabCount
)

// settingsField describes a single adjustable setting row.
type settingsField struct {
	Label   string
	Value   int
	Default int
	Min     int
	Max     int
	Step    int
	Unit    string
}

// settingsAction is the result of a key press in the settings overlay.
type settingsAction int

const (
	settingsNone     settingsAction = iota
	settingsSave                    // ctrl+s was pressed
	settingsCancel                  // esc was pressed
	settingsEQLive                  // live EQ preview on band/toggle change
	settingsEQSave                  // custom preset saved
	settingsEQDelete                // custom preset deleted
	settingsStereoLive              // live stereo wide preview on toggle/amount change
)

// SettingsOverlay is a centered overlay for editing runtime-tunable config values.
type SettingsOverlay struct {
	tab          settingsTab
	cursor       int
	fields       [settingsTabCount][]settingsField
	eqPresets    []config.EQPresetDef
	eqNameInput  *textInput // non-nil when prompting for preset name
	eqConfirmDel bool       // true when showing delete confirmation
	dirty        bool
}

// eqCustomSentinelName is the display name of the transient "not a saved preset" state.
const eqCustomSentinelName = "(Custom)"

// NewSettingsOverlay creates a SettingsOverlay pre-populated from cfg.
func NewSettingsOverlay(cfg config.Config) SettingsOverlay {
	o := SettingsOverlay{}
	o.fields[settingsTabPlayback] = []settingsField{
		{Label: "Volume step", Value: cfg.VolumeIncrement, Default: 5, Min: 1, Max: 25, Step: 1, Unit: "%"},
		{Label: "Seek short", Value: int(cfg.SeekOffsetShort), Default: 5, Min: 1, Max: 60, Step: 1, Unit: "s"},
		{Label: "Seek long", Value: int(cfg.SeekOffsetLong), Default: 30, Min: 5, Max: 300, Step: 5, Unit: "s"},
		{Label: "Default volume", Value: cfg.DefaultVolume, Default: 100, Min: 0, Max: 100, Step: 5, Unit: "%"},
		{Label: "Normalize", Value: boolToInt(cfg.NormalizeVolume), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Gapless", Value: boolToInt(cfg.GaplessAudio), Default: 1, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Stereo wide", Value: boolToInt(cfg.StereoWideEnabled), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Width", Value: cfg.StereoWideAmount, Default: 10, Min: 0, Max: 40, Step: 1, Unit: "tenth"},
	}

	// Build the preset list: built-ins + custom presets + "(Custom)" sentinel.
	eqPresets := make([]config.EQPresetDef, 0, len(config.BuiltinEQPresets)+len(cfg.EQCustomPresets)+1)
	eqPresets = append(eqPresets, config.BuiltinEQPresets...)
	for _, cp := range cfg.EQCustomPresets {
		eqPresets = append(eqPresets, config.EQPresetDef{Name: cp.Name, Key: cp.Key, Bands: cp.Bands})
	}
	eqPresets = append(eqPresets, config.EQPresetDef{Name: eqCustomSentinelName, Key: "custom"})
	o.eqPresets = eqPresets

	presetIdx := config.EQPresetIndex(cfg.EQPreset, eqPresets)
	if presetIdx < 0 {
		presetIdx = len(eqPresets) - 1 // fall back to "(Custom)" sentinel
	}

	o.fields[settingsTabEqualizer] = []settingsField{
		{Label: "Enabled", Value: boolToInt(cfg.EQEnabled), Default: 0, Min: 0, Max: 1, Step: 1, Unit: ""},
		{Label: "Preset", Value: presetIdx, Default: 0, Min: 0, Max: len(eqPresets) - 1, Step: 1, Unit: "preset"},
		{Label: "31 Hz", Value: cfg.EQBands[0], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "62 Hz", Value: cfg.EQBands[1], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "125 Hz", Value: cfg.EQBands[2], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "250 Hz", Value: cfg.EQBands[3], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "500 Hz", Value: cfg.EQBands[4], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "1 kHz", Value: cfg.EQBands[5], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "2 kHz", Value: cfg.EQBands[6], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "4 kHz", Value: cfg.EQBands[7], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "8 kHz", Value: cfg.EQBands[8], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
		{Label: "16 kHz", Value: cfg.EQBands[9], Default: 0, Min: -12, Max: 12, Step: 1, Unit: "dB"},
	}

	o.fields[settingsTabDisplay] = []settingsField{
		{Label: "Notification", Value: cfg.TransientDurationSecs, Default: 5, Min: 1, Max: 30, Step: 1, Unit: "s"},
		{Label: "Vol feedback", Value: cfg.VolumeFeedbackDurationSecs, Default: 2, Min: 1, Max: 10, Step: 1, Unit: "s"},
	}
	return o
}

// EQState returns the current EQ enabled flag and band values from the overlay
// without triggering a full ToConfig merge. Used for live preview.
func (o SettingsOverlay) EQState() (enabled bool, bands [10]int) {
	eq := o.fields[settingsTabEqualizer]
	enabled = eq[0].Value != 0
	for i := range 10 {
		bands[i] = eq[i+2].Value
	}
	return
}

// StereoWideState returns the current stereo wide enabled flag and amount
// from the overlay without triggering a full ToConfig merge. Used for live preview.
func (o SettingsOverlay) StereoWideState() (enabled bool, amount int) {
	pb := o.fields[settingsTabPlayback]
	enabled = pb[6].Value != 0
	amount = pb[7].Value
	return
}

// HandleKey processes a key event and returns the updated overlay and the resulting action.
// ctrl+c returns settingsNone so the caller can fall through to root quit handling.
func (o SettingsOverlay) HandleKey(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction) {
	// Text input mode: all keys route to the name input.
	if o.eqNameInput != nil {
		switch msg.String() {
		case "enter":
			name := o.eqNameInput.Value()
			o.eqNameInput = nil
			if name != "" && o.addCustomPreset(name) {
				return o, settingsEQSave
			}
			return o, settingsNone
		case "esc":
			o.eqNameInput = nil
			return o, settingsNone
		default:
			o.eqNameInput.HandleKey(msg.String())
			return o, settingsNone
		}
	}

	// Delete confirmation mode.
	if o.eqConfirmDel {
		switch msg.String() {
		case "d", "enter":
			o.deleteCurrentPreset()
			o.eqConfirmDel = false
			return o, settingsEQDelete
		case "esc":
			o.eqConfirmDel = false
			return o, settingsNone
		default:
			return o, settingsNone
		}
	}

	switch msg.String() {
	case "[":
		o.tab = (o.tab + settingsTabCount - 1) % settingsTabCount
		o.cursor = 0
	case "]":
		o.tab = (o.tab + 1) % settingsTabCount
		o.cursor = 0
	case "j", "down":
		if n := len(o.fields[o.tab]); n > 0 {
			o.cursor = (o.cursor + 1) % n
		}
	case "k", "up":
		if n := len(o.fields[o.tab]); n > 0 {
			o.cursor = (o.cursor + n - 1) % n
		}
	case "h", "left":
		if len(o.fields[o.tab]) > 0 {
			f := &o.fields[o.tab][o.cursor]
			if f.Value > f.Min {
				f.Value -= f.Step
				if f.Value < f.Min {
					f.Value = f.Min
				}
				o.dirty = true
				if o.tab == settingsTabEqualizer {
					return o, o.applyEQFieldChange()
				}
				if o.tab == settingsTabPlayback && (o.cursor == 6 || o.cursor == 7) {
					return o, settingsStereoLive
				}
			}
		}
	case "l", "right":
		if len(o.fields[o.tab]) > 0 {
			f := &o.fields[o.tab][o.cursor]
			if f.Value < f.Max {
				f.Value += f.Step
				if f.Value > f.Max {
					f.Value = f.Max
				}
				o.dirty = true
				if o.tab == settingsTabEqualizer {
					return o, o.applyEQFieldChange()
				}
				if o.tab == settingsTabPlayback && (o.cursor == 6 || o.cursor == 7) {
					return o, settingsStereoLive
				}
			}
		}
	case "s":
		if o.tab == settingsTabEqualizer {
			o.eqNameInput = &textInput{}
			return o, settingsNone
		}
	case "d":
		if o.tab == settingsTabEqualizer {
			presetIdx := o.fields[settingsTabEqualizer][1].Value
			if presetIdx >= 0 && presetIdx < len(o.eqPresets) {
				p := o.eqPresets[presetIdx]
				if !p.Builtin && p.Key != "custom" {
					o.eqConfirmDel = true
					return o, settingsNone
				}
			}
		}
	case "esc":
		return o, settingsCancel
	case "ctrl+s":
		return o, settingsSave
	}
	return o, settingsNone
}

// applyEQFieldChange returns the appropriate action after an h/l keypress on
// the Equalizer tab, and applies side-effects (snap bands on preset change,
// reset preset to sentinel on band change).
func (o *SettingsOverlay) applyEQFieldChange() settingsAction {
	eq := o.fields[settingsTabEqualizer]
	switch o.cursor {
	case 0: // Enabled toggle
		return settingsEQLive
	case 1: // Preset — snap all 10 band fields to the selected preset's bands
		presetIdx := eq[1].Value
		if presetIdx >= 0 && presetIdx < len(o.eqPresets) {
			p := o.eqPresets[presetIdx]
			for i := range 10 {
				o.fields[settingsTabEqualizer][i+2].Value = p.Bands[i]
			}
		}
		return settingsEQLive
	default: // Band field (cursor 2–11) — revert preset display to "(Custom)"
		o.fields[settingsTabEqualizer][1].Value = len(o.eqPresets) - 1
		return settingsEQLive
	}
}

// addCustomPreset adds a new custom preset with the given name using the current band values.
// It inserts before the "(Custom)" sentinel and updates the preset field accordingly.
func (o *SettingsOverlay) addCustomPreset(name string) bool {
	base := config.SlugFromName(name)
	if base == "" {
		return false
	}
	key := base
	for n := 2; ; n++ {
		found := false
		for _, p := range o.eqPresets {
			if p.Key == key {
				found = true
				break
			}
		}
		if !found {
			break
		}
		key = fmt.Sprintf("%s_%d", base, n)
	}

	var bands [10]int
	for i := range 10 {
		bands[i] = o.fields[settingsTabEqualizer][i+2].Value
	}

	newPreset := config.EQPresetDef{Name: name, Key: key, Bands: bands}
	sentinel := len(o.eqPresets) - 1
	newPresets := make([]config.EQPresetDef, 0, len(o.eqPresets)+1)
	newPresets = append(newPresets, o.eqPresets[:sentinel]...)
	newPresets = append(newPresets, newPreset)
	newPresets = append(newPresets, o.eqPresets[sentinel])
	o.eqPresets = newPresets

	o.fields[settingsTabEqualizer][1].Max = len(o.eqPresets) - 1
	o.fields[settingsTabEqualizer][1].Value = sentinel // new preset index
	o.dirty = true
	return true
}

// deleteCurrentPreset removes the currently selected custom preset and falls back to Flat.
func (o *SettingsOverlay) deleteCurrentPreset() {
	presetIdx := o.fields[settingsTabEqualizer][1].Value
	newPresets := make([]config.EQPresetDef, 0, len(o.eqPresets)-1)
	newPresets = append(newPresets, o.eqPresets[:presetIdx]...)
	newPresets = append(newPresets, o.eqPresets[presetIdx+1:]...)
	o.eqPresets = newPresets

	o.fields[settingsTabEqualizer][1].Max = len(o.eqPresets) - 1
	o.fields[settingsTabEqualizer][1].Value = 0 // fall back to Flat
	flat := o.eqPresets[0]
	for i := range 10 {
		o.fields[settingsTabEqualizer][i+2].Value = flat.Bands[i]
	}
	o.dirty = true
}

// ToConfig merges the overlay's field values back into a copy of base.
func (o SettingsOverlay) ToConfig(base config.Config) config.Config {
	cfg := base
	pb := o.fields[settingsTabPlayback]
	cfg.VolumeIncrement = pb[0].Value
	cfg.SeekOffsetShort = float64(pb[1].Value)
	cfg.SeekOffsetLong = float64(pb[2].Value)
	cfg.DefaultVolume = pb[3].Value
	cfg.NormalizeVolume = pb[4].Value != 0
	cfg.GaplessAudio = pb[5].Value != 0
	cfg.StereoWideEnabled = pb[6].Value != 0
	cfg.StereoWideAmount = pb[7].Value

	eq := o.fields[settingsTabEqualizer]
	cfg.EQEnabled = eq[0].Value != 0
	if eq[1].Value >= 0 && eq[1].Value < len(o.eqPresets) {
		cfg.EQPreset = o.eqPresets[eq[1].Value].Key
	} else {
		cfg.EQPreset = "custom"
	}
	for i := range 10 {
		cfg.EQBands[i] = eq[i+2].Value
	}
	cfg.EQCustomPresets = nil
	for _, p := range o.eqPresets {
		if !p.Builtin && p.Key != "custom" {
			cfg.EQCustomPresets = append(cfg.EQCustomPresets, config.EQCustomPreset{
				Name: p.Name, Key: p.Key, Bands: p.Bands,
			})
		}
	}

	disp := o.fields[settingsTabDisplay]
	cfg.TransientDurationSecs = disp[0].Value
	cfg.VolumeFeedbackDurationSecs = disp[1].Value
	return cfg
}

// Render produces a vertically and horizontally centered settings box.
func (o SettingsOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	boxInner := max(min(termWidth-8, 80), OverlayMinInnerWidth)
	contentWidth := boxInner - OverlayHPadding

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, boxInner)+cornerTR)

	// Title row (centered).
	title := " ⚙ Settings "
	titlePad := max(0, boxInner-ansi.StringWidth(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+borderV)

	// Separator below title.
	lines = append(lines, crossL+strings.Repeat(borderH, boxInner)+crossR)

	// Tab bar row.
	tabNames := [settingsTabCount]string{"Playback", "Equalizer", "Display"}
	var tabParts []string
	for i := range settingsTabCount {
		name := tabNames[i]
		if settingsTab(i) == o.tab {
			tabParts = append(tabParts, s.Heading.Render("["+name+"]"))
		} else {
			tabParts = append(tabParts, s.Dim.Render(name))
		}
	}
	tabContent := strings.Join(tabParts, "  ")
	tabVisW := ansi.StringWidth(tabContent)
	tabPad := max(0, contentWidth-tabVisW)
	tabRow := "  " + tabContent + strings.Repeat(" ", tabPad) + "  "
	lines = append(lines, borderV+tabRow+borderV)

	// Separator below tab bar.
	lines = append(lines, crossL+strings.Repeat(borderH, boxInner)+crossR)

	// Field rows.
	maxLabelW := 0
	for _, f := range o.fields[o.tab] {
		if lw := len(f.Label); lw > maxLabelW {
			maxLabelW = lw
		}
	}
	for i, f := range o.fields[o.tab] {
		var spinner string
		switch {
		case f.Min == 0 && f.Max == 1 && f.Unit == "":
			label := "Off"
			if f.Value != 0 {
				label = "On"
			}
			spinner = fmt.Sprintf("< %s >", label)
		case f.Unit == "preset":
			name := eqCustomSentinelName
			if f.Value >= 0 && f.Value < len(o.eqPresets) {
				name = o.eqPresets[f.Value].Name
			}
			spinner = fmt.Sprintf("< %s >", name)
		case f.Unit == "tenth":
			spinner = fmt.Sprintf("< %.1fx >", float64(f.Value)/10.0)
		default:
			spinner = fmt.Sprintf("< %d%s >", f.Value, f.Unit)
		}
		spinnerW := ansi.StringWidth(spinner)
		gap := max(1, contentWidth-maxLabelW-spinnerW)
		row := fmt.Sprintf("  %-*s%s%s  ", maxLabelW, f.Label, strings.Repeat(" ", gap), spinner)
		if i == o.cursor {
			lines = append(lines, borderV+s.Accent.Render(row)+borderV)
		} else {
			lines = append(lines, borderV+row+borderV)
		}
		// On EQ tab, insert CRUD state row after the Preset field (index 1).
		if o.tab == settingsTabEqualizer && i == 1 {
			if o.eqNameInput != nil {
				body := "Save as: " + o.eqNameInput.Render(contentWidth)
				pad := max(0, contentWidth-ansi.StringWidth(body))
				lines = append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
			} else if o.eqConfirmDel && f.Value >= 0 && f.Value < len(o.eqPresets) {
				presetName := o.eqPresets[f.Value].Name
				body := fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", presetName)
				pad := max(0, contentWidth-ansi.StringWidth(body))
				lines = append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
			}
		}
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, boxInner)+cornerBR)

	// Hint line.
	if o.tab == settingsTabEqualizer {
		lines = append(lines, "  s save · d delete · [/] tabs · j/k fields · h/l adjust · ctrl+s save · esc cancel")
	} else {
		lines = append(lines, "  [/] tabs · j/k fields · h/l adjust · ctrl+s save · esc cancel")
	}

	// Center the box.
	boxW := ansi.StringWidth(lines[0])
	boxH := len(lines)
	leftPad := 0
	if termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}
	prefix := strings.Repeat(" ", leftPad)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range lines {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}
