package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestMixEnqueueAllInsideOpenMix guards that E, advertised as "enqueue all"
// in the Mixes footer, enqueues the open mix's tracks at the track level
// rather than being ignored once the mix is open.
func TestMixEnqueueAllInsideOpenMix(t *testing.T) {
	tracks := sampleMixTracks()
	be := &stubBatchEnqueuer{}
	m := New(context.Background(),
		WithRecommendationReader(&stubRecReader{sufficient: true, tracks: tracks}),
		WithBatchEnqueuer(be),
	)
	m = m.switchToView(viewMixes)
	m.mixes.Mixes = append([]recommendation.MixInfo(nil), recommendation.Registry()...)
	m.mixes.CachedMixes = map[db.MixID][]library.Track{m.mixes.Mixes[1].ID: tracks}
	m.mixes.SelectedMix = 1
	m.mixes.OpenMix(m.mixes.Mixes[1].ID)

	next, cmd := m.handleKeyAction(keys.ActionMixEnqueueAll)
	if cmd == nil {
		t.Fatal("expected an enqueue command inside an open mix")
	}
	msg, ok := cmd().(BatchEnqueuedMsg)
	if !ok || msg.Err != nil {
		t.Fatalf("expected a successful BatchEnqueuedMsg, got %#v", msg)
	}
	if len(be.appendTracksCalls) != 1 || len(be.appendTracksCalls[0]) != len(tracks) {
		t.Fatalf("expected all %d mix tracks appended once, got %v", len(tracks), be.appendTracksCalls)
	}
	if got, want := be.appendTracksSources[0], m.mixes.Mixes[1].Name; got != want {
		t.Errorf("enqueue source = %q, want the open mix %q", got, want)
	}
	if next.(Model).mixes.Level != 1 {
		t.Error("enqueue all must not leave the open mix")
	}
}

// TestMixesFilterAtListLevelExplainsWhy guards that "/" on the Mixes list,
// which the footer advertises, says how to reach a filterable list instead of
// claiming filtering is unavailable in the view.
func TestMixesFilterAtListLevelExplainsWhy(t *testing.T) {
	m := newMixesModel(true, sampleMixTracks())
	m = m.switchToView(viewMixes)
	m, _ = m.handleFilterBarMsg()
	if !insightsNotified(m, "Open a mix to filter its tracks") {
		t.Errorf("expected an open-a-mix notice, got %+v", m.statusBar.Notifications)
	}
}
