package settings

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

const (
	displayTheme   = 3
	displayPrimary = 4
)

var themeNames = []string{"auto", "dark", "light"}
var themeLabels = []string{"Auto", "Dark", "Light"}

func (o *SettingsOverlay) initDisplay(cfg config.Config) {
	idx := max(0, slices.Index(themeNames, cfg.Theme))
	o.primaryColor = cfg.PrimaryColor
	o.Fields[TabDisplay] = []Field{
		{label: "Notification", Value: cfg.TransientDurationSecs, defaultValue: 5, min: 1, max: 30, step: 1, unit: "s"},
		{label: "Vol feedback", Value: cfg.VolumeFeedbackDurationSecs, defaultValue: 2, min: 1, max: 10, step: 1, unit: "s"},
		{label: "Track notify", Value: boolToInt(cfg.NotifyOnTrackChange), defaultValue: 0, min: 0, max: 1, step: 1},
		{label: "Theme", Value: idx, min: 0, max: len(themeNames) - 1, step: 1, unit: "theme"},
		{label: "Primary color", unit: "color"},
	}
}

// ThemeState returns the draft appearance without applying unrelated settings.
func (o SettingsOverlay) ThemeState() (preset, primary string) {
	return themeNames[o.Fields[TabDisplay][displayTheme].Value], o.primaryColor
}

func (o SettingsOverlay) beginPrimaryEdit() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab != TabDisplay || o.cursor != displayPrimary {
		return o, actionNone, nil
	}
	o.PrimaryInput = NewSettingsInput()
	o.PrimaryInput.CharLimit = 64
	o.PrimaryInput.SetValue(o.primaryColor)
	o.PrimaryInput.CursorEnd()
	o.primaryError = ""
	return o, actionNone, nil
}

func (o SettingsOverlay) resetPrimary() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab != TabDisplay || o.cursor != displayPrimary {
		return o, actionNone, nil
	}
	o.primaryColor = ""
	o.dirty = true
	return o, ActionThemeLive, nil
}

func (o SettingsOverlay) handlePrimaryInput(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		value, ok := config.NormalizePrimaryColor(o.PrimaryInput.Value())
		if !ok {
			o.primaryError = "Use #RGB or #RRGGBB, or leave blank for default."
			return o, actionNone, nil
		}
		o.primaryColor = value
		o.PrimaryInput = nil
		o.primaryError = ""
		o.dirty = true
		return o, ActionThemeLive, nil
	case "esc":
		o.PrimaryInput = nil
		o.primaryError = ""
		return o, actionNone, nil
	default:
		updated, cmd := o.PrimaryInput.Update(msg)
		o.PrimaryInput = &updated
		return o, actionNone, cmd
	}
}

func (o SettingsOverlay) renderPrimaryEditor(lines []string, width int, s styles.Styles) []string {
	if o.PrimaryInput == nil {
		return lines
	}
	input := *o.PrimaryInput
	input.SetWidth(max(1, width-8))
	inputStyles := textinput.DefaultStyles(s.HasDark)
	inputStyles.Focused.Text = s.TextPrimary
	inputStyles.Focused.Placeholder = s.Dim
	inputStyles.Cursor.Color = s.AccentMark.GetForeground()
	input.SetStyles(inputStyles)
	body := text.Truncate("Color: "+input.View(), width)
	lines = append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", max(0, width-ansi.StringWidth(body)))+"  "+overlay.BorderV)
	if o.primaryError != "" {
		body = text.Truncate(o.primaryError, width)
		lines = append(lines, overlay.BorderV+"  "+s.Error.Render(body)+strings.Repeat(" ", max(0, width-ansi.StringWidth(body)))+"  "+overlay.BorderV)
	}
	return lines
}

// Keep the focused appearance field and validation feedback visible at the
// minimum terminal height; scroll the ordinary display rows out of the way.
func (o SettingsOverlay) visibleFieldRange(termHeight int) (int, int) {
	n := len(o.Fields[o.Tab])
	if o.Tab != TabDisplay || termHeight <= 0 {
		return 0, n
	}
	extra := 0
	if o.PrimaryInput != nil {
		extra++
	}
	if o.primaryError != "" {
		extra++
	}
	count := min(n, max(1, termHeight-7-extra))
	start := max(0, min(o.cursor-count+1, n-count))
	return start, start + count
}

func (o SettingsOverlay) displayHint() string {
	if o.PrimaryInput != nil {
		return "#RGB or #RRGGBB · blank for default · enter apply · esc back"
	}
	if o.cursor == displayPrimary {
		return "enter edit · r reset · j/k move · ctrl+s save · esc cancel"
	}
	return "j/k move · h/l set · [/] tabs · ctrl+s save · esc cancel"
}
