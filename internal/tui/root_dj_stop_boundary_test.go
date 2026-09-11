package tui

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// These two tests are the other half of the wakeup story. The tick exists so a
// refill that parked on a refused write or an empty result is asked again once
// the world moves; both tests below move the world in exactly the way that
// wakeup is built to notice, after the user has ended playback on purpose. A
// stop the user asked for is not a queue that ran dry, and a recovered writer
// is not permission to start audio again.

// djStopBoundaryModel builds the refill fixture on its last queued row with a
// player that can be stopped. The second row is what the track change below
// moves onto: arriving on the final entry is what puts the queue under the
// low-water-mark while playback is still running, which is the only way to park
// a refill without ending the queue first.
func djStopBoundaryModel(writer *stubDJQueueWriter) (Model, *stubStopAfterPlayer) {
	m := djRefillModel(writer)
	m.queue.Entries = append(m.queue.Entries, queue.Entry{
		ID: 2, Hash: "f2", Path: "/f2.flac", Artist: "Alice", Title: "F2",
	})
	p := &stubStopAfterPlayer{}
	m.player = p
	return m, p
}

// djStopBoundaryTrackChange reports the player arriving on the last queued row.
//
// It is used for two opposite things on purpose. Behind a selection it is the
// start-file a successful load produces; on its own it is the report mpv makes
// about the row it is already holding — a reconnect echo, or a start-file
// emitted before a Stop and delivered after it. The message is identical either
// way, which is why nothing may read a user's intent out of it.
func djStopBoundaryTrackChange() playback.TrackChangedMsg {
	return playback.TrackChangedMsg{Artist: "Alice", Title: "F2", Hash: "f2", Index: 1, QueueID: 2}
}

// djStopBoundarySelect presses Enter on a queue row the way the queue view does
// and follows the whole gesture through: the player's result, then the
// start-file it answers a successful load with. Resumption is asserted against
// this rather than against a bare track change, because a bare track change is
// exactly what an unsolicited report looks like.
func djStopBoundarySelect(m Model, idx int) Model {
	entry := m.queue.Entries[idx]
	m = djWakeDrive(m, playback.PlayQueueIndexMsg{Index: idx}, 4)
	return djWakeDrive(m, playback.TrackChangedMsg{
		Artist: entry.Artist, Title: entry.Title, Hash: entry.Hash,
		Index: idx, QueueID: entry.ID,
	}, 4)
}

// TestAutoDJParkedRefillStaysStoppedAfterStopAfterCurrent covers a refill parked
// on a refused write when the user has asked playback to end after this track.
//
// The stop-after-current flag is already back to false by the time the end
// handler is done with it, and the queue it leaves behind looks exactly like an
// exhausted one: short, idle, with a park waiting for the writer to recover. So
// the boundary has to be recorded while the flag is still readable, or the very
// next tick republishes the batch the user's stop was meant to prevent.
func TestAutoDJParkedRefillStaysStoppedAfterStopAfterCurrent(t *testing.T) {
	writer := &stubDJQueueWriter{prepareErr: errors.New("disk full")}
	m, p := djStopBoundaryModel(writer)

	// The player reaches the last row and the refill it launches is refused.
	m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
	if len(writer.prepared) != 1 {
		t.Fatalf("the low-water-mark attempted %d batches, want exactly 1", len(writer.prepared))
	}
	if !m.DJPlanBlocked() {
		t.Fatal("a refused write did not park the refill, so nothing below is about a parked one")
	}

	// The user asks for the music to end after this track, and it does.
	rm, _, _ := m.handlePlaybackKeyAction(keys.ActionStopAfterCurrent)
	m = rm.(Model)
	if !m.playback.stopAfterCurrent {
		t.Fatal("stop-after-current was not armed, so the end below is ordinary exhaustion")
	}
	m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
	if m.playback.state != playback.StateIdle {
		t.Fatalf("playback state is %v after a stop-after-current, want idle", m.playback.state)
	}
	if len(writer.prepared) != 1 {
		t.Fatalf("the stop-after-current itself attempted %d batches, want the 1 from before it",
			len(writer.prepared))
	}

	// The database recovers a second later — the event the tick wakeup exists
	// to notice, and the one thing that must not restart a stopped player.
	writer.prepareErr = nil
	queued := len(m.queue.Entries)
	m = djWakeTicks(m, djMaxTransientRetries+3)

	if len(writer.prepared) != 1 {
		t.Errorf("ticks prepared %d batches after a stop-after-current, want the 1 from before it",
			len(writer.prepared))
	}
	if len(writer.accepted) != 0 {
		t.Errorf("ticks published %d batches after the user ended playback", len(writer.accepted))
	}
	if got := len(m.queue.Entries); got != queued {
		t.Errorf("the queue grew from %d to %d rows after a stop-after-current", queued, got)
	}
	if p.playCalls != 0 || len(p.playFromIndexCalls) != 0 {
		t.Errorf("the stopped player was started again: %d plays, %d index plays",
			p.playCalls, len(p.playFromIndexCalls))
	}
	if m.playback.state != playback.StateIdle {
		t.Errorf("playback state is %v after the ticks, want it left idle", m.playback.state)
	}

	// Deliberate resumption: the user selects a row, the player takes it and
	// reports the file it started, and continuous radio is continuous again. The
	// selection is the half that matters — the report by itself is the same
	// message an idle reconnect makes, so it proves nothing on its own.
	m = djStopBoundarySelect(m, 1)

	if len(writer.accepted) != 1 {
		t.Fatalf("published %d batches after playback resumed, want 1: "+
			"the stop suspended the refill for good", len(writer.accepted))
	}
	if got := len(m.queue.Entries); got <= queued {
		t.Errorf("the resumed queue holds %d rows, want it refilled past the %d it kept", got, queued)
	}
}

// TestAutoDJParkedRefillStaysStoppedAfterExplicitStop covers the same park under
// an explicit Stop, where there is no flag to read at all.
//
// Stop never sets stopAfterCurrent, so every guard downstream of it sees an
// ordinary idle player over a short queue. Both wakeups that reach a parked
// refill are exercised here: the end-of-file event mpv reports as the stop takes
// effect, and the tick that notices an eligible track arriving afterwards.
func TestAutoDJParkedRefillStaysStoppedAfterExplicitStop(t *testing.T) {
	writer := &stubDJQueueWriter{}
	reader := newDJRecReader()
	m, p := djStopBoundaryModel(writer)
	m.recReader = reader
	m.dj.settings = djDroneSettings()

	// The player reaches the last row while the session is filtered to a genre
	// the library does not carry, so the refill parks on an empty result.
	m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
	if !m.DJPlanBlocked() {
		t.Fatal("an empty result left the refill unparked, so nothing below is about a parked one")
	}
	if len(writer.prepared) != 0 {
		t.Fatalf("prepared %d batches with nothing eligible to select", len(writer.prepared))
	}

	// The user presses Stop.
	rm, _, _ := m.handlePlaybackKeyAction(keys.ActionStop)
	m = rm.(Model)
	if p.stopCalls != 1 {
		t.Fatalf("the player was stopped %d times, want 1", p.stopCalls)
	}
	if m.playback.state != playback.StateStopped {
		t.Fatalf("playback state is %v after Stop, want stopped", m.playback.state)
	}

	// mpv reports the file it was told to end. The queue is short and nothing
	// marks this end as deliberate, so it is the gate that has to refuse.
	m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
	if len(writer.prepared) != 0 {
		t.Fatalf("the end event the stop produced prepared %d batches", len(writer.prepared))
	}

	// A track the filters do admit arrives through the watcher: the eligibility
	// change the tick wakeup exists to pick up.
	queued := len(m.queue.Entries)
	m = djWakeDrive(m, watchBatchMsg{tracks: []library.Track{{
		Artist: "Gus", Album: "Eta", Title: "N1", Hash: "n1",
		Path: "/n1.flac", Genre: "Drone", Duration: 300,
	}}}, 4)
	m = djWakeTicks(m, 5)

	if len(writer.prepared) != 0 {
		t.Errorf("ticks prepared %d batches after an explicit stop", len(writer.prepared))
	}
	if len(writer.accepted) != 0 {
		t.Errorf("ticks published %d batches after an explicit stop", len(writer.accepted))
	}
	if got := len(m.queue.Entries); got != queued {
		t.Errorf("the queue grew from %d to %d rows after an explicit stop", queued, got)
	}
	if p.playCalls != 0 || len(p.playFromIndexCalls) != 0 {
		t.Errorf("the stopped player was started again: %d plays, %d index plays",
			p.playCalls, len(p.playFromIndexCalls))
	}

	// Deliberate resumption, by the other gesture that means it: turning the DJ
	// off and on again, which is a fresh session rather than a resumed track.
	m = djStopBoundaryToggleRadio(m)
	m = djStopBoundaryToggleRadio(m)

	if len(writer.accepted) != 1 {
		t.Fatalf("published %d batches after the DJ was started again, want 1: "+
			"the stop suspended the refill for good", len(writer.accepted))
	}
	if got := len(m.queue.Entries); got <= queued {
		t.Errorf("the restarted session left the queue at %d rows, want it refilled past %d", got, queued)
	}
}

// djStopBoundaryToggleRadio presses the radio toggle and delivers everything it
// schedules, so a start that refills in the same frame is followed through.
func djStopBoundaryToggleRadio(m Model) Model {
	rm, cmd, _ := m.radioToggle()
	m = rm.(Model)
	for _, msg := range djWakeMessages(cmd) {
		m = djWakeDrive(m, msg, 4)
	}
	return m
}
