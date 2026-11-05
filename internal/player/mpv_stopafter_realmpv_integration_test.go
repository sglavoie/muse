//go:build integration

package player

import (
	"testing"
	"time"
)

// waitForEnded blocks until OnPlaybackEnded has fired n times.
func (f *realMPVFixture) waitForEnded(n int, within time.Duration) {
	f.t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		ended := f.ended
		f.mu.Unlock()
		if ended >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("playback did not end within %s (starts: %v)", within, f.playedHashes())
}

// TestRealMPVStopAfterCurrentWithGapless is "stop after this song" against the
// real process with gapless on, which is the case that can leak: mpv preloads
// the successor and moves into it on its own. Arming must take that successor
// back, so the song ends into silence rather than into the next one, and the cue
// must then leave the next song loaded, paused at its start.
func TestRealMPVStopAfterCurrentWithGapless(t *testing.T) {
	f := newRealMPVFixture(t, 3, 2)
	if err := f.mgr.SetGapless(true); err != nil {
		t.Fatalf("enable gapless: %v", err)
	}
	if err := f.mgr.PlayFromIndex(0); err != nil {
		t.Fatalf("start playback: %v", err)
	}
	f.waitForStarts(1, 15*time.Second)
	f.waitForElapsed(0.3, 15*time.Second)
	// Give the gapless prefetch time to append the successor, so arming has a
	// preloaded entry to take back.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p, _ := prefetchTracking(f.mgr); p.index == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p, _ := prefetchTracking(f.mgr); p.index != 1 {
		t.Fatalf("the successor was never preloaded (prefetch index %d)", p.index)
	}

	f.mgr.SetStopAfterCurrent(true)
	f.waitForEnded(1, 15*time.Second)

	// Nothing may start on its own after the stop: not the preloaded successor,
	// not a fraction of it.
	time.Sleep(700 * time.Millisecond)
	if got := f.startCount(); got != 1 {
		t.Fatalf("%d songs started after stop-after-current, want only the first: %v",
			got, f.playedHashes())
	}
	if f.mgr.StopAfterCurrent() {
		t.Error("the flag was not spent when it fired")
	}

	if err := f.mgr.CueNext(); err != nil {
		t.Fatalf("cue the next song: %v", err)
	}
	starts := f.waitForStarts(2, 10*time.Second)
	if next := starts[1]; next.hash != f.tracks[1].Hash || next.index != 1 {
		t.Fatalf("the cue loaded %q at %d, want the next song %q", next.hash, next.index, f.tracks[1].Hash)
	}

	// Paused at its start, and staying there.
	time.Sleep(600 * time.Millisecond)
	c, err := f.mgr.ipc()
	if err != nil {
		t.Fatalf("ipc: %v", err)
	}
	paused, ok, err := c.GetPropertyBool(propPause)
	if err != nil || !ok || !paused {
		t.Fatalf("pause after the cue = %v (ok=%v, err=%v), want paused", paused, ok, err)
	}
	elapsed, _, err := f.mgr.ElapsedPosition()
	if err != nil {
		t.Fatalf("read the position: %v", err)
	}
	if elapsed > 0.2 {
		t.Errorf("the cued song is at %.2fs, want its start", elapsed)
	}
	if got := f.startCount(); got != 2 {
		t.Errorf("%d songs started, want exactly the cued one more: %v", got, f.playedHashes())
	}

	// Play resumes the cued song from its start.
	if err := f.mgr.Play(); err != nil {
		t.Fatalf("play: %v", err)
	}
	f.waitForElapsed(0.3, 10*time.Second)
}
