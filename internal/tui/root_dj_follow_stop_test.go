package tui

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// A Follow report also invalidates batches when it moves the seed. That must
// not bypass the stop guard, even when storage recovers and old work arrives.
func TestAutoDJFollowStopIgnoresUnsolicitedTrackChanges(t *testing.T) {
	for _, stop := range []struct {
		name string
		key  keys.Action
	}{
		{"explicit stop", keys.ActionStop},
		{"completed stop after current", keys.ActionStopAfterCurrent},
	} {
		for _, outcome := range []string{"refused write recovers", "late prepared batch"} {
			t.Run(stop.name+"/"+outcome, func(t *testing.T) {
				writer := &stubDJQueueWriter{}
				refused := outcome == "refused write recovers"
				if refused {
					writer.prepareErr = errors.New("disk full")
				}
				m, p := djStopBoundaryModel(writer)
				m.dj.settings.Anchor = radio.AnchorFollow

				// Ordinary playback moves Follow onto f2 and launches a refill.
				// Hold successful work across the stop; deliver refusals to park it.
				next, cmd := m.Update(djStopBoundaryTrackChange())
				m = next.(Model)
				inFlight := djWakeMessages(cmd)
				if m.DJSeed().TrackHash != "f2" || len(writer.prepared) != 1 || len(writer.accepted) != 0 {
					t.Fatal("Follow did not move to f2 and prepare exactly one unpublished batch")
				}
				if refused {
					for _, msg := range inFlight {
						m = djWakeDrive(m, msg, 4)
					}
					inFlight = nil
					if !m.DJPlanBlocked() {
						t.Fatal("the refused write did not park the refill")
					}
				}

				rm, stopCmd, _ := m.handlePlaybackKeyAction(stop.key)
				m = rm.(Model)
				for _, msg := range djWakeMessages(stopCmd) {
					m = djWakeDrive(m, msg, 4)
				}
				if stop.key == keys.ActionStop && p.stopCalls != 1 {
					t.Fatal("Stop did not reach the player")
				}
				if stop.key == keys.ActionStopAfterCurrent && !m.playback.stopAfterCurrent {
					t.Fatal("stop-after-current was not armed")
				}
				m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
				if m.playback.state != playback.StateIdle || !m.radio.stoppedByUser {
					t.Fatal("playback did not end with a deliberate-stop suspension")
				}
				writer.prepareErr = nil
				queued := len(m.queue.Entries)

				// A delayed report names another indexed queued row, so it reaches
				// the automatic reseed path as well as the start-confirmation guard.
				m = djWakeDrive(m, playback.TrackChangedMsg{
					Artist: "Alice", Title: "F1", Hash: "f1", Index: 0, QueueID: 1,
				}, 4)
				if m.DJSeed().TrackHash != "f1" {
					t.Fatal("the report did not exercise Follow reseeding to a different indexed track")
				}
				djStopIntentAssertQuiet(t, "a delayed Follow report", writer, p, m, queued, 1)
				for _, msg := range inFlight {
					m = djWakeDrive(m, msg, 4)
				}
				m = djWakeTicks(m, djMaxTransientRetries+3)
				djStopIntentAssertQuiet(t, "late work and ticks after Follow reseeding", writer, p, m, queued, 1)
				if !refused && len(writer.rejected) != 1 {
					t.Errorf("retired %d hidden batches, want 1", len(writer.rejected))
				}

				// Both supported gestures must still resume additions. A selection
				// changes the Follow seed again; a DJ restart captures a fresh one.
				if stop.key == keys.ActionStop {
					m = djStopBoundarySelect(m, 1)
					if m.DJSeed().TrackHash != "f2" {
						t.Error("Follow did not reseed after deliberate selection")
					}
				} else {
					m = djStopBoundaryToggleRadio(m)
					m = djStopBoundaryToggleRadio(m)
				}
				djStopIntentAssertResumed(t, writer, m, queued)
			})
		}
	}
}
