package tui

import (
	"testing"
	"time"
)

var sleepEpoch = time.Date(2026, 11, 5, 23, 0, 0, 0, time.UTC)

func TestSleepTimerCountdownAndExpiry(t *testing.T) {
	timer := sleepTimer{}.armTimed(sleepEpoch, 30*time.Minute)
	if !timer.armed() || timer.mode != sleepTimed {
		t.Fatalf("armTimed left %+v", timer)
	}
	cases := []struct {
		after time.Duration
		label string
		due   bool
	}{
		{0, "☾ 30m", false},
		{30 * time.Second, "☾ 30m", false},
		{7 * time.Minute, "☾ 23m", false},
		{7*time.Minute + time.Second, "☾ 23m", false},
		{29 * time.Minute, "☾ 60s", false},
		{29*time.Minute + 15*time.Second, "☾ 45s", false},
		{30*time.Minute - time.Millisecond, "☾ 1s", false},
		{30 * time.Minute, "☾ 0s", true},
		{31 * time.Minute, "☾ 0s", true},
	}
	for _, tc := range cases {
		now := sleepEpoch.Add(tc.after)
		if got := timer.label(now); got != tc.label {
			t.Errorf("label after %s = %q, want %q", tc.after, got, tc.label)
		}
		if got := timer.due(now); got != tc.due {
			t.Errorf("due after %s = %v, want %v", tc.after, got, tc.due)
		}
	}
	if rem := timer.remaining(sleepEpoch.Add(time.Hour)); rem != 0 {
		t.Errorf("remaining past the deadline = %s, want 0", rem)
	}
}

func TestSleepTimerTicksOnlyWhenTheLabelChanges(t *testing.T) {
	timer := sleepTimer{}.armTimed(sleepEpoch, 30*time.Minute)
	cases := []struct {
		after time.Duration
		want  time.Duration
	}{
		{0, time.Minute},
		{20 * time.Second, 40 * time.Second},
		{28*time.Minute + 30*time.Second, 30 * time.Second},
		{29 * time.Minute, time.Second},
		{29*time.Minute + 30*time.Second + 250*time.Millisecond, 750 * time.Millisecond},
		{31 * time.Minute, time.Second},
	}
	for _, tc := range cases {
		if got := timer.nextTick(sleepEpoch.Add(tc.after)); got != tc.want {
			t.Errorf("nextTick after %s = %s, want %s", tc.after, got, tc.want)
		}
	}
	// Walking the countdown tick by tick redraws once a minute and then once a
	// second: 29 minute ticks plus 60 second ticks reach the deadline.
	now, ticks := sleepEpoch, 0
	for !timer.due(now) {
		now = now.Add(timer.nextTick(now))
		ticks++
	}
	if ticks != 29+60 {
		t.Errorf("reached the deadline in %d ticks, want %d", ticks, 29+60)
	}
}

func TestSleepTimerTransitionsBumpGeneration(t *testing.T) {
	timer := sleepTimer{}.armTimed(sleepEpoch, time.Minute)
	gen := timer.gen
	fading := timer.beginFade()
	if !fading.fading || fading.gen != gen {
		t.Fatalf("beginFade = %+v, want fading in the same arming", fading)
	}
	if fading.due(sleepEpoch.Add(time.Hour)) {
		t.Error("a fading timer is not due again")
	}
	if got := fading.label(sleepEpoch); got != "☾ fading" {
		t.Errorf("fading label = %q", got)
	}
	off := fading.off()
	if off.armed() || off.gen <= gen || off.label(sleepEpoch) != "" {
		t.Errorf("off = %+v, want disarmed in a newer generation", off)
	}
	rearmed := off.armUntil(sleepEndOfQueue)
	if rearmed.gen <= off.gen || rearmed.label(sleepEpoch) != "☾ end of queue" {
		t.Errorf("armUntil = %+v", rearmed)
	}
	if rearmed.remaining(sleepEpoch) != 0 || rearmed.due(sleepEpoch.Add(time.Hour)) {
		t.Error("an event-driven timer has no countdown")
	}
	if got := (sleepTimer{}).armUntil(sleepEndOfSong).label(sleepEpoch); got != "☾ end of song" {
		t.Errorf("end-of-song label = %q", got)
	}
}

func TestSleepChoicesReadInOrderByName(t *testing.T) {
	choices := sleepChoices()
	want := []string{"15 minutes", "30 minutes", "45 minutes", "60 minutes", "90 minutes", "End of queue", "End of this song", "Off"}
	if len(choices) != len(want) {
		t.Fatalf("got %d choices, want %d", len(choices), len(want))
	}
	for i, c := range choices {
		if c.name != want[i] {
			t.Errorf("choice %d = %q, want %q", i, c.name, want[i])
		}
		if i > 0 && choices[i-1].name >= c.name {
			t.Errorf("choice %q does not sort after %q, so the palette would reorder them", c.name, choices[i-1].name)
		}
		if !IsSafeDirectCommand(c.id) {
			t.Errorf("choice %q is not fixture-safe", c.id)
		}
	}
}
