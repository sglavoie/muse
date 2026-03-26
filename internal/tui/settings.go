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
	settingsTabPlayback settingsTab = iota
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
	settingsNone   settingsAction = iota
	settingsSave                  // ctrl+s was pressed
	settingsCancel                // esc was pressed
)

// SettingsOverlay is a centered overlay for editing runtime-tunable config values.
type SettingsOverlay struct {
	tab    settingsTab
	cursor int
	fields [settingsTabCount][]settingsField
	dirty  bool
}

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
	}
	o.fields[settingsTabDisplay] = []settingsField{
		{Label: "Notification", Value: cfg.TransientDurationSecs, Default: 5, Min: 1, Max: 30, Step: 1, Unit: "s"},
		{Label: "Vol feedback", Value: cfg.VolumeFeedbackDurationSecs, Default: 2, Min: 1, Max: 10, Step: 1, Unit: "s"},
	}
	return o
}

// HandleKey processes a key event and returns the updated overlay and the resulting action.
// ctrl+c returns settingsNone so the caller can fall through to root quit handling.
func (o SettingsOverlay) HandleKey(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction) {
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
			}
		}
	case "esc":
		return o, settingsCancel
	case "ctrl+s":
		return o, settingsSave
	}
	return o, settingsNone
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
	tabNames := [settingsTabCount]string{"Playback", "Display"}
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
		if f.Min == 0 && f.Max == 1 && f.Unit == "" {
			label := "Off"
			if f.Value != 0 {
				label = "On"
			}
			spinner = fmt.Sprintf("< %s >", label)
		} else {
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
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, boxInner)+cornerBR)

	// Hint line.
	lines = append(lines, "  [/] tabs · j/k fields · h/l adjust · ctrl+s save · esc cancel")

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
