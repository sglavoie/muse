package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/lyrics"
)

// denseNowPlaying is the dense fixture (1:23 into its song) at a size with
// Now Playing open.
func denseNowPlaying(t *testing.T, width, height int) Model {
	t.Helper()
	m := newDenseFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: width, Height: height})
	return pressKey(m, "I")
}

func TestNowPlayingShowsSyncedLyricsAtEverySize(t *testing.T) {
	for _, size := range [][2]int{{60, 15}, {80, 24}, {120, 36}} {
		m := denseNowPlaying(t, size[0], size[1])
		plain := ansi.Strip(m.View().Content)
		// 1:23 is the eleventh synthetic line, which starts at 1:20.
		if !strings.Contains(plain, "Lyrics") || !strings.Contains(plain, "Radio static") {
			t.Fatalf("%dx%d: lyrics missing:\n%s", size[0], size[1], plain)
		}
		lines := strings.Split(plain, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%dx%d: line wider than the screen: %q", size[0], size[1], line)
			}
		}
	}
}

func TestLyricsKeyTogglesAndRemembers(t *testing.T) {
	m := denseNowPlaying(t, 120, 36)
	m = pressKey(m, "L")
	if m.cfg.Lyrics {
		t.Fatal("L should turn lyrics off")
	}
	if m.overlay.nowPlaying == nil {
		t.Fatal("L must not leave Now Playing for Insights")
	}
	plain := ansi.Strip(m.View().Content)
	if strings.Contains(plain, "Radio static") || !strings.Contains(plain, "Lyrics: Off") {
		t.Fatalf("lyrics should hide with a notice:\n%s", plain)
	}
	m = pressKey(m, "L")
	if !m.cfg.Lyrics || !strings.Contains(ansi.Strip(m.View().Content), "Radio static") {
		t.Fatal("L again shows them")
	}
}

func TestLyricsFollowSeeksAtOnce(t *testing.T) {
	m := denseNowPlaying(t, 80, 24)
	m.motion.enabled = true
	m = m.stepLyrics(time.Now()) // the frame loop has seen line 10
	m.playback.elapsedSecs = 20  // seek back to line 2
	view := m.nowPlayingLyrics(time.Now())
	if view.Current != 2 || view.Anchor != 2 {
		t.Fatalf("a seek moves the highlight and the window at once: current %d anchor %d", view.Current, view.Anchor)
	}
	m.motion.enabled = false
	m.playback.elapsedSecs = 160
	if view := m.nowPlayingLyrics(time.Now()); view.Current != 17 || view.Anchor != 17 || view.Fade != 1 {
		t.Fatalf("without motion the window jumps: %+v", view)
	}
}

func TestLyricsWindowGlidesWithMotion(t *testing.T) {
	m := denseNowPlaying(t, 80, 24)
	m.motion.enabled = true
	m.playback.elapsedSecs = 8 // line 1
	now := time.Now()
	m = m.stepLyrics(now)
	if m.lyricsView.shown != 1 || m.lyricsMoving(now.Add(time.Second)) {
		t.Fatalf("settled on line 1: %+v", m.lyricsView)
	}
	m.playback.elapsedSecs = 24 // line 3, two lines on
	view := m.nowPlayingLyrics(now)
	if view.Current != 3 || view.Anchor != 1 || view.Fade != 0 {
		t.Fatalf("the window trails the highlight before the next frame: %+v", view)
	}
	if !m.lyricsMoving(now) {
		t.Fatal("a trailing window asks for transition frames")
	}
	m = m.stepLyrics(now)
	if m.lyricsView.shown != 2 || m.lyricsView.line != 3 {
		t.Fatalf("one line per frame: %+v", m.lyricsView)
	}
	m = m.stepLyrics(now.Add(motionTransitionFrame))
	if m.lyricsView.shown != 3 {
		t.Fatalf("caught up: %+v", m.lyricsView)
	}
	later := now.Add(motionLyricsDuration)
	if m.lyricsMoving(later) {
		t.Fatal("once caught up and faded the loop goes back to the steady frame")
	}
	if view := m.nowPlayingLyrics(later); view.Fade != 1 || view.Anchor != 3 {
		t.Fatalf("settled: %+v", view)
	}
}

// slowLyrics is a loader that answers through a command, like reading a
// file or probing tags.
type slowLyrics struct {
	lyr   *lyrics.Lyrics
	err   error
	calls *int
}

func (s slowLyrics) Load(context.Context, string) (*lyrics.Lyrics, error) {
	*s.calls++
	return s.lyr, s.err
}

// findLyricsLoaded runs cmd, and the commands of any batch it returns,
// until one delivers a lyrics lookup.
func findLyricsLoaded(cmd tea.Cmd) (lyricsLoadedMsg, bool) {
	if cmd == nil {
		return lyricsLoadedMsg{}, false
	}
	switch msg := cmd().(type) {
	case lyricsLoadedMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if found, ok := findLyricsLoaded(c); ok {
				return found, true
			}
		}
	}
	return lyricsLoadedMsg{}, false
}

// openWithLoader opens Now Playing on the playback fixture with loader
// answering its lyrics lookup, and delivers the answer.
func openWithLoader(t *testing.T, loader lyrics.Loader) Model {
	t.Helper()
	m := newPlaybackFixture()
	WithLyrics(loader)(&m)
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 120, Height: 36})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	m = next.(Model)
	msg, ok := findLyricsLoaded(cmd)
	if !ok {
		t.Fatal("opening Now Playing should look the lyrics up")
	}
	return m.handleLyricsLoaded(msg)
}

func TestLyricsLoadLazilyAndOnce(t *testing.T) {
	calls := 0
	m := newPlaybackFixture()
	loader := slowLyrics{lyr: lyrics.ParseString("Plain words\nsecond"), calls: &calls}
	WithLyrics(loader)(&m)
	if _, cmd := m.syncLyrics(); cmd != nil {
		t.Fatal("nothing is read until Now Playing opens")
	}
	m = pressKey(m, "I")
	if calls != 0 || !m.lyrics.pending["/music/bob/gamma/g1.flac"] {
		t.Fatal("the lookup runs in the background")
	}
	if _, again := m.syncLyrics(); again != nil {
		t.Fatal("a pending lookup is not started twice")
	}
	m = openWithLoader(t, loader)
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	if _, again := m.syncLyrics(); again != nil {
		t.Fatal("cached lyrics are not looked up again")
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Plain words") || !strings.Contains(plain, "↑/↓ lyrics") {
		t.Fatalf("unsynced lyrics show as text:\n%s", plain)
	}
	m = pressKey(m, "j")
	if m.lyricsView.scroll != 0 {
		t.Fatalf("two lines fit the window, so there is nothing to scroll: %d", m.lyricsView.scroll)
	}
}

func TestLyricsMissingLeaveNoTrace(t *testing.T) {
	calls := 0
	m := openWithLoader(t, slowLyrics{err: lyrics.ErrNoLyrics, calls: &calls})
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	plain := ansi.Strip(m.View().Content)
	if strings.Contains(strings.ToLower(plain), "lyrics") {
		t.Fatalf("no lyrics, no mention:\n%s", plain)
	}
	if m.nowPlayingLyrics(time.Now()) != nil {
		t.Fatal("a failed lookup caches none")
	}
}

func TestLyricsUnsyncedScroll(t *testing.T) {
	calls := 0
	text := strings.Repeat("words\n", 12)
	m := openWithLoader(t, slowLyrics{lyr: lyrics.ParseString(text), calls: &calls})
	m = pressKey(pressKey(m, "j"), "j")
	if m.lyricsView.scroll != 2 {
		t.Fatalf("j scrolls down: %d", m.lyricsView.scroll)
	}
	m = pressKey(m, "k")
	if m.lyricsView.scroll != 1 || m.nowPlayingLyrics(time.Now()).Anchor != 1 {
		t.Fatalf("k scrolls up: %d", m.lyricsView.scroll)
	}
	for range 20 {
		m = pressKey(m, "j")
	}
	if m.lyricsView.scroll != 12-lyricsScrollTail {
		t.Fatalf("scrolling stops near the end: %d", m.lyricsView.scroll)
	}
}
