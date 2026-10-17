package overlay

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// GoToKind identifies what type of navigation a GoToOption represents.
type GoToKind int

const (
	GoToArtist GoToKind = iota
	GoToAlbum
	GoToGenre
	GoToPlaylist
)

// shortcutKey returns the single-character shortcut for the kind.
func (k GoToKind) shortcutKey() string {
	switch k {
	case GoToArtist:
		return "a"
	case GoToAlbum:
		return "b"
	case GoToGenre:
		return "g"
	case GoToPlaylist:
		return "p"
	}
	return ""
}

// label returns the display prefix for the kind.
func (k GoToKind) label() string {
	switch k {
	case GoToArtist:
		return "Artist"
	case GoToAlbum:
		return "Album"
	case GoToGenre:
		return "Genre"
	case GoToPlaylist:
		return "Source"
	}
	return ""
}

// GoToOption is a single navigation target in the Go To popup.
type GoToOption struct {
	Kind  GoToKind
	Value string // raw value passed to the jump function
}

// displayLabel returns the formatted label shown in the popup row.
// Format: "a  Artist: Bob"
func (o GoToOption) displayLabel() string {
	value := o.Value
	if o.Kind == GoToAlbum {
		value = library.AlbumLabel(value)
	}
	return o.Kind.shortcutKey() + "  " + o.Kind.label() + ": " + value
}

// GoToPickedMsg is dispatched when the user confirms a Go To selection.
type GoToPickedMsg struct {
	Kind  GoToKind
	Value string
}

// GoToDismissedMsg is dispatched when the user dismisses the Go To popup.
type GoToDismissedMsg struct{}

// GoToModal is the overlay sub-model for the "Go To" navigation popup.
type GoToModal struct {
	options []GoToOption
	cursor  int
}

// NewGoToModal creates a GoToModal with the given options.
func NewGoToModal(options []GoToOption) GoToModal {
	return GoToModal{options: options}
}

// HandleKey processes a key press and returns the updated modal and optional Cmd.
//
//   - j / down: move cursor down
//   - k / up:   move cursor up
//   - enter:    confirm focused option
//   - esc:      dismiss
//   - a/b/g/p:  shortcut to confirm matching option
func (m GoToModal) HandleKey(key string) (GoToModal, tea.Cmd) {
	switch key {
	case "j", "down":
		return m.nextOption(), nil
	case "k", "up":
		return m.prevOption(), nil
	case "enter":
		return m, m.confirmFocused()
	case "esc":
		return m, func() tea.Msg { return GoToDismissedMsg{} }
	case "a", "b", "g", "p":
		if opt, ok := m.findByShortcut(key); ok {
			return m, m.confirmOption(opt)
		}
	}
	return m, nil
}

// nextOption moves the cursor one option down, stopping at the last option.
func (m GoToModal) nextOption() GoToModal {
	if m.cursor < len(m.options)-1 {
		m.cursor++
	}
	return m
}

// prevOption moves the cursor one option up, stopping at the first option.
func (m GoToModal) prevOption() GoToModal {
	if m.cursor > 0 {
		m.cursor--
	}
	return m
}

// confirmFocused returns the command confirming the focused option, or nil when
// the cursor is out of range.
func (m GoToModal) confirmFocused() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.options) {
		return nil
	}
	return m.confirmOption(m.options[m.cursor])
}

// findByShortcut returns the first option matching the shortcut key.
// If the cursor is on an option with that shortcut, it is preferred.
func (m GoToModal) findByShortcut(key string) (GoToOption, bool) {
	// Prefer the focused option if it matches the shortcut.
	if m.cursor >= 0 && m.cursor < len(m.options) {
		if m.options[m.cursor].Kind.shortcutKey() == key {
			return m.options[m.cursor], true
		}
	}
	// Otherwise, return the first match.
	for _, o := range m.options {
		if o.Kind.shortcutKey() == key {
			return o, true
		}
	}
	return GoToOption{}, false
}

func (m GoToModal) confirmOption(opt GoToOption) tea.Cmd {
	return func() tea.Msg {
		return GoToPickedMsg(opt)
	}
}

// Border characters (duplicated from playlists/modal.go to avoid circular dep).
const (
	gtBorderH  = "─"
	gtBorderV  = "│"
	gtCornerTL = "╭"
	gtCornerTR = "╮"
	gtCornerBL = "╰"
	gtCornerBR = "╯"
	gtCrossL   = "├"
	gtCrossR   = "┤"
)

// Render produces a centered overlay string for the modal.
func (m GoToModal) Render(termWidth, termHeight int, s styles.Styles) string {
	boxLines := m.buildBoxLines(termWidth, s)
	return RenderCenteredLines(boxLines, "j/k navigate · a/b/g/p shortcut · enter confirm · esc dismiss", termWidth, termHeight)
}

func (m GoToModal) buildBoxLines(termWidth int, s styles.Styles) []string {
	labels := make([]string, len(m.options))
	for i, o := range m.options {
		labels[i] = o.displayLabel()
	}

	const minInner = 24
	innerWidth := minInner
	const modalTitle = " Go To "
	if ansi.StringWidth(modalTitle)+2 > innerWidth {
		innerWidth = ansi.StringWidth(modalTitle) + 2
	}
	for _, l := range labels {
		if ansi.StringWidth(l)+2 > innerWidth {
			innerWidth = ansi.StringWidth(l) + 2
		}
	}
	if maxInner := termWidth - 4; termWidth > 0 && innerWidth > maxInner {
		innerWidth = maxInner
	}
	if innerWidth < 1 {
		innerWidth = 1
	}
	contentWidth := max(innerWidth-2, 0)

	var lines []string

	// Top border.
	lines = append(lines, gtCornerTL+strings.Repeat(gtBorderH, innerWidth)+gtCornerTR)

	// Title row (centered).
	titlePad := max(0, innerWidth-ansi.StringWidth(modalTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, gtBorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(modalTitle)+strings.Repeat(" ", rightTP)+gtBorderV)

	// Separator.
	lines = append(lines, gtCrossL+strings.Repeat(gtBorderH, innerWidth)+gtCrossR)

	// Option rows.
	for i, label := range labels {
		display := text.FocusMarker(i == m.cursor) + text.Pad(label, contentWidth-2)
		rowContent := " " + display + " "
		if i == m.cursor {
			lines = append(lines, gtBorderV+s.Accent.Render(rowContent)+gtBorderV)
		} else {
			lines = append(lines, gtBorderV+rowContent+gtBorderV)
		}
	}

	// Bottom border.
	lines = append(lines, gtCornerBL+strings.Repeat(gtBorderH, innerWidth)+gtCornerBR)

	return lines
}
