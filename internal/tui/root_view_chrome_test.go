package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestChrome_WindowTitleOnlyWhenHosted(t *testing.T) {
	m := New(context.Background())
	loadChromeTrack(&m, playback.StatePlaying)
	if got := m.View().WindowTitle; got != "" {
		t.Fatalf("unhosted window title = %q, want none", got)
	}

	hosted, _ := newHostedModel(playback.StatePlaying)
	cases := []struct {
		state playback.State
		want  string
	}{
		{playback.StatePlaying, "▶ Song — Band"},
		{playback.StatePaused, "⏸ Song — Band"},
		{playback.StateStopped, "muse"},
		{playback.StateIdle, "muse"},
	}
	for _, tc := range cases {
		hosted.playback.state = tc.state
		if got := hosted.View().WindowTitle; got != tc.want {
			t.Errorf("state %v: window title = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestChrome_TooSmallViewKeepsChrome(t *testing.T) {
	m, _ := newHostedModel(playback.StatePaused)
	m.termWidth, m.termHeight = 10, 3
	if !m.terminalTooSmall() {
		t.Fatal("setup: terminal should be too small")
	}
	if got := m.View().WindowTitle; got != "⏸ Song — Band" {
		t.Fatalf("too-small window title = %q", got)
	}
}

func TestChrome_BackgroundWaitsForDetection(t *testing.T) {
	m := New(context.Background())
	if bg := m.View().BackgroundColor; bg != nil {
		t.Fatalf("background before detection = %v, want nil", bg)
	}
	next, _ := m.Update(tea.BackgroundColorMsg{})
	m = next.(Model)
	if bg := m.View().BackgroundColor; bg == nil || bg != m.styles.Background {
		t.Fatalf("background after detection = %v, want %v", bg, m.styles.Background)
	}
}

func TestChrome_ExplicitPresetKnowsBackground(t *testing.T) {
	for _, preset := range []string{"dark", "light"} {
		m := New(context.Background())
		m.cfg.Theme = preset
		m = m.applyTheme()
		if bg := m.View().BackgroundColor; bg == nil {
			t.Errorf("preset %s: background = nil, want the theme background", preset)
		}
	}
}

func TestChrome_QuittingDropsBackground(t *testing.T) {
	m := New(context.Background())
	m.backgroundDetected = true
	m = m.markQuitting()
	if bg := m.View().BackgroundColor; bg != nil {
		t.Fatalf("background while quitting = %v, want nil", bg)
	}
}

func loadChromeTrack(m *Model, state playback.State) {
	m.playback.state = state
	m.playback.currentHash = "h1"
	m.statusBar.Title, m.statusBar.Artist = "Song", "Band"
}
