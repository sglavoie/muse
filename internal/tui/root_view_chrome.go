package tui

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// hostedIdleTitle is the window title a hosted session shows with no track
// playing or paused.
const hostedIdleTitle = "muse"

// finishView applies the window chrome to a rendered frame: the window title
// when hosted, and the terminal background (OSC 11) once muse knows which
// background it is drawing on.
func (m Model) finishView(v tea.View) tea.View {
	if m.hosted() {
		v.WindowTitle = m.windowTitle()
	}
	v.BackgroundColor = m.chromeBackground()
	return v
}

// markQuitting records that an action asked Bubble Tea to quit, so the last
// frame stops setting the terminal background and Bubble Tea restores it.
func (m Model) markQuitting() Model {
	m.quitting = true
	return m
}

// windowTitle is "▶ Title — Artist" while playing, "⏸ Title — Artist" while
// paused, and "muse" otherwise.
func (m Model) windowTitle() string {
	var icon string
	switch m.playback.state {
	case playback.StatePlaying:
		icon = "▶ "
	case playback.StatePaused:
		icon = "⏸ "
	default:
		return hostedIdleTitle
	}
	title := m.statusBar.Title
	if title == "" {
		return hostedIdleTitle
	}
	if m.statusBar.Artist != "" {
		title += " — " + m.statusBar.Artist
	}
	return icon + title
}

// chromeBackground is the color muse paints the terminal background with, or
// nil to leave it alone. It stays nil until the terminal has reported its own
// background or an explicit dark/light preset makes that report irrelevant:
// setting it earlier would let auto-theme detection read muse's own color back.
// It is nil again while quitting, so Bubble Tea restores the terminal's color.
func (m Model) chromeBackground() color.Color {
	if m.quitting || !m.backgroundKnown() {
		return nil
	}
	return m.styles.Background
}

// backgroundKnown reports whether the effective theme no longer depends on the
// terminal's background answer.
func (m Model) backgroundKnown() bool {
	if m.backgroundDetected {
		return true
	}
	switch preset, _ := m.themeState(); preset {
	case "dark", "light":
		return true
	}
	return false
}
