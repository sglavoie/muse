package player

import (
	"context"
	"errors"
	"testing"
	"time"
)

// lastReplace returns the path and per-file options of the newest replacing
// loadfile the fake received.
func lastReplace(t *testing.T, f *fakePlaylist) (string, string) {
	t.Helper()
	cmds := f.commands()
	for i := len(cmds) - 1; i >= 0; i-- {
		if isLoadfileReplace(cmds[i]) {
			path, _ := cmds[i].stringAt(1)
			opts, _ := cmds[i].stringAt(len(cmds[i]) - 1)
			return path, opts
		}
	}
	t.Fatalf("no replacing loadfile in %v", cmds)
	return "", ""
}

func TestCueNext_LoadsSuccessorPausedAtStart(t *testing.T) {
	m, f := newFakePlaylistManager(t, "cue-next")
	q := queueOf(t, 3)
	installQueue(m, q, 0, false)

	if err := m.CueNext(); err != nil {
		t.Fatalf("CueNext: %v", err)
	}
	path, opts := lastReplace(t, f)
	if path != q[1].Path || opts != "pause=yes" {
		t.Fatalf("cue loaded %q with %q, want %q paused", path, opts, q[1].Path)
	}
	if n := f.count(func(c ipcTestCommand) bool { return isSetPause(c, false) }); n != 0 {
		t.Fatalf("cue must not unpause, saw %d unpause commands", n)
	}
	m.mu.RLock()
	idx, paused := m.currentIndex, m.lastPaused
	m.mu.RUnlock()
	if idx != 1 || !paused {
		t.Fatalf("after cue: currentIndex=%d lastPaused=%v, want 1 and true", idx, paused)
	}

	// The pause option belongs to the cue's transaction alone.
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if _, opts := lastReplace(t, f); opts != "pause=no" {
		t.Fatalf("an ordinary load after the cue used %q, want pause=no", opts)
	}
}

func TestCueNext_RepeatOneCuesTheSameSong(t *testing.T) {
	m, f := newFakePlaylistManager(t, "cue-one")
	q := queueOf(t, 3)
	installQueue(m, q, 1, false)
	m.mu.Lock()
	m.repeatMode = "one"
	m.mu.Unlock()

	if err := m.CueNext(); err != nil {
		t.Fatalf("CueNext: %v", err)
	}
	if path, opts := lastReplace(t, f); path != q[1].Path || opts != "pause=yes" {
		t.Fatalf("repeat-one cue loaded %q with %q, want the current song paused", path, opts)
	}
}

func TestCueNext_EndOfQueue(t *testing.T) {
	t.Run("repeat off reports exhaustion and loads nothing", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "cue-end")
		q := queueOf(t, 2)
		installQueue(m, q, 1, false)
		if err := m.CueNext(); !errors.Is(err, ErrPlaybackFailed) {
			t.Fatalf("CueNext at the end = %v, want ErrPlaybackFailed", err)
		}
		if n := f.count(isAnyLoadfile); n != 0 {
			t.Fatalf("exhausted cue sent %d loadfiles", n)
		}
	})
	t.Run("repeat all wraps to the first song", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "cue-wrap")
		q := queueOf(t, 2)
		installQueue(m, q, 1, false)
		m.mu.Lock()
		m.repeatMode = "all"
		m.mu.Unlock()
		if err := m.CueNext(); err != nil {
			t.Fatalf("CueNext: %v", err)
		}
		if path, opts := lastReplace(t, f); path != q[0].Path || opts != "pause=yes" {
			t.Fatalf("repeat-all cue loaded %q with %q, want the first song paused", path, opts)
		}
	})
}

// TestStopAfterCurrent_GaplessEndsCleanlyThenCues walks the whole stop: arming
// drops the preloaded successor, the song ends into an idle mpv without starting
// it, and the cue then parks it paused.
func TestStopAfterCurrent_GaplessEndsCleanlyThenCues(t *testing.T) {
	m, f := newFakePlaylistManager(t, "stop-cue")
	q := queueOf(t, 3)
	installQueue(m, q, 0, true)
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex: %v", err)
	}
	f.waitForEntries(t, paths(q, 0, 1))
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	m.SetStopAfterCurrent(true)
	f.waitForEntries(t, paths(q, 0))
	f.finishCurrent()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire")
	}
	if !f.idle() {
		t.Fatal("mpv started the next song after a stop-after-current")
	}
	if err := m.CueNext(); err != nil {
		t.Fatalf("CueNext: %v", err)
	}
	if path, opts := lastReplace(t, f); path != q[1].Path || opts != "pause=yes" {
		t.Fatalf("cue loaded %q with %q", path, opts)
	}
}

func TestSetStopAfterCurrent_LiftsRepeatOneLoop(t *testing.T) {
	m, f := newFakePlaylistManager(t, "stop-loop")
	if err := m.SetRepeat("one"); err != nil {
		t.Fatalf("SetRepeat: %v", err)
	}
	loopFile := func(value string) func() bool {
		return func() bool {
			cmds := f.commands()
			for i := len(cmds) - 1; i >= 0; i-- {
				if cmds[i].stringIs(0, cmdSetProperty) && cmds[i].stringIs(1, propLoopFile) {
					return cmds[i].stringIs(2, value)
				}
			}
			return false
		}
	}
	f.waitFor(t, "loop-file=inf", loopFile("inf"))
	m.SetStopAfterCurrent(true)
	f.waitFor(t, "loop-file=no while armed", loopFile("no"))
	if err := m.SetRepeat("one"); err != nil {
		t.Fatalf("SetRepeat: %v", err)
	}
	f.waitFor(t, "repeat-one stays lifted while armed", loopFile("no"))
	m.SetStopAfterCurrent(false)
	f.waitFor(t, "loop-file=inf after disarm", loopFile("inf"))
}

// volumeSets returns the volumes the fake was told to apply, in order.
func volumeSets(f *fakePlaylist) []float64 {
	var out []float64
	for _, c := range f.commands() {
		if c.stringIs(0, cmdSetProperty) && c.stringIs(1, propVolume) {
			if v, ok := c.float64At(2); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

func TestFadeOutAndPause_FadesPausesAndRestores(t *testing.T) {
	m, f := newFakePlaylistManager(t, "sleep-fade")
	m.mu.Lock()
	m.lastVolume, m.volumeKnown = 80, true
	m.mu.Unlock()

	if err := m.FadeOutAndPause(context.Background(), 4*sleepFadeStep); err != nil {
		t.Fatalf("FadeOutAndPause: %v", err)
	}
	got := volumeSets(f)
	want := []float64{60, 40, 20, 0, 80}
	if len(got) != len(want) {
		t.Fatalf("volume steps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("volume steps = %v, want %v", got, want)
		}
	}
	pauseAt := f.commandOrder(func(c ipcTestCommand) bool { return isSetPause(c, true) })
	restoreAt := len(f.commands()) - 1
	if pauseAt < 0 || pauseAt > restoreAt {
		t.Fatalf("pause must land before the volume is restored: commands=%v", f.commands())
	}
	m.mu.RLock()
	paused := m.lastPaused
	m.mu.RUnlock()
	if !paused {
		t.Fatal("lastPaused not mirrored after the fade")
	}
}

func TestFadeOutAndPause_CancelKeepsPlayingAtFullVolume(t *testing.T) {
	m, f := newFakePlaylistManager(t, "sleep-cancel")
	m.mu.Lock()
	m.lastVolume, m.volumeKnown = 50, true
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.FadeOutAndPause(ctx, time.Minute) }()
	f.waitFor(t, "the fade to start", func() bool { return len(volumeSets(f)) >= 2 })
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled fade returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled fade did not return")
	}
	if n := f.count(func(c ipcTestCommand) bool { return isSetPause(c, true) }); n != 0 {
		t.Fatal("a cancelled fade must not pause")
	}
	sets := volumeSets(f)
	if last := sets[len(sets)-1]; last != 50 {
		t.Fatalf("cancelled fade ended at volume %v, want 50", last)
	}
}
