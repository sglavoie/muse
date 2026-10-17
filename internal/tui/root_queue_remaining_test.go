package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

type remainingPlayer struct {
	*fakeTraversalPlayer
	snapshot player.QueueTraversal
}

func (p *remainingPlayer) QueueTraversal() player.QueueTraversal { return p.snapshot }

func TestQueueRemainingUsesScheduledRows(t *testing.T) {
	p := &remainingPlayer{fakeTraversalPlayer: &fakeTraversalPlayer{}, snapshot: player.QueueTraversal{
		Order: []int{2, 0, 3, 1}, CurrentPosition: 1, Remaining: 1, RemainingIndices: []int{3}, Shuffled: true,
	}}
	m := New(context.Background(), WithPlayer(p))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{{DurationMs: 60000}, {DurationMs: 900000}, {DurationMs: 700000}, {DurationMs: 120000}}
	m.playback.currentIndex = 0
	m.playback.state = playback.StatePlaying
	m.playback.totalSecs = 60
	m.playback.elapsedSecs = 20
	ms, n, unknown := m.queueRemaining()
	if ms != 160000 || n != 2 || unknown {
		t.Fatalf("remaining = %d,%d,%v", ms, n, unknown)
	}
	summary := m.buildQueueSummary()
	if summary.RemainMs != ms || summary.RemainN != n {
		t.Fatal("summary and header disagree")
	}
	m.queue.Width = 100
	m.queue.Height = 20
	screen := ansi.Strip(m.renderPrimaryViewBody(*m.styles, ""))
	if !strings.Contains(screen, "4 tracks · 2:40 remaining") {
		t.Fatalf("missing header duration: %s", screen)
	}
	m.playback.state = playback.StatePaused
	paused, _, _ := m.queueRemaining()
	if paused != ms {
		t.Fatal("pause changed remaining time")
	}
	m.queue.Filter.Text = "no matches"
	filtered, _, _ := m.queueRemaining()
	if filtered != ms {
		t.Fatal("filter changed playback duration")
	}
}

func TestQueueRemainingStartupUnknownAndExhausted(t *testing.T) {
	m := New(context.Background())
	m.queue.Entries = []queue.Entry{{DurationMs: 60000}, {DurationMs: 0}, {DurationMs: 30000, Missing: true}}
	m.playback.currentIndex = -1
	ms, n, unknown := m.queueRemaining()
	if ms != 60000 || n != 2 || !unknown {
		t.Fatalf("startup = %d,%d,%v", ms, n, unknown)
	}
	if got := m.queueRemainingLabel(); got != "1:00+ queued" {
		t.Fatalf("label = %q", got)
	}
	m.playback.queueExhausted = true
	ms, n, _ = m.queueRemaining()
	if ms != 0 || n != 0 {
		t.Fatalf("exhausted = %d,%d", ms, n)
	}
}
