package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/text"
)

const welcomeKeyColWidth = 20

func welcomeRow(key, action string) OverlayRow {
	return OverlayRow{Text: text.Pad(key, welcomeKeyColWidth) + action}
}

// welcomeVisible reports whether the first-run welcome overlay should be shown.
// It fires either because the config file was just created, or because no
// usable music directory was found — the latter can happen to a user who has
// had a config.toml for months and then moves or unmounts their music, which is
// exactly the case where the overlay's instructions are most useful.
func (m Model) welcomeVisible() bool {
	return (m.configCreated || m.musicDirHint != "") && !m.welcomeDismissed
}

// handleWelcomeKeyPress consumes any key press while the welcome overlay is
// shown, dismissing it so the user proceeds to the library view.
func (m Model) handleWelcomeKeyPress(_ tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.welcomeVisible() {
		return m, nil, false
	}
	m.welcomeDismissed = true
	return m, nil, true
}

func (m Model) renderWelcomeOverlay() string {
	rows := []OverlayRow{{Blank: true}}
	maxInner := 0
	if m.musicDirHint != "" {
		rows = append(rows, OverlayRow{Text: "No music directories found.", Styled: true})
		rows = append(rows, hintRows(m.musicDirHint)...)
		rows = append(rows, OverlayRow{Blank: true})
		// A config path can be long enough to push the box past the terminal;
		// clamp so the borders stay inside the screen and the path truncates.
		if m.termWidth > OverlayHPadding {
			maxInner = m.termWidth - OverlayHPadding
		}
	}
	rows = append(rows,
		welcomeRow("j / k  or  ↑ / ↓", "Navigate"),
		welcomeRow("Enter", "Add to queue"),
		welcomeRow("Space", "Play / Pause"),
		welcomeRow("/", "Filter tracks"),
		welcomeRow("Tab", "Switch views"),
		welcomeRow("?", "Show all keybindings"),
		OverlayRow{Blank: true},
	)
	o := OverlayBox{
		Title:         " Welcome to Muse ",
		Rows:          rows,
		Hint:          "  Press any key to start  ",
		MaxInnerWidth: maxInner,
	}
	return o.Render(m.termWidth, m.termHeight, *m.styles)
}

// hintRows turns config.ConfigHint's multi-line instruction block into overlay
// rows. The text is reused verbatim (leading blank line aside) so the overlay
// and the validation error can never disagree about which key to edit.
func hintRows(hint string) []OverlayRow {
	var rows []OverlayRow
	for _, line := range strings.Split(hint, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, OverlayRow{Text: line})
	}
	return rows
}
