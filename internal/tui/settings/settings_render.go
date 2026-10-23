package settings

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render produces a horizontally centered, top-anchored settings box. The box
// keeps a fixed frame across tabs, with scrolling content and a pinned footer.
func (o SettingsOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	if o.discard.Active {
		return o.discard.Render(termWidth, termHeight, s)
	}
	termHeight = overlay.PanelHeight(termHeight)
	bodyHeight := overlay.PanelContentHeight(termHeight)
	boxInner := overlay.PanelInnerWidth(termWidth)
	contentWidth := boxInner - overlay.HPadding

	var lines []string

	// Top border.
	lines = append(lines, overlay.CornerTL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerTR)

	// Title row (centered).
	changes := o.changedFields()
	count := 0
	for _, fields := range changes {
		count += changedCount(fields)
	}
	title := text.Truncate(o.title(count), boxInner)
	titlePad := max(0, boxInner-ansi.StringWidth(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, overlay.BorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+overlay.BorderV)

	// Separator below title.
	lines = append(lines, overlay.CrossL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CrossR)

	// Tab bar row.
	tabNames := [tabCount]string{"Library", "Playback", "Equalizer", "Display"}
	var tabParts []string
	for i := range tabCount {
		name := tabNames[i]
		if changedCount(changes[i]) > 0 {
			name += "*"
		}
		if Tab(i) == o.Tab {
			tabParts = append(tabParts, s.Heading.Render("["+name+"]"))
		} else {
			tabParts = append(tabParts, s.Dim.Render(name))
		}
	}
	tabContent := strings.Join(tabParts, " ")
	tabContent = text.Truncate(tabContent, contentWidth)
	tabVisW := ansi.StringWidth(tabContent)
	tabPad := max(0, contentWidth-tabVisW)
	tabRow := "  " + tabContent + strings.Repeat(" ", tabPad) + "  "
	lines = append(lines, overlay.BorderV+tabRow+overlay.BorderV)

	// Separator below tab bar.
	lines = append(lines, overlay.CrossL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CrossR)

	// Content rows.
	if o.Tab == tabLibrary {
		lines = o.renderLibraryTab(lines, contentWidth, bodyHeight, s)
	} else {
		lines = o.renderFieldsTab(lines, contentWidth, bodyHeight, s)
	}

	// Hints get spare rows before explanatory copy does.
	hints := text.HintLines(o.keyHint(), boxInner-2, overlay.PanelHintRows(termHeight), "ctrl+s", "esc", "enter", "tab/shift+tab", "j/k")
	lines = o.appendDescription(lines, contentWidth, termHeight-len(lines)-1-overlay.PanelHintRows(termHeight), s)
	// Bottom border.
	lines = append(lines, overlay.CornerBL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerBR)

	for _, hint := range hints {
		lines = append(lines, "  "+s.Dim.Render(hint))
	}
	return overlay.RenderPanelLines(lines, termWidth, termHeight)
}

func (o SettingsOverlay) keyHint() string {
	if o.EQNameInput != nil {
		return "enter save preset · esc cancel"
	}
	if o.EQConfirmDel || o.libConfirmDel || o.exclConfirmDel {
		return "enter delete · esc cancel"
	}
	switch o.Tab {
	case tabLibrary:
		return o.libraryHint()
	case TabDisplay:
		return o.displayHint()
	case TabEqualizer:
		return "s save preset · d delete · tab/shift+tab tabs · j/k navigate · h/l adjust · ctrl+s save · esc cancel"
	default:
		return "tab/shift+tab tabs · j/k navigate · h/l adjust · ctrl+s save · esc cancel"
	}
}

func (o SettingsOverlay) libraryHint() string {
	if o.LibDirInput != nil || o.ExclPathInput != nil {
		return "enter add · esc cancel"
	}
	if o.libSubView == libSubViewExcluded {
		return "a add · d delete · tab/shift+tab tabs · j/k navigate · ctrl+s save · esc back"
	}
	return "a add · d delete · e excluded · tab/shift+tab tabs · j/k navigate · ctrl+s save · esc cancel"
}

func (o SettingsOverlay) renderLibraryTab(lines []string, contentWidth, termHeight int, s styles.Styles) []string {
	if o.libSubView == libSubViewExcluded {
		return o.renderExcludedPaths(lines, contentWidth, termHeight, s)
	}
	return o.renderLibraryDirs(lines, contentWidth, termHeight, s)
}

func (o SettingsOverlay) renderExcludedPaths(lines []string, contentWidth, termHeight int, s styles.Styles) []string {
	if len(o.ExcludedPaths) == 0 && o.ExclPathInput == nil {
		row := "  (none)" + strings.Repeat(" ", max(0, contentWidth-6)) + "  "
		lines = append(lines, overlay.BorderV+s.Dim.Render(row)+overlay.BorderV)
	}
	extra := 0
	if o.ExclPathInput != nil || o.exclConfirmDel {
		extra++
	}
	start, end := settingsRowWindow(len(o.ExcludedPaths), o.exclCursor, termHeight, extra)
	for i := start; i < end; i++ {
		pattern := o.ExcludedPaths[i] + changedValueMark(o.ExcludedPaths[i], o.initialConfig.ExcludedPaths)
		pattern = text.Truncate(pattern, contentWidth)
		patW := ansi.StringWidth(pattern)
		pad := max(0, contentWidth-patW)
		row := text.FocusMarker(i == o.exclCursor) + pattern + strings.Repeat(" ", pad) + "  "
		if i == o.exclCursor {
			lines = append(lines, overlay.BorderV+s.Accent.Render(row)+overlay.BorderV)
		} else {
			lines = append(lines, overlay.BorderV+row+overlay.BorderV)
		}
	}
	if o.ExclPathInput != nil {
		body := text.Truncate("Add pattern: "+o.ExclPathInput.View(), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
	} else if o.exclConfirmDel && o.exclCursor < len(o.ExcludedPaths) {
		pattern := o.ExcludedPaths[o.exclCursor]
		body := text.Truncate(fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", pattern), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
	}
	return lines
}

func (o SettingsOverlay) renderLibraryDirs(lines []string, contentWidth, termHeight int, s styles.Styles) []string {
	extra := 0
	if o.LibDirInput != nil || o.libConfirmDel {
		extra++
	}
	if o.libDirError != "" {
		extra++
	}
	start, end := settingsRowWindow(len(o.LibraryDirs), o.libCursor, termHeight, extra)
	for i := start; i < end; i++ {
		dir := o.LibraryDirs[i]
		label := collapseHomeDir(dir) + changedValueMark(dir, o.initialConfig.MusicDirectories)
		display := text.Truncate(label, contentWidth)
		displayW := ansi.StringWidth(display)
		pad := max(0, contentWidth-displayW)
		row := text.FocusMarker(i == o.libCursor) + display + strings.Repeat(" ", pad) + "  "
		if i == o.libCursor {
			lines = append(lines, overlay.BorderV+s.Accent.Render(row)+overlay.BorderV)
		} else {
			lines = append(lines, overlay.BorderV+row+overlay.BorderV)
		}
	}
	if o.LibDirInput != nil {
		body := text.Truncate("Add dir: "+o.LibDirInput.View(), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
		if o.libDirError != "" {
			lines = append(lines, overlay.BorderV+"  "+s.Error.Render(text.Pad(o.libDirError, contentWidth))+"  "+overlay.BorderV)
		}
	} else if o.libConfirmDel && o.libCursor < len(o.LibraryDirs) {
		dirName := collapseHomeDir(o.LibraryDirs[o.libCursor])
		body := text.Truncate(fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", dirName), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
	}
	return lines
}

func (o SettingsOverlay) renderFieldsTab(lines []string, contentWidth, termHeight int, s styles.Styles) []string {
	changes := o.changedFields()[o.Tab]
	fields := slices.Clone(o.Fields[o.Tab])
	maxLabelW := 0
	for i := range fields {
		if changes[i] {
			fields[i].label += " *"
		}
		maxLabelW = max(maxLabelW, len(fields[i].label))
	}
	start, end := o.visibleFieldRange(termHeight)
	for i := start; i < end; i++ {
		f := fields[i]
		lines = append(lines, o.renderFieldRow(f, i == o.cursor, maxLabelW, contentWidth, s))
		if o.Tab == TabEqualizer && i == 1 {
			lines = o.renderEQStateRow(lines, contentWidth, f)
		}
	}
	if o.Tab == TabDisplay {
		lines = o.renderPrimaryEditor(lines, contentWidth, s)
	}
	return lines
}

// renderFieldRow lays out one settings row: a left-aligned label, then the
// field's spinner pushed to the right edge of the content area.
func (o SettingsOverlay) renderFieldRow(f Field, focused bool, maxLabelW, contentWidth int, s styles.Styles) string {
	spinner := o.fieldSpinner(f)
	gap := max(1, contentWidth-maxLabelW-ansi.StringWidth(spinner))
	rowBody := fmt.Sprintf("%-*s%s%s", maxLabelW, f.label, strings.Repeat(" ", gap), spinner)
	rowBody = text.Truncate(rowBody, contentWidth)
	rowPad := max(0, contentWidth-ansi.StringWidth(rowBody))
	row := text.FocusMarker(focused) + rowBody + strings.Repeat(" ", rowPad) + "  "
	if focused {
		return overlay.BorderV + s.Accent.Render(row) + overlay.BorderV
	}
	return overlay.BorderV + row + overlay.BorderV
}

// fieldSpinner renders the "< value >" control for a field, formatting the
// value according to the field's declared unit.
func (o SettingsOverlay) fieldSpinner(f Field) string {
	switch f.unit {
	case "theme":
		return fmt.Sprintf("< %s >", themeLabels[f.Value])
	case "color":
		if o.primaryColor == "" {
			return "Preset default [enter]"
		}
		return o.primaryColor + " [enter]"
	case "preset":
		return o.presetSpinner(f.Value)
	case "tenth":
		return fmt.Sprintf("< %.1fx >", float64(f.Value)/10.0)
	case "":
		if f.min == 0 && f.max == 1 {
			return boolSpinner(f.Value != 0)
		}
	}
	return fmt.Sprintf("< %d%s >", f.Value, f.unit)
}

func boolSpinner(enabled bool) string {
	if enabled {
		return "< On >"
	}
	return "< Off >"
}

func (o SettingsOverlay) presetSpinner(index int) string {
	name := eqCustomSentinelName
	if index >= 0 && index < len(o.EQPresets) {
		name = o.EQPresets[index].Name
	}
	return fmt.Sprintf("< %s >", name)
}

func (o SettingsOverlay) renderEQStateRow(lines []string, contentWidth int, presetField Field) []string {
	if o.EQNameInput != nil {
		body := text.Truncate("Save as: "+o.EQNameInput.View(), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		return append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
	}
	if o.EQConfirmDel && presetField.Value >= 0 && presetField.Value < len(o.EQPresets) {
		presetName := o.EQPresets[presetField.Value].Name
		body := text.Truncate(fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", presetName), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		return append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
	}
	return lines
}

func (o SettingsOverlay) title(changes int) string {
	if changes > 0 {
		return fmt.Sprintf("Settings · %d unsaved %s", changes, text.Pluralize(changes, "change", "changes"))
	}
	if !o.editingText() {
		return "Settings · tab/shift+tab tabs"
	}
	return " ⚙ Settings "
}
