package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (cp CommandPalette) pickHelp() tea.Cmd {
	for _, command := range cp.all {
		if command.Pinned == PinTop && command.IsEnabled() {
			return func() tea.Msg { return CommandPalettePickedMsg{Command: command} }
		}
	}
	return nil
}

func (cp CommandPalette) page(key string, heights []int) CommandPalette {
	height := 15
	if len(heights) > 0 {
		height = heights[0]
	}
	plan := cp.planRows(cpCommandRowBudget(height), strings.TrimSpace(cp.Filter.Text) == "")
	step := max(plan.bodyBudget, 1)
	if key == "pgup" {
		step = -step
	}
	cp.cursor = clamp(cp.cursor+step, 0, max(len(cp.Filtered)-1, 0))
	return cp
}

func commandPriority(command PaletteCommand) int {
	if !command.IsEnabled() {
		return 0
	}
	if command.Intent == IntentListening {
		return 4
	}
	if command.Intent == IntentRemoval {
		return 1
	}
	if command.Relevant {
		return 3
	}
	return 2
}
