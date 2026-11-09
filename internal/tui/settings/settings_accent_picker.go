package settings

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// accentPicker is the swatch grid opened from the Accent row. Every named
// accent is visible at once, plus a trailing Custom cell for a hex color.
// Moving the cursor previews that accent; enter keeps it, esc restores the
// accent the picker opened with.
type accentPicker struct {
	cursor   int
	original string
}

// accentCols is fixed so j/k move by the same amount the grid shows: four
// columns keep all fourteen cells within four rows at the minimum height.
const accentCols = 4

// accentCellCount is the named accents plus the Custom cell.
func accentCellCount() int { return len(config.AccentPresets) + 1 }

func accentCustomCell() int { return len(config.AccentPresets) }

// accentCellWidth spreads the grid across width: room for the focus marker,
// swatch and longest name, wider on roomy terminals, narrower (names cut)
// only when the panel is very tight.
func accentCellWidth(width int) int { return max(4, min(15, width/accentCols)) }

// accentCellFor is the cell showing primary: its name, or Custom for a hex.
func accentCellFor(primary string) int {
	if primary == "" {
		return 0
	}
	if i := slices.Index(config.AccentPresets, primary); i >= 0 {
		return i
	}
	return accentCustomCell()
}

func (o SettingsOverlay) openAccentPicker() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab != TabDisplay || o.cursor != displayPrimary {
		return o, actionNone, nil
	}
	o.accentPicker = &accentPicker{cursor: accentCellFor(o.primaryColor), original: o.primaryColor}
	return o, actionNone, nil
}

// moveAccentCursor moves to cell and previews it. The Custom cell previews the
// accent the picker opened with, so leaving through it changes nothing.
func (o SettingsOverlay) moveAccentCursor(cell int) (SettingsOverlay, Action, tea.Cmd) {
	p := *o.accentPicker
	cell = max(0, min(accentCellCount()-1, cell))
	if cell == p.cursor {
		return o, actionNone, nil
	}
	p.cursor = cell
	o.accentPicker = &p
	next := p.original
	if cell != accentCustomCell() {
		next, _ = config.NormalizePrimaryColor(config.AccentPresets[cell])
	}
	if next == o.primaryColor {
		return o, actionNone, nil
	}
	o.primaryColor = next
	o.dirty = true
	return o, ActionThemeLive, nil
}

// accentPickerMoves maps each navigation key to the cell it moves to, given
// the current cell and the cell count. h/l wrap; j/k move a grid row.
var accentPickerMoves = map[string]func(cur, n int) int{
	"h":     func(cur, n int) int { return (cur - 1 + n) % n },
	"left":  func(cur, n int) int { return (cur - 1 + n) % n },
	"l":     func(cur, n int) int { return (cur + 1) % n },
	"right": func(cur, n int) int { return (cur + 1) % n },
	"j":     func(cur, _ int) int { return cur + accentCols },
	"down":  func(cur, _ int) int { return cur + accentCols },
	"k":     func(cur, _ int) int { return cur - accentCols },
	"up":    func(cur, _ int) int { return cur - accentCols },
	"g":     func(int, int) int { return 0 },
	"home":  func(int, int) int { return 0 },
	"G":     func(_, n int) int { return n - 1 },
	"end":   func(_, n int) int { return n - 1 },
}

func (o SettingsOverlay) handleAccentPicker(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	p := *o.accentPicker
	if move, ok := accentPickerMoves[msg.String()]; ok {
		return o.moveAccentCursor(move(p.cursor, accentCellCount()))
	}
	switch msg.String() {
	case "enter":
		o.accentPicker = nil
		if p.cursor == accentCustomCell() {
			return o.beginPrimaryEdit()
		}
		return o, actionNone, nil
	case "ctrl+s":
		o.accentPicker = nil
		return o, ActionSave, nil
	case "esc":
		o.accentPicker = nil
		if o.primaryColor == p.original {
			return o, actionNone, nil
		}
		o.primaryColor = p.original
		return o, ActionThemeLive, nil
	}
	return o, actionNone, nil
}

// renderAccentPicker lays the swatches out in a grid, each dot drawn in the
// shade that accent takes in the current preset, scrolling by grid row when
// the terminal is too short to show them all.
func (o SettingsOverlay) renderAccentPicker(lines []string, width, termHeight int, s styles.Styles) []string {
	p := o.accentPicker
	cols, cellW := accentCols, accentCellWidth(width)
	n := accentCellCount()
	rows := (n + cols - 1) / cols
	heading := text.Truncate("Accent · "+accentLabel(o.primaryColor), width)
	lines = append(lines, overlay.BorderV+"  "+s.Bold.Render(heading)+strings.Repeat(" ", max(0, width-ansi.StringWidth(heading)))+"  "+overlay.BorderV)
	start, end := settingsRowWindow(rows, p.cursor/cols, termHeight, 1)
	for r := start; r < end; r++ {
		var row strings.Builder
		used := 0
		for c := range cols {
			i := r*cols + c
			if i >= n {
				break
			}
			cell := o.renderAccentCell(i, i == p.cursor, cellW, s)
			row.WriteString(cell)
			used += ansi.StringWidth(cell)
		}
		lines = append(lines, overlay.BorderV+"  "+row.String()+strings.Repeat(" ", max(0, width-used))+"  "+overlay.BorderV)
	}
	return lines
}

func (o SettingsOverlay) renderAccentCell(i int, focused bool, cellW int, s styles.Styles) string {
	name, label, dot := "", "Custom", "◌"
	if i < accentCustomCell() {
		name = config.AccentPresets[i]
		label, dot = capitalize(name), "●"
	} else if accentCellFor(o.accentPicker.original) == accentCustomCell() {
		name, dot = o.accentPicker.original, "●"
	}
	swatch := s.Dim.Render(dot)
	if name != "" {
		swatch = styles.TagColorStyle(styles.Hex(styles.NewTheme(s.HasDark, name).Primary)).Render(dot)
	}
	label = text.Truncate(label, max(0, cellW-5))
	body := text.FocusMarker(focused) + swatch + " " + label
	body += strings.Repeat(" ", max(0, cellW-ansi.StringWidth(body)))
	if focused {
		return s.Accent.Render(body)
	}
	return body
}
