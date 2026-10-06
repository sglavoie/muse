package playlists

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

const (
	queueLabel  = "Play Queue"
	createLabel = "New playlist…"

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
// "New playlist…" entry.
type DestinationPickerCreateMsg struct{}

// DestinationPickerModal is a focused overlay sub-model for choosing where to
// append tracks. It lists the play queue as the first entry followed by all
// saved playlists in alphabetical order and a "New playlist…" action.
// Searching filters playlist names while keeping both actions available.
type DestinationPickerModal struct {
	playlists  []db.PlaylistRow
	cursor     int
	height     int
	search     filter.State
	trackCount int
	duration   string
}

// WithTrackSummary describes the captured tracks the picker will add.
func (m DestinationPickerModal) WithTrackSummary(count int, duration string) DestinationPickerModal {
	m.trackCount, m.duration = count, duration
	return m
}

// NewDestinationPicker returns a DestinationPickerModal populated with the
// given playlists. The slice should be pre-sorted alphabetically, as returned
// by db.ReadPlaylists.
func NewDestinationPicker(playlists []db.PlaylistRow) DestinationPickerModal {
	return DestinationPickerModal{playlists: playlists}
}

// destinations returns the ordered list of destination labels: the play queue
// first, then each playlist name, and finally a "New playlist…" entry so the
// user can always create a new playlist on the fly.
func (m DestinationPickerModal) destinations() []string {
	labels := make([]string, 0, 1+len(m.playlists)+1)
	labels = append(labels, queueLabel)
	for _, i := range m.matchingPlaylists() {
		labels = append(labels, m.playlists[i].Name)
	}
	labels = append(labels, createLabel)
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
	m.cursor = clamp(m.cursor, 0, n-1)
	m = m.navigate(key, n)
	switch key {
	case "/":
		var cmd tea.Cmd
		m.search, cmd = m.search.Activate()
		return m, cmd
	case "enter":
		return m, m.confirmCmd()
	case "esc":
		if m.search.Active || m.search.Text != "" {
			return m.clearSearch(), nil
		}
		return m, func() tea.Msg { return DestinationPickerDismissedMsg{} }
	}
	return m, nil
}

func (m DestinationPickerModal) navigate(key string, n int) DestinationPickerModal {
	switch key {
	case "j", "down":
		if m.cursor < n-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "pgdown", "ctrl+d":
		m.cursor = min(n-1, m.cursor+m.visibleRows())
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-m.visibleRows())
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = n - 1
	}
	return m
}

// confirmCmd builds the Cmd that confirms the current selection.
func (m DestinationPickerModal) confirmCmd() tea.Cmd {
	matches := m.matchingPlaylists()
	cursor := clamp(m.cursor, 0, len(matches)+1)
	// Last entry is always "New playlist…".
	if cursor == len(matches)+1 {
		return func() tea.Msg { return DestinationPickerCreateMsg{} }
	}
	if cursor == 0 {
		return func() tea.Msg { return DestinationPickedMsg{IsQueue: true} }
	}
	name := m.playlists[matches[cursor-1]].Name
	return func() tea.Msg { return DestinationPickedMsg{PlaylistName: name} }
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Render produces a centered overlay string for the modal. The caller should
// pass the current terminal dimensions so the box is positioned in the middle
// of the screen. When termWidth or termHeight is zero the box is rendered
// without any centering offset.
func (m DestinationPickerModal) Render(termWidth, termHeight int, s styles.Styles) string {
	m.height = termHeight
	boxLines := m.buildBoxLines(termWidth, s)

	// Determine the display width of the box (first line = top border, all
	// ASCII/Unicode single-width characters so byte-len equals display width).
	boxW := ansi.StringWidth(boxLines[0])
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
	for i, line := range boxLines {
		pad := prefix
		if i == len(boxLines)-1 && termWidth > 0 {
			pad = strings.Repeat(" ", max(0, (termWidth-ansi.StringWidth(line))/2))
		}
		b.WriteString(pad + line + "\n")
	}
	return b.String()
}

// buildBoxLines constructs the list of display lines that form the modal box
// and its keybinding hint. Styling is applied to the focused row content only,
// after all width calculations, so it does not affect padding.
func (m DestinationPickerModal) buildBoxLines(termWidth int, s styles.Styles) []string {
	labels := m.destinations()
	focusCursor := clamp(m.cursor, 0, len(labels)-1)

	modalTitle := " Destination "
	if len(labels) > m.visibleRows() {
		modalTitle = fmt.Sprintf(" Destination %d/%d ", focusCursor+1, len(labels))
	}
	if m.trackCount > 0 {
		modalTitle = fmt.Sprintf(" Add %d %s · %s ", m.trackCount, text.Pluralize(m.trackCount, "track", "tracks"), m.duration)
		if len(labels) > m.visibleRows() {
			modalTitle += fmt.Sprintf("· %d/%d ", focusCursor+1, len(labels))
		}
	}
	innerWidth := m.innerWidth(termWidth, labels, modalTitle)
	modalTitle = text.Truncate(modalTitle, innerWidth)
	contentWidth := innerWidth - 2 // characters available for the label inside a row
	contentWidth = max(contentWidth, 0)

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, innerWidth)+cornerTR)

	// Title row (centered in innerWidth).
	titlePad := max(0, innerWidth-ansi.StringWidth(modalTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(modalTitle)+strings.Repeat(" ", rightTP)+borderV)

	// Separator.
	lines = append(lines, crossL+strings.Repeat(borderH, innerWidth)+crossR)

	if m.search.Active || m.search.Text != "" {
		lines = append(lines, borderV+" "+text.Pad(m.searchLabel(), contentWidth)+" "+borderV)
	}

	// Keep the focused row visible, including after a terminal resize.
	rows := min(len(labels), m.visibleRows())
	start := clamp(focusCursor-rows/2, 0, len(labels)-rows)
	for i := start; i < start+rows; i++ {
		label := labels[i]
		// Truncate and pad using display-width-aware helpers so that
		// multi-byte characters (CJK, emoji) are handled correctly.
		display := text.Pad(label, contentWidth)
		// Row content between the borders: " label_padded " = innerWidth chars.
		rowContent := " " + display + " "
		if i == focusCursor {
			lines = append(lines, borderV+s.Accent.Render(rowContent)+borderV)
		} else {
			lines = append(lines, borderV+rowContent+borderV)
		}
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, innerWidth)+cornerBR)

	lines = append(lines, m.keyHint(termWidth))

	return lines
}

func (m DestinationPickerModal) innerWidth(termWidth int, labels []string, modalTitle string) int {
	// Compute inner width: the number of characters between the │ borders.
	// Row format: │ + " " + label_padded + " " + │
	// The two non-label characters between the borders: " " + " " = 2.
	// So innerWidth = 2 + contentWidth, where contentWidth = label display width.
	// Mirrors tui.OverlayMinInnerWidth (cannot import due to circular dep).
	const minInner = 24
	innerWidth := minInner
	if ansi.StringWidth(modalTitle)+2 > innerWidth {
		innerWidth = ansi.StringWidth(modalTitle) + 2
	}
	for _, l := range labels {
		if ansi.StringWidth(l)+2 > innerWidth {
			innerWidth = ansi.StringWidth(l) + 2
		}
	}
	if m.search.Active || m.search.Text != "" {
		innerWidth = max(innerWidth, ansi.StringWidth(m.searchLabel())+2)
	}
	if maxInner := termWidth - 4; termWidth > 0 && innerWidth > maxInner {
		innerWidth = maxInner
	}
	if innerWidth < 1 {
		innerWidth = 1
	}
	return innerWidth
}

func (m DestinationPickerModal) keyHint(termWidth int) string {
	// Keybinding hint outside the box.
	hint := "  / search · j/k · PgUp/Dn · enter choose"
	switch {
	case m.search.Active:
		hint = "  enter apply · esc clear"
	case m.search.Text != "":
		hint = "  / edit · esc clear · j/k move · enter choose"
	default:
		hint += " · esc close"
	}
	if termWidth > 0 {
		hint = text.Truncate(hint, max(0, termWidth-2))
	}
	return hint
}
