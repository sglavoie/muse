package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestBuildMixSeed(t *testing.T) {
	tracks := []library.Track{
		{Hash: "a", Artist: "ArtistX", Genre: "Rock", Duration: 180},
		{Hash: "b", Artist: "ArtistX", Genre: "Rock;Pop", Duration: 240},
		{Hash: "c", Artist: "ArtistY", Genre: "Pop", Duration: 120},
	}
	trackTags := map[string]map[int64]struct{}{
		"a": {1: {}, 2: {}},
		"b": {2: {}, 3: {}},
	}

	seed := buildMixSeed(tracks, trackTags)

	if seed.Artist != "ArtistX" {
		t.Errorf("expected top artist ArtistX, got %q", seed.Artist)
	}

	genreSet := make(map[string]struct{}, len(seed.Genres))
	for _, g := range seed.Genres {
		genreSet[g] = struct{}{}
	}
	if _, ok := genreSet["Rock"]; !ok {
		t.Error("expected genre Rock in seed")
	}
	if _, ok := genreSet["Pop"]; !ok {
		t.Error("expected genre Pop in seed")
	}

	if seed.Duration != 180 {
		t.Errorf("expected median duration 180, got %v", seed.Duration)
	}

	for _, id := range []int64{1, 2, 3} {
		if _, ok := seed.TagIDs[id]; !ok {
			t.Errorf("expected tag ID %d in seed", id)
		}
	}

	if seed.Hash != "" {
		t.Errorf("expected empty Hash for synthetic seed, got %q", seed.Hash)
	}
}

func TestMixesRadioSeedEnable(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	m2, _ := m.handleMixesRadioSeed()

	if !m2.radio.enabled {
		t.Fatal("expected radio to be enabled after handleMixesRadioSeed")
	}
	if m2.radio.syntheticSeed == nil {
		t.Fatal("expected syntheticSeed to be set")
	}
	if m2.radio.seedHash != "" {
		t.Errorf("expected seedHash to be empty for synthetic seed, got %q", m2.radio.seedHash)
	}
	if m2.statusBar.topNotification().msg == "" {
		t.Error("expected a status notification")
	}
}

func TestMixesRadioSeedDisable(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)
	m.mixes.cachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.selectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	m, _ = m.handleMixesRadioSeed()
	if !m.radio.enabled {
		t.Fatal("precondition: radio should be enabled")
	}

	m2, _ := m.handleMixesRadioSeed()

	if m2.radio.enabled {
		t.Error("expected radio to be disabled after second A press")
	}
	if m2.radio.syntheticSeed != nil {
		t.Error("expected syntheticSeed to be cleared after disable")
	}
}

func TestMixesRadioSeedEmptyFallback(t *testing.T) {
	m := newMixesModel(true, nil)
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)

	m2, _ := m.handleMixesRadioSeed()

	if !m2.radio.enabled {
		t.Error("expected fallback radioToggle to enable radio")
	}
	if m2.radio.syntheticSeed != nil {
		t.Error("expected syntheticSeed to remain nil in fallback path")
	}
}
