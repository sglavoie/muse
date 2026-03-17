package playlists

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

const (
	queueLabel  = "Play Queue"
	createLabel = "New playlist..."

	borderH  = "─"
	borderV  = "│"
	cornerTL = "╭"
	cornerTR = "╮"
	cornerBL = "╰"
	cornerBR = "╯"
	crossL   = "├"
	crossR   = "┤"
)

// DestinationPickedMsg is dispatched when the user confirms a selection in the
// destination picker modal by pressing Enter. IsQueue is true when the play
// queue was chosen; otherwise PlaylistName names the selected playlist.
type DestinationPickedMsg struct {
	IsQueue      bool
	PlaylistName string
}

// DestinationPickerDismissedMsg is dispatched when the user presses Esc to
// close the modal without making a selection.
type DestinationPickerDismissedMsg struct{}

// DestinationPickerCreateMsg is dispatched when the user selects the
// "New playlist..." entry that is shown when no playlists exist.
type DestinationPickerCreateMsg struct{}

// DestinationPickerModal is a focused overlay sub-model for choosing where to
// append tracks. It lists the play queue as the first entry followed by all
// saved playlists in alphabetical order. When no playlists exist, a "New
// playlist..." entry is shown as an inline create-playlist prompt so the user
// can initiate playlist creation without leaving the modal.
type DestinationPickerModal struct {
	playlists []db.PlaylistRow
	cursor    int
}

// NewDestinationPicker returns a DestinationPickerModal populated with the
// given playlists. The slice should be pre-sorted alphabetically, as returned
// by db.ReadPlaylists.
func NewDestinationPicker(playlists []db.PlaylistRow) DestinationPickerModal {
	return DestinationPickerModal{playlists: playlists}
}

// destinations returns the ordered list of destination labels: the play queue
// first, then each playlist name. When no playlists exist, a "New playlist..."
// entry is appended as the inline create prompt.
func (m DestinationPickerModal) destinations() []string {
	labels := make([]string, 0, 1+len(m.playlists)+1)
	labels = append(labels, queueLabel)
	for _, p := range m.playlists {
		labels = append(labels, p.Name)
	}
	if len(m.playlists) == 0 {
		labels = append(labels, createLabel)
	}
	return labels
}

// HandleKey processes a single raw key string and returns the updated modal and
// an optional Cmd.
//
//   - j / down: move cursor down
//   - k / up:   move cursor up
//   - enter:    dispatch DestinationPickedMsg or DestinationPickerCreateMsg
//   - esc:      dispatch DestinationPickerDismissedMsg
func (m DestinationPickerModal) HandleKey(key string) (DestinationPickerModal, tea.Cmd) {
	n := len(m.destinations())
	switch key {
	case "j", "down":
		if m.cursor < n-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		return m, m.confirmCmd()
	case "esc":
		return m, func() tea.Msg { return DestinationPickerDismissedMsg{} }
	}
	return m, nil
}

// confirmCmd builds the Cmd that confirms the current selection.
func (m DestinationPickerModal) confirmCmd() tea.Cmd {
	// When no playlists exist the list is [Play Queue, New playlist...].
	if len(m.playlists) == 0 {
		if m.cursor == 1 {
			return func() tea.Msg { return DestinationPickerCreateMsg{} }
		}
		return func() tea.Msg { return DestinationPickedMsg{IsQueue: true} }
	}
	if m.cursor == 0 {
		return func() tea.Msg { return DestinationPickedMsg{IsQueue: true} }
	}
	name := m.playlists[m.cursor-1].Name
	return func() tea.Msg { return DestinationPickedMsg{PlaylistName: name} }
}

// Render produces a centered overlay string for the modal. The caller should
// pass the current terminal dimensions so the box is positioned in the middle
// of the screen. When termWidth or termHeight is zero the box is rendered
// without any centering offset.
func (m DestinationPickerModal) Render(termWidth, termHeight int, s styles.Styles) string {
	boxLines := m.buildBoxLines(s)

	// Determine the display width of the box (first line = top border, all
	// ASCII/Unicode single-width characters so byte-len equals display width).
	boxW := len([]rune(boxLines[0]))
	boxH := len(boxLines)

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
	for _, line := range boxLines {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

// buildBoxLines constructs the list of display lines that form the modal box
// and its keybinding hint. Styling is applied to the focused row content only,
// after all width calculations, so it does not affect padding.
func (m DestinationPickerModal) buildBoxLines(s styles.Styles) []string {
	labels := m.destinations()

	// Compute inner width: the number of characters between the │ borders.
	// Row format: │ + " " + cursor(1) + " " + label_padded + " " + │
	// The four non-label characters between the borders: " > " + " " = 4.
	// So innerWidth = 4 + contentWidth, where contentWidth = label display width.
	const minInner = 24
	innerWidth := minInner
	const modalTitle = " Destination "
	if len([]rune(modalTitle))+2 > innerWidth {
		innerWidth = len([]rune(modalTitle)) + 2
	}
	for _, l := range labels {
		if len([]rune(l))+4 > innerWidth {
			innerWidth = len([]rune(l)) + 4
		}
	}
	contentWidth := innerWidth - 4 // characters available for the label inside a row

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, innerWidth)+cornerTR)

	// Title row (centered in innerWidth).
	titlePad := innerWidth - len([]rune(modalTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(modalTitle)+strings.Repeat(" ", rightTP)+borderV)

	// Separator.
	lines = append(lines, crossL+strings.Repeat(borderH, innerWidth)+crossR)

	// Destination rows.
	for i, label := range labels {
		cursorCh := " "
		if i == m.cursor {
			cursorCh = ">"
		}
		// Row content between the borders: " cursor label_padded " = innerWidth chars.
		rowContent := fmt.Sprintf(" %s %-*s ", cursorCh, contentWidth, label)
		if i == m.cursor {
			lines = append(lines, borderV+s.Accent.Render(rowContent)+borderV)
		} else {
			lines = append(lines, borderV+rowContent+borderV)
		}
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, innerWidth)+cornerBR)

	// Keybinding hint outside the box.
	lines = append(lines, "  j/k navigate · enter confirm · esc dismiss")

	return lines
}
