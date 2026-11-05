package tui

import (
	"fmt"
	"time"
)

// This file holds the sleep timer's state machine. It is deliberately pure:
// every method takes the current time instead of reading a clock, so the
// countdown, its expiry and its labels can be tested against an injected clock,
// and the Model wiring in root_sleep_timer.go only decides when to ask.

// sleepMode is what the sleep timer is waiting for.
type sleepMode uint8

const (
	sleepOff        sleepMode = iota
	sleepTimed                // a countdown to deadline, then a fade into pause
	sleepEndOfSong            // the end of the current song (stop-after-current)
	sleepEndOfQueue           // the end of the queue, with no Auto-DJ refill or repeat wrap
)

// sleepFadeDuration is how long the timed sleep timer takes to fade the volume
// to silence before it pauses.
const sleepFadeDuration = 15 * time.Second

// sleepTimer is the timer's whole state. gen identifies one arming: every
// transition that abandons a countdown or a fade bumps it, so a tick or a fade
// result belonging to an older arming is recognised and dropped.
type sleepTimer struct {
	mode     sleepMode
	deadline time.Time
	fading   bool
	gen      uint64
}

// armTimed starts a countdown of d from now.
func (t sleepTimer) armTimed(now time.Time, d time.Duration) sleepTimer {
	return sleepTimer{mode: sleepTimed, deadline: now.Add(d), gen: t.gen + 1}
}

// armUntil waits for the end of the song or of the queue.
func (t sleepTimer) armUntil(mode sleepMode) sleepTimer {
	return sleepTimer{mode: mode, gen: t.gen + 1}
}

// off disarms the timer, abandoning any countdown or fade in progress.
func (t sleepTimer) off() sleepTimer {
	return sleepTimer{gen: t.gen + 1}
}

// armed reports whether the timer is waiting for anything.
func (t sleepTimer) armed() bool { return t.mode != sleepOff }

// remaining is how long the countdown has left, never negative. It is zero for
// the modes that wait for an event rather than a time.
func (t sleepTimer) remaining(now time.Time) time.Duration {
	if t.mode != sleepTimed {
		return 0
	}
	return max(t.deadline.Sub(now), 0)
}

// due reports whether the countdown has run out and the fade has not begun.
func (t sleepTimer) due(now time.Time) bool {
	return t.mode == sleepTimed && !t.fading && !now.Before(t.deadline)
}

// beginFade records that the fade into pause is under way.
func (t sleepTimer) beginFade() sleepTimer {
	t.fading = true
	return t
}

// nextTick is how long to wait before the countdown's label next changes, so
// the timer redraws once a minute while minutes are shown and once a second for
// the last minute, and never faster.
func (t sleepTimer) nextTick(now time.Time) time.Duration {
	rem := t.remaining(now)
	step := time.Minute
	if rem <= time.Minute {
		step = time.Second
	}
	next := rem % step
	if next == 0 {
		next = step
	}
	return max(next, 10*time.Millisecond)
}

// label is the compact marker the status bar and Now Playing show, or "" when
// the timer is off. The countdown rounds up, so "1m" means "less than a minute
// and more than zero seconds left" never reads as "0m".
func (t sleepTimer) label(now time.Time) string {
	switch t.mode {
	case sleepTimed:
		if t.fading {
			return "☾ fading"
		}
		rem := t.remaining(now)
		if rem <= time.Minute {
			return fmt.Sprintf("☾ %ds", ceilDiv(rem, time.Second))
		}
		return fmt.Sprintf("☾ %dm", ceilDiv(rem, time.Minute))
	case sleepEndOfSong:
		return "☾ end of song"
	case sleepEndOfQueue:
		return "☾ end of queue"
	default:
		return ""
	}
}

// ceilDiv divides d by unit, rounding up.
func ceilDiv(d, unit time.Duration) int64 {
	return int64((d + unit - 1) / unit)
}

// sleepChoice is one option of the "Sleep timer…" picker.
type sleepChoice struct {
	id       string
	name     string
	mode     sleepMode
	duration time.Duration
}

// sleepChoices lists the picker's options. The palette lists an empty query by
// name, so the names are chosen to read in this order there too: the
// durations ascending, then the two events, then Off.
func sleepChoices() []sleepChoice {
	choices := make([]sleepChoice, 0, 8)
	for _, minutes := range []int{15, 30, 45, 60, 90} {
		choices = append(choices, sleepChoice{
			id:       fmt.Sprintf("sleep.timer.%d", minutes),
			name:     fmt.Sprintf("%d minutes", minutes),
			mode:     sleepTimed,
			duration: time.Duration(minutes) * time.Minute,
		})
	}
	return append(choices,
		sleepChoice{id: "sleep.timer.queue", name: "End of queue", mode: sleepEndOfQueue},
		sleepChoice{id: "sleep.timer.song", name: "End of this song", mode: sleepEndOfSong},
		sleepChoice{id: "sleep.timer.off", name: "Off", mode: sleepOff},
	)
}
