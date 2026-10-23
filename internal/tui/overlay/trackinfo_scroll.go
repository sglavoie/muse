package overlay

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Wrap before paging so even long paths and comments remain fully readable.
func (t TrackInfoOverlay) wrappedRows(width int) []Row {
	contentWidth := max(1, width-10-HPadding)
	labelWidth := min(boxLabelWidth(t.rows), contentWidth-2)
	valueWidth := max(1, contentWidth-labelWidth-1)
	var rows []Row
	for _, row := range t.rows {
		if row.Blank {
			rows = append(rows, row)
			continue
		}
		wrapWidth := contentWidth
		if row.Label != "" {
			wrapWidth = valueWidth
		}
		value := strings.ReplaceAll(ansi.Strip(row.Text), "\t", "    ")
		for i, line := range strings.Split(ansi.Wrap(value, wrapWidth, " "), "\n") {
			if row.Label != "" {
				label := ""
				if i == 0 {
					label = row.Label
				}
				line = text.PadLeft(label, labelWidth) + " " + line
			}
			rows = append(rows, Row{Text: text.Pad(line, contentWidth), Styled: row.Styled})
		}
	}
	return rows
}

func (t *TrackInfoOverlay) Scroll(key string, width, height int) {
	visible := max(1, height-7)
	end := max(0, len(t.wrappedRows(width))-visible)
	t.offset = max(0, min(t.offset, end))
	switch key {
	case "j", "down":
		t.offset++
	case "k", "up":
		t.offset--
	case "pgdown", "ctrl+d":
		t.offset += visible
	case "pgup", "ctrl+u":
		t.offset -= visible
	case "g", "home":
		t.offset = 0
	case "G", "end":
		t.offset = end
	}
	t.offset = max(0, min(t.offset, end))
}
