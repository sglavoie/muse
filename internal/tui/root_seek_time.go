package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/text"
)

type openSeekTimeMsg struct{}

type seekTimeState struct {
	overlay.SeekTime
	hash    string
	index   int
	request uint64
}

func (m Model) seekTimeCommand() palette.PaletteCommand {
	cmd := paletteMessageCmd("playback.seek.time", "play", "Seek to time…", "", openSeekTimeMsg{})
	cmd.Enabled = func() bool { return playbackHasTrack(m) }
	cmd.DisabledReason = "Load a track first"
	if playbackHasTrack(m) {
		cmd.Detail = "Seek within: " + text.TrackDisplay(m.statusBar.Artist, m.statusBar.Title, "")
	}
	return cmd
}

func (m Model) openSeekTime() (Model, tea.Cmd) {
	if !playbackHasTrack(m) {
		return m, nil
	}
	prompt := overlay.NewSeekTime(text.TrackDisplay(m.statusBar.Artist, m.statusBar.Title, ""))
	cmd := prompt.Input.Focus()
	m.overlay.seekTime = &seekTimeState{SeekTime: prompt, hash: m.playback.currentHash,
		index: m.playback.currentIndex, request: m.playback.playRequestSeq}
	return m, cmd
}

func (m Model) handleSeekTimeKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.seekTime == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m.markQuitting(), tea.Quit, true
	case "esc":
		m.overlay.seekTime = nil
		return m, nil, true
	case "enter":
		m, cmd := m.confirmSeekTime()
		return m, cmd, true
	default:
		return m.forwardSeekTimeMsg(msg)
	}
}

func (m Model) forwardSeekTimeMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.overlay.seekTime == nil {
		return m, nil, false
	}
	prompt := *m.overlay.seekTime
	var cmd tea.Cmd
	prompt.Input, cmd = prompt.Input.Update(msg)
	m.overlay.seekTime = &prompt
	return m, cmd, true
}

func (m Model) confirmSeekTime() (Model, tea.Cmd) {
	prompt := *m.overlay.seekTime
	if prompt.hash != m.playback.currentHash || prompt.index != m.playback.currentIndex || prompt.request != m.playback.playRequestSeq {
		m.overlay.seekTime = nil
		m.statusBar.Notify("Track changed — open Seek to time again")
		return m, nil
	}
	seconds, err := overlay.ParseSeekTime(prompt.Input.Value())
	if err == nil && m.playback.totalSecs > 0 && seconds > m.playback.totalSecs {
		err = fmt.Errorf("time exceeds track duration (%s)", text.FormatPositionSecs(m.playback.totalSecs))
	}
	if err != nil {
		prompt.Error = err.Error()
		m.overlay.seekTime = &prompt
		return m, nil
	}
	return m.seekToTime(seconds)
}

func (m Model) seekToTime(seconds float64) (Model, tea.Cmd) {
	cp, ok := m.player.(commandPlayer)
	if !ok {
		m.statusBar.NotifyError("Seek failed: player unavailable")
		return m, nil
	}
	if err := cp.SeekAbsolute(seconds); err != nil {
		m.statusBar.NotifyError("Seek failed: " + err.Error())
		return m, nil
	}
	m.overlay.seekTime = nil
	m.playback.elapsedSecs = seconds
	m.playback.lastSampleSecs = seconds
	m.statusBar.Notify("Seeked to " + text.FormatPositionSecs(seconds))
	return m, m.sessionPositionCmd()
}
