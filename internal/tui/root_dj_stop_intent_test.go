package tui

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestAutoDJStopIgnoresUnsolicitedTrackChanges is about the difference between
// the player telling the TUI where it is and the user telling the player to go
// somewhere.
//
// A deliberate stop suspends automatic refills until the user asks for audio
// again, and a confirmed track change was taken to be that asking. It is not:
// Stop pauses and seeks the file it was holding rather than unloading it, so
// mpv re-announces that same row whenever the event reader reconnects, paused or
// not, and a start-file emitted just before the stop can still be delivered
// after it. Both arrive as an ordinary TrackChangedMsg over a short queue with
// the selection barrier already down — indistinguishable, at that point, from
// the report a real start produces. So each case below ends playback on purpose,
// then hands the model every report and every tick that could be mistaken for a
// resumption, and only afterwards performs the gesture that genuinely is one.
func TestAutoDJStopIgnoresUnsolicitedTrackChanges(t *testing.T) {
	t.Run("a reconnect report does not resume an explicitly stopped refill", func(t *testing.T) {
		writer := &stubDJQueueWriter{prepareErr: errors.New("disk full")}
		m, p := djStopBoundaryModel(writer)

		// The player reaches the last row and the refill it launches is refused.
		m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
		if len(writer.prepared) != 1 {
			t.Fatalf("the low-water mark attempted %d batches, want exactly 1", len(writer.prepared))
		}
		if !m.DJPlanBlocked() {
			t.Fatal("a refused write did not park the refill, so nothing below is about a parked one")
		}

		// The user presses Stop, and mpv reports the file the stop ended.
		rm, _, _ := m.handlePlaybackKeyAction(keys.ActionStop)
		m = rm.(Model)
		m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
		if m.playback.state != playback.StateIdle {
			t.Fatalf("playback state is %v after Stop and the end it produced, want idle", m.playback.state)
		}

		// The writer recovers a second later: the refill could run now, and the
		// only question left is whether anything is entitled to ask it.
		writer.prepareErr = nil
		queued := len(m.queue.Entries)

		// The event socket drops and comes back, and mpv announces the row it is
		// still holding. Nobody pressed anything.
		m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
		m = djWakeTicks(m, djMaxTransientRetries+3)

		djStopIntentAssertQuiet(t, "a reconnect report", writer, p, m, queued, 1)

		// Deliberate resumption: a real selection, followed through to the start
		// the player reports for it.
		m = djStopBoundarySelect(m, 1)

		djStopIntentAssertResumed(t, writer, m, queued)
	})

	t.Run("a pre-stop report and a pre-stop batch do not resume a stop-after-current", func(t *testing.T) {
		writer := &stubDJQueueWriter{}
		m, p := djStopBoundaryModel(writer)

		// The player reaches the last row and launches a refill whose result is
		// held back, the way a slow planner leaves one in flight across a keypress.
		next, cmd := m.Update(djStopBoundaryTrackChange())
		m = next.(Model)
		inFlight := djWakeMessages(cmd)
		if len(writer.prepared) != 1 {
			t.Fatalf("the low-water mark attempted %d batches, want exactly 1", len(writer.prepared))
		}
		if len(writer.accepted) != 0 {
			t.Fatal("the held batch published itself, so nothing below is about an in-flight one")
		}

		// The user asks playback to end after this track, and it does.
		rm, _, _ := m.handlePlaybackKeyAction(keys.ActionStopAfterCurrent)
		m = rm.(Model)
		if !m.playback.stopAfterCurrent {
			t.Fatal("stop-after-current was not armed, so the end below is ordinary exhaustion")
		}
		m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
		if m.playback.state != playback.StateIdle {
			t.Fatalf("playback state is %v after a stop-after-current, want idle", m.playback.state)
		}

		queued := len(m.queue.Entries)

		// Everything the stop raced now arrives at once: the batch that was
		// already planned and written hidden, and the start-file for the row the
		// player was on when the user pressed. Neither is a new request.
		for _, msg := range inFlight {
			m = djWakeDrive(m, msg, 4)
		}
		m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
		m = djWakeTicks(m, djMaxTransientRetries+3)

		djStopIntentAssertQuiet(t, "a delayed pre-stop report", writer, p, m, queued, 1)
		if len(writer.rejected) != 1 {
			t.Errorf("the in-flight batch left %d preparations retired, want 1: "+
				"its hidden rows are stranded", len(writer.rejected))
		}

		// Deliberate resumption, followed through the same way.
		m = djStopBoundarySelect(m, 1)

		djStopIntentAssertResumed(t, writer, m, queued)
	})

	t.Run("a refused start leaves the suspension in place", func(t *testing.T) {
		writer := &stubDJQueueWriter{prepareErr: errors.New("disk full")}
		m, p := djStopBoundaryModel(writer)

		m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
		if !m.DJPlanBlocked() {
			t.Fatal("a refused write did not park the refill, so nothing below is about a parked one")
		}
		rm, _, _ := m.handlePlaybackKeyAction(keys.ActionStop)
		m = rm.(Model)
		m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)

		writer.prepareErr = nil
		queued := len(m.queue.Entries)

		// The user asks for a row and the player refuses to load it. The gesture
		// was real; the playback it asked for never happened.
		p.playFromIndexErr = errStubPlay
		m = djWakeDrive(m, playback.PlayQueueIndexMsg{Index: 1}, 4)
		if len(p.playFromIndexCalls) != 1 {
			t.Fatalf("the selection reached the player %d times, want 1", len(p.playFromIndexCalls))
		}

		// A reconnect report arrives behind the refusal. It names the row mpv is
		// holding, which is the row the refused selection asked for, so nothing in
		// the message itself says the start failed.
		m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
		m = djWakeTicks(m, djMaxTransientRetries+3)

		if len(writer.prepared) != 1 {
			t.Errorf("a refused start prepared %d batches, want the 1 from before the stop",
				len(writer.prepared))
		}
		if len(writer.accepted) != 0 {
			t.Errorf("a refused start published %d batches", len(writer.accepted))
		}
		if got := len(m.queue.Entries); got != queued {
			t.Errorf("the queue grew from %d to %d rows behind a refused start", queued, got)
		}

		// The same selection, taken this time.
		p.playFromIndexErr = nil
		m = djStopBoundarySelect(m, 1)

		djStopIntentAssertResumed(t, writer, m, queued)
	})
}

// djStopIntentAssertQuiet checks that nothing the DJ does was woken by what the
// caller just delivered: no new planning, no publication, no queue growth, and
// no player start. wantPrepared is the batch count earned before the stop, which
// the stop itself must not add to.
func djStopIntentAssertQuiet(t *testing.T, what string, writer *stubDJQueueWriter,
	p *stubStopAfterPlayer, m Model, queued, wantPrepared int,
) {
	t.Helper()
	if len(writer.prepared) != wantPrepared {
		t.Errorf("%s prepared %d batches, want the %d from before the stop",
			what, len(writer.prepared), wantPrepared)
	}
	if len(writer.accepted) != 0 {
		t.Errorf("%s published %d batches after the user ended playback", what, len(writer.accepted))
	}
	if got := len(m.queue.Entries); got != queued {
		t.Errorf("%s grew the queue from %d to %d rows", what, queued, got)
	}
	if p.playCalls != 0 || len(p.playFromIndexCalls) != 0 {
		t.Errorf("%s restarted the stopped player: %d plays, %d index plays",
			what, p.playCalls, len(p.playFromIndexCalls))
	}
	// The status bar does follow an unsolicited report onto the row it names —
	// that is what the report is for, and a pause change corrects it. What must
	// not follow it is the refill suspension, so that is asserted directly: it is
	// the state every guard above reads, and naming it makes a regression say so
	// rather than leaving it to be inferred from a count.
	if !m.radio.stoppedByUser {
		t.Errorf("%s lifted the deliberate-stop refill suspension", what)
	}
}

// djStopIntentAssertResumed checks that a start the user actually made puts the
// refill back to work, so the suspension above is a distinction rather than a
// permanent silence.
func djStopIntentAssertResumed(t *testing.T, writer *stubDJQueueWriter, m Model, queued int) {
	t.Helper()
	if len(writer.accepted) != 1 {
		t.Fatalf("published %d batches after a deliberate selection, want 1: "+
			"the stop suspended the refill for good", len(writer.accepted))
	}
	if got := len(m.queue.Entries); got <= queued {
		t.Errorf("the resumed queue holds %d rows, want it refilled past the %d it kept", got, queued)
	}
}
