package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

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
	tabNames := [settingsTabCount]string{"Library", "Playback", "Equalizer", "Display"}
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

	// Content rows.
	if o.tab == settingsTabLibrary {
		lines = o.renderLibraryTab(lines, contentWidth, s)
	} else {
		lines = o.renderFieldsTab(lines, contentWidth, s)
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, boxInner)+cornerBR)

	// Hint line.
	switch o.tab {
	case settingsTabLibrary:
		if o.libSubView == libSubViewExcluded {
			lines = append(lines, "  a add · d delete · [/] tabs · j/k navigate · ctrl+s save · esc back")
		} else {
			lines = append(lines, "  a add · d delete · e excluded · [/] tabs · j/k navigate · ctrl+s save · esc cancel")
		}
	case settingsTabEqualizer:
		lines = append(lines, "  s save · d delete · [/] tabs · j/k fields · h/l adjust · ctrl+s save · esc cancel")
	default:
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

func (o SettingsOverlay) renderLibraryTab(lines []string, contentWidth int, s styles.Styles) []string {
	if o.libSubView == libSubViewExcluded {
		return o.renderExcludedPaths(lines, contentWidth, s)
	}
	return o.renderLibraryDirs(lines, contentWidth, s)
}

func (o SettingsOverlay) renderExcludedPaths(lines []string, contentWidth int, s styles.Styles) []string {
	if len(o.excludedPaths) == 0 && o.exclPathInput == nil {
		row := "  (none)" + strings.Repeat(" ", max(0, contentWidth-6)) + "  "
		lines = append(lines, borderV+s.Dim.Render(row)+borderV)
	}
	for i, pattern := range o.excludedPaths {
		patW := ansi.StringWidth(pattern)
		pad := max(0, contentWidth-patW)
		row := "  " + pattern + strings.Repeat(" ", pad) + "  "
		if i == o.exclCursor {
			lines = append(lines, borderV+s.Accent.Render(row)+borderV)
		} else {
			lines = append(lines, borderV+row+borderV)
		}
	}
	if o.exclPathInput != nil {
		body := "Add pattern: " + o.exclPathInput.View()
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
	} else if o.exclConfirmDel && o.exclCursor < len(o.excludedPaths) {
		pattern := o.excludedPaths[o.exclCursor]
		body := fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", pattern)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
	}
	return lines
}

func (o SettingsOverlay) renderLibraryDirs(lines []string, contentWidth int, s styles.Styles) []string {
	for i, dir := range o.libraryDirs {
		display := collapseHomeDir(dir)
		displayW := ansi.StringWidth(display)
		pad := max(0, contentWidth-displayW)
		row := "  " + display + strings.Repeat(" ", pad) + "  "
		if i == o.libCursor {
			lines = append(lines, borderV+s.Accent.Render(row)+borderV)
		} else {
			lines = append(lines, borderV+row+borderV)
		}
	}
	if o.libDirInput != nil {
		body := "Add dir: " + o.libDirInput.View()
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
	} else if o.libConfirmDel && o.libCursor < len(o.libraryDirs) {
		dirName := collapseHomeDir(o.libraryDirs[o.libCursor])
		body := fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", dirName)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		lines = append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
	}
	return lines
}

func (o SettingsOverlay) renderFieldsTab(lines []string, contentWidth int, s styles.Styles) []string {
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
		if o.tab == settingsTabEqualizer && i == 1 {
			lines = o.renderEQStateRow(lines, contentWidth, f)
		}
	}
	return lines
}

func (o SettingsOverlay) renderEQStateRow(lines []string, contentWidth int, presetField settingsField) []string {
	if o.eqNameInput != nil {
		body := "Save as: " + o.eqNameInput.View()
		pad := max(0, contentWidth-ansi.StringWidth(body))
		return append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
	}
	if o.eqConfirmDel && presetField.Value >= 0 && presetField.Value < len(o.eqPresets) {
		presetName := o.eqPresets[presetField.Value].Name
		body := fmt.Sprintf("Delete %q? D/Enter=yes  Esc=no", presetName)
		pad := max(0, contentWidth-ansi.StringWidth(body))
		return append(lines, borderV+"  "+body+strings.Repeat(" ", pad)+"  "+borderV)
	}
	return lines
}
