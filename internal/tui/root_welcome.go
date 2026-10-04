package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// welcomeVisible reports whether the first-run welcome overlay should be shown.
// It fires either because the config file was just created, or because no
// usable music directory was found — the latter can happen to a user who has
// had a config.toml for months and then moves or unmounts their music, which is
// exactly the case where the overlay's instructions are most useful.
func (m Model) welcomeVisible() bool {
	return (m.configCreated || m.musicDirHint != "") && !m.welcomeDismissed
}

// handleWelcomeKeyPress consumes any key press while the welcome overlay is
// shown, dismissing it or honoring the global quit shortcut.
func (m Model) handleWelcomeKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.welcomeVisible() {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m.markQuitting(), tea.Quit, true
	}
	m.welcomeDismissed = true
	return m, nil, true
}

func (m Model) renderWelcomeOverlay() string {
	rows := []overlay.Row{{Blank: true}}
	maxInner := 0
	if m.musicDirHint != "" {
		rows = append(rows, overlay.Row{Text: overlay.MusicDirHeadline(m.musicDirsPartial), Styled: true})
		if m.musicDirsPartial {
			rows = append(rows, overlay.Row{Text: "The remaining directories are being scanned."})
		}
		rows = append(rows, overlay.HintRows(m.musicDirHint)...)
		rows = append(rows, overlay.Row{Blank: true})
		// A config path can be long enough to push the box past the terminal;
		// clamp so the borders stay inside the screen and the path truncates.
		if m.termWidth > overlay.HPadding {
			maxInner = m.termWidth - overlay.HPadding
		}
	}
	rows = append(rows,
		overlay.WelcomeRow("j / k  or  ↑ / ↓", "Navigate"),
		overlay.WelcomeRow("Enter", "Add to queue"),
		overlay.WelcomeRow("Space", "Play / Pause"),
		overlay.WelcomeRow("/", "Filter tracks"),
		overlay.WelcomeRow("Tab", "Switch views"),
		overlay.WelcomeRow("?", "Show all keybindings"),
		overlay.Row{Blank: true},
	)
	o := overlay.Box{
		Title:         " Welcome to Muse ",
		Rows:          rows,
		Hint:          "  Press any key to start  ",
		MaxInnerWidth: maxInner,
	}
	return o.Render(m.termWidth, m.termHeight, *m.styles)
}
