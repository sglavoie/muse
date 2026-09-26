package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestMixesSStopsOnListAndSortsInsideMix guards that s, which Mixes rebinds
// to sort-cycle for an open mix, still stops playback on the mix list where
// there is nothing to sort, and that ctrl+s never stops.
func TestMixesSStopsOnListAndSortsInsideMix(t *testing.T) {
	p := &stubCommandPlayer{}
	m := New(context.Background(),
		WithRecommendationReader(&stubRecReader{sufficient: true, tracks: sampleMixTracks()}),
		WithPlayer(p),
	)
	m = m.switchToView(viewMixes)
	m.mixes.Mixes = append([]recommendation.MixInfo(nil), recommendation.Registry()...)
	m.playback.state = playback.StatePlaying

	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = next.(Model)
	if p.stopCalls != 0 {
		t.Fatalf("ctrl+s on the mix list must not stop, got %d stop calls", p.stopCalls)
	}
	m = pressKey(m, "s")
	if p.stopCalls != 1 {
		t.Fatalf("s on the mix list should stop playback, got %d stop calls", p.stopCalls)
	}

	m.mixes.CachedMixes = map[db.MixID][]library.Track{m.mixes.Mixes[0].ID: sampleMixTracks()}
	m.mixes.OpenMix(m.mixes.Mixes[0].ID)
	m.playback.state = playback.StatePlaying
	before := m.mixes.TrackView.SortState
	m = pressKey(m, "s")
	if p.stopCalls != 1 {
		t.Errorf("s inside an open mix must sort, not stop; got %d stop calls", p.stopCalls)
	}
	if m.mixes.TrackView.SortState == before {
		t.Error("s inside an open mix should cycle the sort field")
	}
}
