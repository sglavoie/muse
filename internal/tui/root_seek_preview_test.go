package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// seekPreviewPlayer records absolute and relative seeks and reports a
// position, so the preview's commit and settling can be observed.
type seekPreviewPlayer struct {
	stubSeekPlayer
	absolute []float64
	elapsed  float64
	total    float64
}

func (p *seekPreviewPlayer) SeekAbsolute(s float64) error {
	p.absolute = append(p.absolute, s)
	return nil
}

func (p *seekPreviewPlayer) ElapsedPosition() (float64, float64, error) {
	return p.elapsed, p.total, nil
}

// fakeClock is a settable clock for the seek preview.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// newSeekPreviewModel builds a model playing a 4-minute song at 1:00.
func newSeekPreviewModel(t *testing.T) (Model, *seekPreviewPlayer, *fakeClock) {
	t.Helper()
	p := &seekPreviewPlayer{elapsed: 60, total: 240}
	m := New(context.Background(), WithPlayer(p), WithLibraryTracks(sampleLibraryTracks()))
	m.termWidth, m.termHeight = 100, 30
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "hash-a"
	m.playback.elapsedSecs, m.playback.totalSecs = 60, 240
	m.playback.lastSampleSecs = 60
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	m.seek.clock = clock.now
	return m, p, clock
}

func pressArrow(t *testing.T, m Model, code rune) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return next.(Model), cmd
}

func TestSeekKeysPreviewTargetWithoutSeeking(t *testing.T) {
	m, p, clock := newSeekPreviewModel(t)
	for range 3 {
		m, _ = pressArrow(t, m, tea.KeyRight)
		clock.advance(100 * time.Millisecond)
	}
	if len(p.absolute) != 0 || len(p.seekDeltas) != 0 {
		t.Fatalf("seek keys sent seeks before going quiet: abs=%v rel=%v", p.absolute, p.seekDeltas)
	}
	if got := m.displayElapsed(time.Now()); got != 75 {
		t.Fatalf("displayed position = %v, want the 1:15 target", got)
	}
	bar := ansi.Strip(m.RenderStatusBar())
	if !strings.Contains(bar, "→ 1:15 / 4:00") {
		t.Fatalf("status bar does not preview the target:\n%s", bar)
	}
}

func TestSeekPreviewCommitsOnceAfterLastPress(t *testing.T) {
	m, p, clock := newSeekPreviewModel(t)
	m, _ = pressArrow(t, m, tea.KeyRight)
	first := m.seek.seq
	clock.advance(100 * time.Millisecond)
	m, _ = pressArrow(t, m, tea.KeyLeft)
	m, _ = pressArrow(t, m, tea.KeyLeft)

	// The first press's tick is stale and must not seek.
	next, _ := m.Update(seekCommitMsg{seq: first})
	m = next.(Model)
	if len(p.absolute) != 0 {
		t.Fatalf("stale commit tick seeked: %v", p.absolute)
	}
	next, _ = m.Update(seekCommitMsg{seq: m.seek.seq})
	m = next.(Model)
	if len(p.absolute) != 1 || p.absolute[0] != 55 {
		t.Fatalf("absolute seeks = %v, want one seek to 55", p.absolute)
	}
	if m.seek.pending || m.playback.elapsedSecs != 55 || m.playback.lastSampleSecs != 55 {
		t.Fatalf("after commit: pending=%v elapsed=%v last=%v", m.seek.pending, m.playback.elapsedSecs, m.playback.lastSampleSecs)
	}
}

func TestSeekPreviewClampsToTrack(t *testing.T) {
	m, _, _ := newSeekPreviewModel(t)
	for range 20 {
		m, _ = pressArrow(t, m, tea.KeyLeft)
	}
	if m.seek.target != 0 {
		t.Fatalf("target = %v, want clamped to 0", m.seek.target)
	}
	if arrow := m.seekPreviewArrow(); arrow != "←" {
		t.Fatalf("arrow = %q, want ←", arrow)
	}
}

func TestSeekPreviewAcceleratesWhileHeld(t *testing.T) {
	m, _, clock := newSeekPreviewModel(t)
	var steps []float64
	prev := 60.0
	// Key repeat every 50ms for three seconds.
	for range 61 {
		m, _ = pressArrow(t, m, tea.KeyRight)
		steps = append(steps, m.seek.target-prev)
		prev = m.seek.target
		clock.advance(50 * time.Millisecond)
		if m.seek.target >= 240 {
			break
		}
	}
	if steps[0] != 5 {
		t.Fatalf("first step = %v, want 5", steps[0])
	}
	if steps[25] != 10 {
		t.Fatalf("step after 1.25s = %v, want 10", steps[25])
	}
	m2, _, clock2 := newSeekPreviewModel(t)
	m2.playback.totalSecs = 3600
	for range 60 {
		m2, _ = pressArrow(t, m2, tea.KeyRight)
		clock2.advance(50 * time.Millisecond)
	}
	before := m2.seek.target
	m2, _ = pressArrow(t, m2, tea.KeyRight)
	if step := m2.seek.target - before; step != 30 {
		t.Fatalf("step after 3s = %v, want 30", step)
	}
}

func TestSeekPreviewStreakResetsAfterPause(t *testing.T) {
	m, _, clock := newSeekPreviewModel(t)
	m.playback.totalSecs = 3600
	for range 40 {
		m, _ = pressArrow(t, m, tea.KeyRight)
		clock.advance(50 * time.Millisecond)
	}
	clock.advance(time.Second)
	before := m.seek.target
	m, _ = pressArrow(t, m, tea.KeyRight)
	if step := m.seek.target - before; step != 5 {
		t.Fatalf("step after a pause = %v, want back to 5", step)
	}
}

func TestSeekPreviewCommitsBeforeAnotherKey(t *testing.T) {
	m, p, _ := newSeekPreviewModel(t)
	m, _ = pressArrow(t, m, tea.KeyRight)
	m = pressKey(m, "j")
	if len(p.absolute) != 1 || p.absolute[0] != 65 {
		t.Fatalf("another key did not commit the pending seek: %v", p.absolute)
	}
	if m.seek.pending {
		t.Fatal("seek still pending after commit")
	}
}

func TestSeekPreviewDroppedOnTrackChange(t *testing.T) {
	m, p, _ := newSeekPreviewModel(t)
	m, _ = pressArrow(t, m, tea.KeyRight)
	m.playback.currentHash = "hash-b"
	if got := m.displayElapsed(time.Now()); got != 60 {
		t.Fatalf("a target for the old song is still shown: %v", got)
	}
	next, _ := m.Update(seekCommitMsg{seq: m.seek.seq})
	m = next.(Model)
	if len(p.absolute) != 0 {
		t.Fatalf("pending seek was applied to the next song: %v", p.absolute)
	}
	if m.seek.pending {
		t.Fatal("pending seek not discarded")
	}
}

func TestSeekPreviewIgnoresStaleReadingsWhileSettling(t *testing.T) {
	m, p, clock := newSeekPreviewModel(t)
	m.motion.enabled = true
	m, _ = pressArrow(t, m, tea.KeyRight)
	m, _ = pressArrow(t, m, tea.KeyRight)
	next, _ := m.Update(seekCommitMsg{seq: m.seek.seq})
	m = next.(Model)

	// mpv still reports the old position: the bar must not snap back.
	p.elapsed = 61
	m, _ = m.handleTickMsg()
	if m.playback.elapsedSecs != 70 {
		t.Fatalf("stale reading moved the position to %v", m.playback.elapsedSecs)
	}
	// The new position arrives and is taken as usual.
	p.elapsed = 71
	m, _ = m.handleTickMsg()
	if m.playback.elapsedSecs != 71 {
		t.Fatalf("settled reading not applied: %v", m.playback.elapsedSecs)
	}
	// After the window, any reading is trusted again.
	m = m.noteSeekLanded(100)
	clock.advance(2 * time.Second)
	p.elapsed = 20
	m, _ = m.handleTickMsg()
	if m.playback.elapsedSecs != 20 {
		t.Fatalf("reading after the settle window ignored: %v", m.playback.elapsedSecs)
	}
}

func TestSeekToTimeCancelsPendingPreview(t *testing.T) {
	m, p, _ := newSeekPreviewModel(t)
	m, _ = pressArrow(t, m, tea.KeyRight)
	m, _ = m.seekToTime(200)
	if m.seek.pending {
		t.Fatal("Seek to time left the key preview pending")
	}
	next, _ := m.Update(seekCommitMsg{seq: m.seek.seq})
	m = next.(Model)
	if len(p.absolute) != 1 || p.absolute[0] != 200 || m.playback.elapsedSecs != 200 {
		t.Fatalf("absolute seeks = %v elapsed=%v, want only the 200s seek", p.absolute, m.playback.elapsedSecs)
	}
}

func TestSeekPreviewShownInNowPlaying(t *testing.T) {
	m, _, _ := newSeekPreviewModel(t)
	m.overlay.nowPlaying = &overlay.NowPlayingOverlay{Title: "Song"}
	m, _ = pressArrow(t, m, tea.KeyRight)
	if m.overlay.nowPlaying == nil {
		t.Fatal("seek key closed Now Playing")
	}
	np := m.decorateNowPlaying(*m.overlay.nowPlaying, *m.styles, time.Now())
	if np.SeekPreview != "→" || np.Elapsed != 65 {
		t.Fatalf("Now Playing preview = %q at %v, want → at 65", np.SeekPreview, np.Elapsed)
	}
}
