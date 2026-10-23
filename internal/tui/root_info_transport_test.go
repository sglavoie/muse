package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestInfoOverlaysPassTransportAndKeepTheirContext(t *testing.T) {
	cases := map[string]func(*Model){
		"now playing": func(m *Model) { m.overlay.nowPlaying = &overlay.NowPlayingOverlay{} },
		"track info":  func(m *Model) { m.overlay.trackInfo = &overlay.TrackInfoOverlay{} },
		"summary":     func(m *Model) { m.overlay.queueSummary = &overlay.QueueSummaryOverlay{} },
		"modes":       func(m *Model) { m.overlay.modesLegend = &overlay.ModesLegendOverlay{} },
		"messages":    func(m *Model) { m.overlay.messages = overlay.NewMessages(nil) },
		"help":        func(m *Model) { m.helpVisible = true },
	}
	for name, open := range cases {
		t.Run(name, func(t *testing.T) {
			p := &stubSeekPlayer{}
			m := newRichFixture()
			m.player = p
			m.playback.state = playback.StatePlaying
			open(&m)
			n, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
			m = browsingTestDrain(t, n.(Model), cmd)
			if p.pauseCalls != 1 || !m.hasActiveOverlay() {
				t.Fatal("pause lost or overlay dismissed")
			}
			n, cmd = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
			m = browsingTestDrain(t, n.(Model), cmd)
			if p.nextCalls != 1 || !m.hasActiveOverlay() {
				t.Fatal("next lost or overlay dismissed")
			}
			n, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			m = browsingTestDrain(t, n.(Model), cmd)
			if len(p.seekDeltas) != 1 {
				t.Fatal("seek lost")
			}
		})
	}
}

func TestHelpSearchKeepsTransportCharactersAsText(t *testing.T) {
	p := &stubSeekPlayer{}
	m := newRichFixture()
	m.player = p
	m.playback.state = playback.StatePlaying
	m.helpVisible = true
	m.helpSearchFilter, _ = m.helpSearchFilter.Activate()
	for _, key := range []tea.KeyPressMsg{{Code: 'n', Text: "n"}, {Code: tea.KeySpace, Text: " "}, {Code: 'p', Text: "p"}} {
		next, _ := m.Update(key)
		m = next.(Model)
	}
	if m.helpSearchFilter.Text != "n p" || p.pauseCalls != 0 || p.nextCalls != 0 || p.prevCalls != 0 {
		t.Fatalf("query %q; pause=%d next=%d previous=%d", m.helpSearchFilter.Text, p.pauseCalls, p.nextCalls, p.prevCalls)
	}
}
