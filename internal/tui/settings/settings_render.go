package settings

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render produces a horizontally centered, top-anchored settings box. The box
// keeps a fixed width so switching tabs only changes its height, and it grows
// downward from the top rather than re-centering on every tab.
func (o SettingsOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	if o.discard.Active {
		return o.discard.Render(termWidth, termHeight, s)
	}
	boxInner := max(min(termWidth-8, 80), overlay.MinInnerWidth)
	contentWidth := boxInner - overlay.HPadding

	var lines []string

	// Top border.
	lines = append(lines, overlay.CornerTL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerTR)

	// Title row (centered).
	title := " ⚙ Settings "
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
		if Tab(i) == o.Tab {
			tabParts = append(tabParts, s.Heading.Render("["+name+"]"))
		} else {
			tabParts = append(tabParts, s.Dim.Render(name))
		}
	}
	tabContent := strings.Join(tabParts, "  ")
	tabContent = text.Truncate(tabContent, contentWidth)
	tabVisW := ansi.StringWidth(tabContent)
	tabPad := max(0, contentWidth-tabVisW)
	tabRow := "  " + tabContent + strings.Repeat(" ", tabPad) + "  "
	lines = append(lines, overlay.BorderV+tabRow+overlay.BorderV)

	// Separator below tab bar.
	lines = append(lines, overlay.CrossL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CrossR)

	// Content rows.
	if o.Tab == tabLibrary {
		lines = o.renderLibraryTab(lines, contentWidth, s)
	} else {
		lines = o.renderFieldsTab(lines, contentWidth, termHeight, s)
	}

	// Bottom border.
	lines = append(lines, overlay.CornerBL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerBR)

	var hint string
	switch o.Tab {
	case tabLibrary:
		hint = o.libraryHint()
	case TabDisplay:
		hint = o.displayHint()
	case TabEqualizer:
		hint = "s save preset · d delete · [/] tabs · j/k navigate · h/l adjust · ctrl+s save · esc cancel"
	default:
		hint = "[/] tabs · j/k navigate · h/l adjust · ctrl+s save · esc cancel"
	}

	return overlay.RenderTopLines(lines, hint, termWidth)
}

func (o SettingsOverlay) libraryHint() string {
	if o.LibDirInput != nil {
		hint := "enter add · esc cancel"
		if o.libDirError != "" {
			return o.libDirError + " · " + hint
		}
		return hint
	}
	if o.libSubView == libSubViewExcluded {
		return "a add · d delete · [/] tabs · j/k navigate · ctrl+s save · esc back"
	}
	return "a add · d delete · e excluded · [/] tabs · j/k navigate · ctrl+s save · esc cancel"
}

func (o SettingsOverlay) renderLibraryTab(lines []string, contentWidth int, s styles.Styles) []string {
	if o.libSubView == libSubViewExcluded {
		return o.renderExcludedPaths(lines, contentWidth, s)
	}
	return o.renderLibraryDirs(lines, contentWidth, s)
}

func (o SettingsOverlay) renderExcludedPaths(lines []string, contentWidth int, s styles.Styles) []string {
	if len(o.ExcludedPaths) == 0 && o.ExclPathInput == nil {
		row := "  (none)" + strings.Repeat(" ", max(0, contentWidth-6)) + "  "
		lines = append(lines, overlay.BorderV+s.Dim.Render(row)+overlay.BorderV)
	}
	for i, pattern := range o.ExcludedPaths {
		pattern = text.Truncate(pattern, contentWidth)
		patW := ansi.StringWidth(pattern)
		pad := max(0, contentWidth-patW)
		row := "  " + pattern + strings.Repeat(" ", pad) + "  "
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

func (o SettingsOverlay) renderLibraryDirs(lines []string, contentWidth int, s styles.Styles) []string {
	for i, dir := range o.LibraryDirs {
		display := text.Truncate(collapseHomeDir(dir), contentWidth)
		displayW := ansi.StringWidth(display)
		pad := max(0, contentWidth-displayW)
		row := "  " + display + strings.Repeat(" ", pad) + "  "
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
	} else if o.libConfirmDel && o.libCursor < len(o.LibraryDirs) {
		dirName := collapseHomeDir(o.LibraryDirs[o.libCursor])
		body := text.Truncate(fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", dirName), contentWidth)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, overlay.BorderV+"  "+body+strings.Repeat(" ", pad)+"  "+overlay.BorderV)
	}
	return lines
}

func (o SettingsOverlay) renderFieldsTab(lines []string, contentWidth, termHeight int, s styles.Styles) []string {
	maxLabelW := 0
	for _, f := range o.Fields[o.Tab] {
		maxLabelW = max(maxLabelW, len(f.label))
	}
	start, end := o.visibleFieldRange(termHeight)
	for i := start; i < end; i++ {
		f := o.Fields[o.Tab][i]
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
	row := "  " + rowBody + strings.Repeat(" ", rowPad) + "  "
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
