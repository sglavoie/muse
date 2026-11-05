package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// stubSleepFader is a command player whose FadeOutAndPause blocks until either
// the test releases it or its context is cancelled.
type stubSleepFader struct {
	stubStopAfterPlayer
	mu        sync.Mutex
	fades     int
	duration  time.Duration
	release   chan struct{}
	cancelled chan struct{}
}

func newStubSleepFader() *stubSleepFader {
	return &stubSleepFader{release: make(chan struct{}), cancelled: make(chan struct{}, 1)}
}

func (s *stubSleepFader) FadeOutAndPause(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.fades++
	s.duration = d
	s.mu.Unlock()
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		s.cancelled <- struct{}{}
		return ctx.Err()
	}
}

// sleepClock is a settable clock for the Model's sleep timer.
type sleepClock struct{ now time.Time }

func (c *sleepClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func sleepModel(t *testing.T, p playPauser) (Model, *sleepClock) {
	t.Helper()
	clock := &sleepClock{now: sleepEpoch}
	m := New(context.Background(), WithPlayer(p))
	m.sleep.now = func() time.Time { return clock.now }
	m.playback.state = playback.StatePlaying
	m.queue.Entries = []queue.Entry{{ID: 1, Path: "/a.flac", Hash: "a"}, {ID: 2, Path: "/b.flac", Hash: "b"}}
	m.playback.currentIndex, m.playback.currentHash = 0, "a"
	return m, clock
}

// choose applies the picker choice with the given id.
func choose(t *testing.T, m Model, id string) Model {
	t.Helper()
	for _, c := range sleepChoices() {
		if c.id == id {
			next, _ := m.Update(sleepTimerChoiceMsg{choice: c})
			return next.(Model)
		}
	}
	t.Fatalf("no sleep choice %q", id)
	return m
}

func TestSleepTimerTimedFadesThenPauses(t *testing.T) {
	p := newStubSleepFader()
	m, clock := sleepModel(t, p)

	m = choose(t, m, "sleep.timer.30")
	if got := m.RenderStatusBar(); !strings.Contains(got, "☾ 30m") {
		t.Fatalf("status bar lacks the countdown: %q", got)
	}
	gen := m.sleep.timer.gen

	// A tick before the deadline only redraws and re-arms.
	clock.advance(7 * time.Minute)
	next, cmd := m.Update(sleepTickMsg{gen: gen})
	m = next.(Model)
	if cmd == nil || m.sleepFading() {
		t.Fatal("an early tick must reschedule, not fade")
	}
	if got := m.sleepTimerLabel(); got != "☾ 23m" {
		t.Errorf("label after 7 minutes = %q", got)
	}
	// A stale tick from an older arming does nothing.
	if _, cmd := m.Update(sleepTickMsg{gen: gen - 1}); cmd != nil {
		t.Error("a stale tick rescheduled itself")
	}

	clock.advance(23 * time.Minute)
	next, cmd = m.Update(sleepTickMsg{gen: gen})
	m = next.(Model)
	if !m.sleepFading() || cmd == nil {
		t.Fatal("the deadline did not start the fade")
	}
	done := make(chan sleepFadeDoneMsg, 1)
	go func() { done <- cmd().(sleepFadeDoneMsg) }()
	close(p.release)
	msg := <-done
	if p.duration != sleepFadeDuration {
		t.Errorf("fade duration = %s, want %s", p.duration, sleepFadeDuration)
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.playback.state != playback.StatePaused || m.sleep.timer.armed() {
		t.Fatalf("after the fade: state=%v armed=%v, want paused and disarmed", m.playback.state, m.sleep.timer.armed())
	}
	if m.sleepTimerLabel() != "" {
		t.Error("the marker outlived the timer")
	}
}

func TestSleepTimerPlayDuringFadeCancels(t *testing.T) {
	for _, via := range []string{"key", "media play", "media pause"} {
		t.Run(via, func(t *testing.T) {
			p := newStubSleepFader()
			m, clock := sleepModel(t, p)
			m = choose(t, m, "sleep.timer.15")
			clock.advance(15 * time.Minute)
			next, cmd := m.Update(sleepTickMsg{gen: m.sleep.timer.gen})
			m = next.(Model)
			done := make(chan sleepFadeDoneMsg, 1)
			go func() { done <- cmd().(sleepFadeDoneMsg) }()

			wantState := playback.StatePlaying
			switch via {
			case "key":
				rm, _, handled := m.handlePlaybackKeyAction(keys.ActionPlayPause)
				if !handled {
					t.Fatal("play/pause during the fade was not handled")
				}
				m = rm.(Model)
			case "media play":
				m, _ = controlRequest(t, m, control.Request{ID: 1, Op: control.OpPlay})
			case "media pause":
				m, _ = controlRequest(t, m, control.Request{ID: 1, Op: control.OpPause})
				wantState = playback.StatePaused
			}
			select {
			case <-p.cancelled:
			case <-time.After(2 * time.Second):
				t.Fatal("the fade was not cancelled")
			}
			if m.sleep.timer.armed() {
				t.Error("the timer stayed armed after the cancel")
			}
			if m.playback.state != wantState {
				t.Errorf("state = %v, want %v", m.playback.state, wantState)
			}
			// The cancelled fade's result is ignored.
			next, _ = m.Update(<-done)
			if next.(Model).playback.state != wantState {
				t.Error("the cancelled fade's result changed playback state")
			}
		})
	}
}

func TestSleepTimerDueWhileNotPlayingJustEnds(t *testing.T) {
	p := newStubSleepFader()
	m, clock := sleepModel(t, p)
	m = choose(t, m, "sleep.timer.45")
	m.playback.state = playback.StatePaused
	clock.advance(time.Hour)
	next, _ := m.Update(sleepTickMsg{gen: m.sleep.timer.gen})
	m = next.(Model)
	if m.sleep.timer.armed() || p.fades != 0 {
		t.Fatalf("armed=%v fades=%d, want the timer spent without a fade", m.sleep.timer.armed(), p.fades)
	}
}

func TestSleepTimerEndOfSongArmsStopAfter(t *testing.T) {
	p := newStubSleepFader()
	m, _ := sleepModel(t, p)
	m = choose(t, m, "sleep.timer.song")
	if !m.playback.stopAfterCurrent || !p.stopAfter {
		t.Fatal("end of this song did not arm stop-after-current")
	}
	if got := m.sleepTimerLabel(); got != "☾ end of song" {
		t.Errorf("label = %q", got)
	}
	if modes := m.nowPlayingModes(); strings.Contains(modes, "Stops after this song") {
		t.Errorf("Now Playing repeats the stop the timer already names: %q", modes)
	}

	// Switching to a countdown hands stop-after-current back.
	m = choose(t, m, "sleep.timer.60")
	if m.playback.stopAfterCurrent || p.stopAfter {
		t.Error("a countdown left the end-of-song stop armed")
	}

	// And ctrl+n taking the stop back retires the timer's claim on it.
	m = choose(t, m, "sleep.timer.song")
	rm, _, _ := m.handlePlaybackKeyAction(keys.ActionStopAfterCurrent)
	m = rm.(Model)
	if m.sleep.timer.armed() || m.sleepTimerLabel() != "" {
		t.Error("toggling stop-after off left the sleep timer armed")
	}
}

func TestSleepTimerEndOfQueueHoldsRepeatAndAutoDJ(t *testing.T) {
	p := &stubPlayer{}
	m, _ := sleepModel(t, p)
	m.playback.repeatMode = playback.RepeatAll
	m = choose(t, m, "sleep.timer.queue")
	if !m.sleepHoldsQueueEnd() {
		t.Fatal("end of queue not armed")
	}
	m.radio.enabled = true
	if m.radioCanEnqueue() {
		t.Error("Auto-DJ may refill while the timer waits for the end of the queue")
	}

	next, _ := m.Update(playback.PlaybackEndedMsg{})
	m = next.(Model)
	if len(p.playFromIndexCalls) != 0 {
		t.Error("repeat-all wrapped the queue the timer was waiting to end")
	}
	if m.playback.state != playback.StateIdle || m.sleep.timer.armed() {
		t.Fatalf("state=%v armed=%v, want idle and spent", m.playback.state, m.sleep.timer.armed())
	}
	if got := m.statusBar.TopNotification().Msg; got != "Sleep timer: queue finished" {
		t.Errorf("notice = %q", got)
	}
}

func TestSleepTimerEndOfQueueUnderRepeatOneStopsAfterTheSong(t *testing.T) {
	p := newStubSleepFader()
	m, _ := sleepModel(t, p)
	m.playback.repeatMode = playback.RepeatOne
	m = choose(t, m, "sleep.timer.queue")
	if m.sleep.timer.mode != sleepEndOfSong || !m.playback.stopAfterCurrent {
		t.Fatalf("repeat one: mode=%v stopAfter=%v, want the end of this song", m.sleep.timer.mode, m.playback.stopAfterCurrent)
	}
}

func TestSleepTimerOffAndPicker(t *testing.T) {
	m, _ := sleepModel(t, newStubSleepFader())
	next, _ := m.Update(openSleepTimerPickerMsg{})
	m = next.(Model)
	if m.overlay.commandPalette == nil {
		t.Fatal("the picker did not open")
	}
	m.overlay.commandPalette = nil

	m = choose(t, m, "sleep.timer.90")
	gen := m.sleep.timer.gen
	m = choose(t, m, "sleep.timer.off")
	if m.sleep.timer.armed() || m.statusBar.TopNotification().Msg != "Sleep timer off" {
		t.Fatalf("off left %+v / %q", m.sleep.timer, m.statusBar.TopNotification().Msg)
	}
	if _, cmd := m.Update(sleepTickMsg{gen: gen}); cmd != nil {
		t.Error("the abandoned countdown kept ticking")
	}
}
