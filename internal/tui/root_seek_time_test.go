package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
)

type timestampPlayer struct {
	stubCommandPlayer
	positions []float64
	err       error
}

func (p *timestampPlayer) SeekAbsolute(seconds float64) error {
	p.positions = append(p.positions, seconds)
	return p.err
}

func timestampModel() (Model, *timestampPlayer) {
	p := &timestampPlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.currentHash = "song"
	m.playback.totalSecs = 1000
	m.playback.state = playback.StatePaused
	m, _ = m.openSeekTime()
	return m, p
}

func TestSeekTimeThroughInputPreservesPause(t *testing.T) {
	m, p := timestampModel()
	for _, ch := range "12:30" {
		next, _ := m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if len(p.positions) != 1 || p.positions[0] != 750 || m.overlay.seekTime != nil {
		t.Fatalf("seek positions %v, prompt %v", p.positions, m.overlay.seekTime)
	}
	if m.playback.state != playback.StatePaused || m.playback.elapsedSecs != 750 || m.playback.listenedSecs != 0 {
		t.Fatalf("seek changed playback/listening incorrectly: %+v", m.playback)
	}
}

func TestSeekTimeRefusesInvalidAndStaleTargets(t *testing.T) {
	for _, value := range []string{"junk", "20:00"} {
		m, p := timestampModel()
		m.overlay.seekTime.Input.SetValue(value)
		m, _ = m.confirmSeekTime()
		if len(p.positions) != 0 || m.overlay.seekTime == nil || m.overlay.seekTime.Error == "" {
			t.Fatalf("invalid %q was not retained for correction", value)
		}
	}
	for _, change := range []func(*Model){
		func(m *Model) { m.playback.currentHash = "other" },
		func(m *Model) { m.playback.currentIndex++ },
		func(m *Model) { m.playback.playRequestSeq++ },
	} {
		m, p := timestampModel()
		m.overlay.seekTime.Input.SetValue("30")
		change(&m)
		m, _ = m.confirmSeekTime()
		if len(p.positions) != 0 || m.overlay.seekTime != nil {
			t.Fatal("stale seek target was not dismissed")
		}
	}
}

func TestSeekTimeCancelFailureAndUnknownDuration(t *testing.T) {
	m, p := timestampModel()
	m, _, _ = m.handleSeekTimeKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay.seekTime != nil || len(p.positions) != 0 {
		t.Fatal("cancel sought or left prompt open")
	}
	m, _ = m.openSeekTime()
	m.overlay.seekTime.Input.SetValue("90")
	p.err = errors.New("mpv unavailable")
	m, _ = m.confirmSeekTime()
	if m.overlay.seekTime == nil || m.playback.elapsedSecs != 0 {
		t.Fatal("failed seek should retain prompt and position")
	}
	p.err = nil
	m.playback.totalSecs = 0
	m, _ = m.confirmSeekTime()
	if m.playback.elapsedSecs != 90 || m.overlay.seekTime != nil {
		t.Fatal("unknown duration should allow seek")
	}
}

func TestSeekTimePaletteAvailability(t *testing.T) {
	m := New(context.Background())
	if polishCommand(t, m, "playback.seek.time").IsEnabled() {
		t.Fatal("seek enabled without loaded track")
	}
	m, _ = m.openSeekTime()
	if m.overlay.seekTime != nil {
		t.Fatal("prompt opened without track")
	}
}

func TestSeekTimeTimingUpdatesWhileOpen(t *testing.T) {
	m, _ := timestampModel()
	m.termWidth, m.termHeight = 60, 15
	m.playback.elapsedSecs, m.playback.totalSecs = 222, 728
	out := ansi.Strip(m.renderActiveBody())
	if !strings.Contains(out, "Current: 3:42 · Duration: 12:08") {
		t.Fatalf("timing missing: %s", out)
	}
	m.playback.elapsedSecs, m.playback.totalSecs = 224, 730
	out = ansi.Strip(m.renderActiveBody())
	if !strings.Contains(out, "Current: 3:44 · Duration: 12:10") {
		t.Fatalf("timing stale: %s", out)
	}
}
