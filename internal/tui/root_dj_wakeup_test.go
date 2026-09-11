package tui

import (
	"errors"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// These two tests are about the running application rather than about the
// Model's methods. A refill that parks — because a write was refused, or because
// the filters admitted nothing — is asked again only if something asks it, and
// once playback is exhausted no track change and no queue end will ever arrive
// to do so. So nothing here calls startRadioEnqueueCmd: every refill below is
// launched by an ordinary update message and by the commands that message
// returns, which is the only way to tell a wakeup the application has from one
// only a test has.

// teaTickCmd is the code pointer of the closure tea.Tick returns. Every periodic
// command this package schedules is a tea.Tick — the one-second tick, the
// session tick, the now-playing pulse, the notification fade — and running one
// blocks for its whole interval before rescheduling itself forever. Recognising
// them lets djWakeDrive deliver everything else the update loop scheduled
// without either sleeping through a timer or chasing the timer loop.
var teaTickCmd = reflect.ValueOf(tea.Tick(time.Hour, func(time.Time) tea.Msg { return nil })).Pointer()

func isPeriodicCmd(cmd tea.Cmd) bool {
	return cmd != nil && reflect.ValueOf(cmd).Pointer() == teaTickCmd
}

// djWakeDrive delivers msg through the ordinary update loop and keeps going with
// whatever the handlers schedule, which is how a refill is followed from the
// message that wakes it, through planning, to the acceptance that publishes the
// batch. depth bounds the chain the way the runtime never needs to.
func djWakeDrive(m Model, msg tea.Msg, depth int) Model {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if depth <= 0 {
		return m
	}
	for _, out := range djWakeMessages(cmd) {
		m = djWakeDrive(m, out, depth-1)
	}
	return m
}

// djWakeMessages runs cmd and flattens what it produces, descending into
// batches the way the runtime's scheduler does and skipping its timers.
func djWakeMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil || isPeriodicCmd(cmd) {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, djWakeMessages(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// djWakeTicks delivers n one-second ticks, each with the commands it schedules,
// the way a user staring at an exhausted queue would.
func djWakeTicks(m Model, n int) Model {
	for i := 0; i < n; i++ {
		m = djWakeDrive(m, tickMsg{}, 4)
	}
	return m
}

// TestAutoDJTransientRefillRetriesFromUpdateLoop covers a refused write on an
// exhausted queue, from both ends.
//
// A refused write parks the refill, and the queue running dry is exactly the
// moment there is no playback event left to ask again — so a writer that
// recovers a second later has to be noticed by the periodic tick or continuous
// radio is over until the user presses something. The same tick must not turn a
// database that is permanently unhappy into a write loop, so the retry budget
// bounds it and the refill goes quiet for good once it is spent.
func TestAutoDJTransientRefillRetriesFromUpdateLoop(t *testing.T) {
	t.Run("a writer that recovers is picked up without a playback event", func(t *testing.T) {
		writer := &stubDJQueueWriter{prepareErr: errors.New("disk full")}
		m := djRefillModel(writer)

		m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)

		if len(writer.prepared) != 1 {
			t.Fatalf("the queue end attempted %d batches, want exactly 1", len(writer.prepared))
		}
		if !m.DJPlanBlocked() || m.djPlanBlockErr() == nil {
			t.Fatal("a refused write did not park the refill with its error")
		}
		if !m.playback.queueExhausted {
			t.Fatal("the queue did not end, so nothing below is about an exhausted refill")
		}

		writer.prepareErr = nil
		m = djWakeTicks(m, 1)

		if len(writer.accepted) != 1 {
			t.Fatalf("published %d batches after the writer recovered, want 1: "+
				"nothing in the running application asked the refill again", len(writer.accepted))
		}
		if m.DJPlanBlocked() {
			t.Error("a refill that published a batch is still parked")
		}
		if got := len(m.queue.Entries); got != 1+radioBatchSize {
			t.Errorf("the queue holds %d rows, want the played row plus a full batch", got)
		}
	})

	t.Run("a writer that never recovers stops after its budget", func(t *testing.T) {
		writer := &stubDJQueueWriter{prepareErr: errors.New("disk full")}
		m := djRefillModel(writer)

		m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
		m = djWakeTicks(m, djMaxTransientRetries+3)

		if len(writer.prepared) != djMaxTransientRetries {
			t.Errorf("a permanently failing writer was attempted %d times, want %d",
				len(writer.prepared), djMaxTransientRetries)
		}
		if len(writer.accepted) != 0 {
			t.Errorf("a failing writer published %d batches", len(writer.accepted))
		}
		if !m.DJPlanBlocked() || m.djPlanBlockErr() == nil {
			t.Error("the refill stopped retrying without recording why")
		}
		if got := len(m.queue.Entries); got != 1 {
			t.Errorf("the queue grew to %d rows behind a writer that refused every batch", got)
		}
	})
}

// TestAutoDJExhaustedRefillWakesOnEligibilityChange covers the other way a
// refill goes silent: the filters admitted nothing.
//
// Parking on an empty result is deliberate — "no candidates pauses additions" —
// and re-asking the same question every second would defeat it. But the answer
// changes the moment a track the filters do admit appears, and a watcher batch
// only advances the data revision: it launches nothing. With the queue already
// exhausted there is no track change and no queue end left either, so the tick
// is the only thing that can notice, and it must notice only once the eligible
// track is actually there.
func TestAutoDJExhaustedRefillWakesOnEligibilityChange(t *testing.T) {
	writer := &stubDJQueueWriter{}
	reader := newDJRecReader()
	m := djRefillModel(writer)
	m.recReader = reader
	m.dj.settings = djDroneSettings()

	// The queue runs out while the session is filtered to a genre the library
	// does not carry.
	m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
	if !m.DJPlanBlocked() {
		t.Fatal("an empty result left the refill unparked, so it would re-ask every tick")
	}
	if !m.playback.queueExhausted {
		t.Fatal("the queue did not end, so nothing below is about an exhausted refill")
	}

	// One tick settles the park onto the inputs in force: the queue end moved
	// the protected set out from under the fingerprint the refill was launched
	// with, and that is a real input change like any other.
	m = djWakeTicks(m, 1)
	_, settled, _ := reader.snapshot()

	m = djWakeTicks(m, 4)
	if _, attempts, _ := reader.snapshot(); attempts != settled {
		t.Fatalf("%d further plans ran against inputs that had not moved, want 0: "+
			"the park no longer pauses additions", attempts-settled)
	}
	if len(writer.prepared) != 0 {
		t.Fatalf("prepared %d batches with nothing eligible to select", len(writer.prepared))
	}

	// A track the filters admit arrives through the watcher, which bumps the
	// data revision and returns no refill command of its own.
	m = djWakeDrive(m, watchBatchMsg{tracks: []library.Track{{
		Artist: "Gus", Album: "Eta", Title: "N1", Hash: "n1",
		Path: "/n1.flac", Genre: "Drone", Duration: 300,
	}}}, 4)

	if len(writer.accepted) != 0 {
		t.Fatalf("the watcher batch published %d batches by itself, so the tick is not under test",
			len(writer.accepted))
	}

	m = djWakeTicks(m, 1)

	if len(writer.accepted) != 1 {
		t.Fatalf("published %d batches after an eligible track arrived, want 1: "+
			"the exhausted refill was never woken", len(writer.accepted))
	}
	if m.DJPlanBlocked() {
		t.Error("a refill that published a batch is still parked")
	}
	if got := len(m.queue.Entries); got != 2 {
		t.Fatalf("the queue holds %d rows, want the played row plus the one eligible track", got)
	}
	if got := m.queue.Entries[1].Hash; got != "n1" {
		t.Errorf("the batch selected %q, want the newly eligible track", got)
	}
}

// djDroneSettings filters the session to a genre no fixture track carries, which
// is the cheapest way to make the planner return nothing without breaking the
// library or the writer.
func djDroneSettings() radio.Settings {
	s := radio.DefaultSettings()
	s.Include.Genres = []string{"Drone"}
	return s
}
